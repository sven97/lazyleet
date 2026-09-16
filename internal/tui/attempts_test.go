package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sven97/lazyleet/internal/attempt"
	"github.com/sven97/lazyleet/internal/runner"
)

type memoryHistory struct {
	entries          []attempt.Entry
	saveErr, listErr error
}

func (h *memoryHistory) RecordAttempt(_ context.Context, e attempt.Entry) error {
	if h.saveErr != nil {
		return h.saveErr
	}
	h.entries = append(h.entries, e)
	return nil
}
func (h *memoryHistory) RecentAttempts(_ context.Context, slug string, limit int) ([]attempt.Entry, error) {
	if h.listErr != nil {
		return nil, h.listErr
	}
	var out []attempt.Entry
	for i := len(h.entries) - 1; i >= 0 && len(out) < limit; i-- {
		if h.entries[i].Slug == slug {
			out = append(out, h.entries[i])
		}
	}
	return out, nil
}

type historyRunner struct {
	result runner.Result
	err    error
}

func (r historyRunner) Available() bool { return true }
func (r historyRunner) Run(context.Context, runner.Spec) (runner.Result, error) {
	return r.result, r.err
}

type historyRemote struct{ err error }

func (r historyRemote) Available() bool { return true }
func (r historyRemote) Run(context.Context, string, string) (RemoteOutcome, error) {
	return RemoteOutcome{RemoteID: "run-123", Verdict: "Sample tests passed", Passed: 1, Total: 1}, r.err
}
func (r historyRemote) Submit(context.Context, string) (RemoteOutcome, error) {
	return RemoteOutcome{RemoteID: "submit-456", Verdict: "Accepted", Passed: 10, Total: 10}, r.err
}

func TestCommandsPersistLocalRunAndSubmitBeforeUpdate(t *testing.T) {
	m := newTestModel(t)
	repo := &memoryHistory{}
	m.SetHistory(repo)
	m.runner = historyRunner{result: runner.Result{Passed: 1, Total: 1, Elapsed: time.Millisecond}}
	m.remote = historyRemote{}
	msg := m.runCmd()()
	if len(repo.entries) != 1 || repo.entries[0].Verdict != "Passed" {
		t.Fatal("local result not recorded in command")
	}
	m.Update(msg)
	for _, kind := range []string{"run", "submit"} {
		msg = m.remoteCmd(kind)()
		m.Update(msg)
	}
	if len(repo.entries) != 3 || repo.entries[1].Kind != "run" || repo.entries[2].Kind != "submit" || repo.entries[2].RemoteID != "submit-456" {
		t.Fatalf("missing remote history: %+v", repo.entries)
	}
	for _, e := range repo.entries {
		if e.Slug != "two-sum" || e.Lang != "python3" || e.CreatedAt.IsZero() {
			t.Fatalf("missing identity: %+v", e)
		}
	}
	// Closing the workspace must not prevent a command already created from
	// recording its eventual result, even without delivering its completion.
	cmd := m.runCmd()
	m.Close()
	cmd()
	if len(repo.entries) != 4 {
		t.Fatal("completion after close was lost")
	}
}

func TestHistoryRecordsErrorsAndDoesNotReplaceJudgeResult(t *testing.T) {
	m := newTestModel(t)
	repo := &memoryHistory{}
	m.SetHistory(repo)
	m.runner = historyRunner{err: errors.New("runner unavailable")}
	m.runCmd()()
	m.remote = historyRemote{err: errors.New("poll timeout")}
	m.remoteCmd("submit")()
	if len(repo.entries) != 2 || repo.entries[0].Verdict != "Run error" || !strings.Contains(repo.entries[0].Detail, "runner unavailable") || repo.entries[1].RemoteID != "submit-456" || !strings.Contains(repo.entries[1].Detail, "poll timeout") {
		t.Fatalf("lost errors: %+v", repo.entries)
	}
	repo.saveErr = errors.New("disk full")
	m.runner = historyRunner{result: runner.Result{Passed: 1, Total: 1}}
	msg := m.runCmd()()
	m.Update(msg)
	if m.lastRun == nil || !m.lastRun.OK() || !strings.Contains(m.statusMsg, "could not save attempt history") {
		t.Fatal("save failure replaced judge success")
	}

	// A remote verdict already carries a status-bar summary; a subsequent
	// history-save failure must fold into it, not clobber it.
	m.remote = historyRemote{}
	msg = m.remoteCmd("submit")()
	m.Update(msg)
	if !strings.Contains(m.statusMsg, "Accepted") || !strings.Contains(m.statusMsg, "disk full") {
		t.Fatalf("history save failure hid the submit verdict: %q", m.statusMsg)
	}
}

func TestLocalHistoryVerdicts(t *testing.T) {
	for _, tc := range []struct {
		res  runner.Result
		want string
	}{
		{runner.Result{}, "No test cases"},
		{runner.Result{BuildErr: "compiler error"}, "Build error"},
		{runner.Result{Total: 1, Cases: []runner.CaseResult{{Status: runner.StatusUnknown}}}, "Unverified"},
		{runner.Result{Total: 2, Cases: []runner.CaseResult{{Status: runner.StatusFail}, {Status: runner.StatusUnknown}}}, "Wrong answer"},
		{runner.Result{Total: 1, Cases: []runner.CaseResult{{Status: runner.StatusError}}}, "Runtime error"},
		{runner.Result{Total: 1, Cases: []runner.CaseResult{{Status: runner.StatusTimeout}}}, "Time limit exceeded"},
	} {
		if got := localAttempt("x", "python3", time.Now(), tc.res, nil); got.Verdict != tc.want {
			t.Fatalf("got %s want %s", got.Verdict, tc.want)
		}
	}
}

func TestHistoryViewNavigationEmptyFailureAndLongDetails(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	repo := &memoryHistory{}
	m.SetHistory(repo)
	runCmd(&m, pressRune(&m, 'a'))
	if !m.showHistory || !strings.Contains(ansi.Strip(m.View()), "No attempts") {
		t.Fatal("empty history not shown")
	}
	repo.entries = []attempt.Entry{{Slug: "two-sum", Lang: "python3", Kind: "local", Verdict: "Passed", CreatedAt: time.Now(), Detail: strings.Repeat("long diagnostic output ", 200)}}
	runCmd(&m, pressRune(&m, 'r'))
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.historyDetail || !strings.Contains(ansi.Strip(m.View()), "Passed") {
		t.Fatal("detail not opened")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.historyVP.YOffset == 0 {
		t.Fatal("details did not scroll")
	}
	for _, size := range [][2]int{{40, 12}, {80, 24}, {120, 40}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if lipgloss.Width(m.View()) > size[0] || lipgloss.Height(m.View()) > size[1] {
			t.Fatal("history detail overflow")
		}
		if m.historyVP.YOffset == 0 {
			t.Fatal("resize reset scroll position")
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	repo.listErr = errors.New("unreadable database")
	runCmd(&m, pressRune(&m, 'r'))
	if !strings.Contains(ansi.Strip(m.View()), "History unavailable") {
		t.Fatal("read failure hidden")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHistory {
		t.Fatal("history did not close")
	}
}

func TestHistoryIgnoresSupersededLoads(t *testing.T) {
	m := newTestModel(t)
	repo := &memoryHistory{}
	m.SetHistory(repo)
	older := m.loadHistory()
	newer := m.loadHistory()
	m.Update(newer())
	repo.entries = []attempt.Entry{{Slug: "two-sum", Lang: "python3", Kind: "local", Verdict: "Old result"}}
	m.Update(older())
	if len(m.historyEntries) != 0 {
		t.Fatal("outdated load replaced newer history")
	}
}
