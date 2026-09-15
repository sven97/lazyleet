package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/tui"
)

// remoteJudge implements tui.RemoteJudge over a LeetCode client. It is created
// per workspace session.
type remoteJudge struct {
	app    *appContext
	client *leetcode.Client
	slug   string
	qid    int
	lang   string
	authed bool
}

func newRemoteJudge(app *appContext, q leetcode.Question, lang string) tui.RemoteJudge {
	creds, _ := app.loadCredentials()
	client, _ := app.newClient()
	return &remoteJudge{
		app:    app,
		client: client,
		slug:   q.Slug,
		qid:    q.QuestionID,
		lang:   lang,
		authed: client != nil && !creds.Anonymous(),
	}
}

func (r *remoteJudge) Available() bool { return r.authed && r.client != nil && r.qid != 0 }

func (r *remoteJudge) Run(ctx context.Context, code, dataInput string) (tui.RemoteOutcome, error) {
	ir, err := r.client.Interpret(ctx, r.slug, r.qid, r.lang, code, dataInput)
	if err != nil {
		return tui.RemoteOutcome{}, err
	}
	res, err := r.client.PollResult(ctx, r.slug, ir.InterpretID, nil)
	if err != nil {
		return tui.RemoteOutcome{}, err
	}
	return mapOutcome("run", res), nil
}

func (r *remoteJudge) Submit(ctx context.Context, code string) (tui.RemoteOutcome, error) {
	sr, err := r.client.Submit(ctx, r.slug, r.qid, r.lang, code)
	if err != nil {
		return tui.RemoteOutcome{}, err
	}
	res, err := r.client.PollResult(ctx, r.slug, fmt.Sprint(sr.SubmissionID), nil)
	if err != nil {
		return tui.RemoteOutcome{}, err
	}
	out := mapOutcome("submit", res)
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
		Kind:          kind,
		Verdict:       res.StatusMsg,
		Passed:        res.TotalCorrect,
		Total:         res.TotalTestcases,
		Runtime:       res.StatusRuntime,
		Memory:        res.StatusMemory,
		RuntimePct:    res.RuntimePercentile,
		MemoryPct:     res.MemoryPercentile,
		LastCase:      res.LastTestcase,
		Stdout:        res.CodeOutput,
		Expected:      res.ExpectedAnswer,
		Actual:        res.CodeAnswer,
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
