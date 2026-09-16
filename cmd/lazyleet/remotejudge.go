package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/tui"
)

// sessionRecheckInterval bounds how often authenticatedClient re-verifies a
// session against LeetCode. Run/Submit are pressed repeatedly within one
// workspace session, and re-pinging WhoAmI before every single one of them
// roughly doubled perceived latency for what is normally the common case (a
// still-valid session).
const sessionRecheckInterval = 60 * time.Second

// remoteJudge implements tui.RemoteJudge over a LeetCode client. It is created
// per workspace session.
type remoteJudge struct {
	app  *appContext
	slug string
	qid  int
	lang string

	// client/creds/verifiedAt cache the last-built, last-verified client so
	// repeated Run/Submit calls reuse one rate limiter instead of resetting
	// it on every keypress. They're rebuilt only when the on-disk credentials
	// actually change, e.g. a sign-in from another terminal.
	client     *leetcode.Client
	creds      leetcode.Credentials
	verifiedAt time.Time
}

func newRemoteJudge(app *appContext, q leetcode.Question, lang string) tui.RemoteJudge {
	return &remoteJudge{
		app:  app,
		slug: q.Slug,
		qid:  q.QuestionID,
		lang: lang,
	}
}

func (r *remoteJudge) Available() bool {
	creds, err := r.app.loadCredentials()
	return err == nil && !creds.Anonymous() && r.qid != 0
}

// Reload credentials for each attempt: signing in from another terminal must
// take effect without discarding the open workspace. The client itself (and
// its rate limiter) is reused across calls as long as credentials haven't
// changed, and the session is only re-verified against LeetCode at most once
// per sessionRecheckInterval rather than on every Run/Submit.
func (r *remoteJudge) authenticatedClient(ctx context.Context) (*leetcode.Client, error) {
	creds, err := r.app.loadCredentials()
	if err != nil {
		return nil, err
	}
	if creds.Anonymous() {
		return nil, fmt.Errorf("run `lazyleet auth` in another terminal, then retry R/s")
	}
	if r.client == nil || creds != r.creds {
		client, err := r.app.newClient()
		if err != nil {
			return nil, err
		}
		r.client, r.creds, r.verifiedAt = client, creds, time.Time{}
	}
	if time.Since(r.verifiedAt) > sessionRecheckInterval {
		if err := r.client.VerifySession(ctx); err != nil {
			if errors.Is(err, leetcode.ErrSessionExpired) {
				return nil, fmt.Errorf("run `lazyleet auth` in another terminal, then retry R/s")
			}
			return nil, err
		}
		r.verifiedAt = time.Now()
	}
	return r.client, nil
}

func (r *remoteJudge) Run(ctx context.Context, code, dataInput string) (tui.RemoteOutcome, error) {
	client, err := r.authenticatedClient(ctx)
	if err != nil {
		return tui.RemoteOutcome{}, err
	}
	ir, err := client.Interpret(ctx, r.slug, r.qid, r.lang, code, dataInput)
	if err != nil {
		return tui.RemoteOutcome{}, err
	}
	res, err := client.PollResult(ctx, r.slug, ir.InterpretID, nil)
	if err != nil {
		return tui.RemoteOutcome{RemoteID: ir.InterpretID}, err
	}
	out := mapOutcome("run", res)
	out.RemoteID = ir.InterpretID
	return out, nil
}

func (r *remoteJudge) Submit(ctx context.Context, code string) (tui.RemoteOutcome, error) {
	client, err := r.authenticatedClient(ctx)
	if err != nil {
		return tui.RemoteOutcome{}, err
	}
	sr, err := client.Submit(ctx, r.slug, r.qid, r.lang, code)
	if err != nil {
		return tui.RemoteOutcome{}, err
	}
	res, err := client.PollResult(ctx, r.slug, fmt.Sprint(sr.SubmissionID), nil)
	if err != nil {
		return tui.RemoteOutcome{RemoteID: fmt.Sprint(sr.SubmissionID)}, err
	}
	out := mapOutcome("submit", res)
	out.RemoteID = fmt.Sprint(sr.SubmissionID)
	if out.Accepted {
		r.markSolved()
	}
	return out, nil
}

func (r *remoteJudge) markSolved() {
	db, err := r.app.openStore()
	if err != nil {
		return
	}
	defer db.Close()
	_ = db.SetProblemStatus(context.Background(), r.slug, "ac")
}

func mapOutcome(kind string, res leetcode.JudgeResult) tui.RemoteOutcome {
	out := tui.RemoteOutcome{
		Kind:       kind,
		Verdict:    res.StatusMsg,
		Passed:     res.TotalCorrect,
		Total:      res.TotalTestcases,
		Runtime:    res.StatusRuntime,
		Memory:     res.StatusMemory,
		RuntimePct: res.RuntimePercentile,
		MemoryPct:  res.MemoryPercentile,
		LastCase:   res.LastTestcase,
		Stdout:     res.CodeOutput,
		// CodeAnswer/ExpectedAnswer (plural, per-case) are what's populated
		// for a Run Code check; a submission check leaves those empty and
		// carries the one failing case's actual/expected in CodeOutput/
		// ExpectedOutput instead — see the field comments on JudgeResult.
		Expected:      firstNonEmptySlice(res.ExpectedAnswer, res.ExpectedOutput),
		Actual:        firstNonEmptySlice(res.CodeAnswer, res.CodeOutput),
		CompareResult: res.CompareResult,
		CompileErr:    strings.TrimSpace(res.FullCompileError),
		RuntimeErr:    strings.TrimSpace(res.FullRuntimeError),
	}

	if kind == "run" {
		switch {
		case out.CompileErr != "":
			out.Verdict = "Compile Error"
		case out.RuntimeErr != "":
			out.Verdict = "Runtime Error"
		case res.CorrectAnswer:
			out.Verdict, out.Accepted = "Sample tests passed", true
		default:
			out.Verdict = "Wrong Answer (sample)"
		}
	} else {
		out.Accepted = res.Accepted()
		if out.Verdict == "" {
			out.Verdict = "Unknown"
		}
	}
	return out
}

// firstNonEmptySlice returns a, or b if a is empty.
func firstNonEmptySlice(a, b []string) []string {
	if len(a) > 0 {
		return a
	}
	return b
}
