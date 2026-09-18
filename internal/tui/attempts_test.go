package tui

import (
	"context"
	"errors"
	"fmt"
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

type remoteHistoryFake struct {
	available bool
	entries   []attempt.Entry
	err       error
}

func (r remoteHistoryFake) Available() bool { return r.available }
func (r remoteHistoryFake) Submissions(context.Context, int) ([]attempt.Entry, error) {
	return r.entries, r.err
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
	if m.lastRun == nil || !m.lastRun.OK() || !strings.Contains(m.statusMsg, "local: 1/1 passed") || !strings.Contains(m.statusMsg, "disk full") {
		t.Fatalf("save failure hid the judge verdict: %q", m.statusMsg)
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

func TestMergeRemoteHistoryDedupsAndSorts(t *testing.T) {
	local := []attempt.Entry{
		{Slug: "two-sum", Kind: "submit", Verdict: "Accepted", RemoteID: "222", CreatedAt: time.Unix(2000, 0)},
	}
	remote := remoteHistoryFake{available: true, entries: []attempt.Entry{
		{Slug: "two-sum", Kind: "remote", Verdict: "Accepted", RemoteID: "222", CreatedAt: time.Unix(2000, 0)}, // dup, must be dropped
		{Slug: "two-sum", Kind: "remote", Verdict: "Wrong Answer", RemoteID: "111", CreatedAt: time.Unix(1000, 0)},
		{Slug: "two-sum", Kind: "remote", Verdict: "Accepted", RemoteID: "333", CreatedAt: time.Unix(3000, 0)},
	}}
	got := mergeRemoteHistory(remote, local)
	if len(got) != 3 {
		t.Fatalf("want 3 entries after dedup, got %d: %+v", len(got), got)
	}
	if got[0].RemoteID != "333" || got[1].RemoteID != "222" || got[2].RemoteID != "111" {
		t.Fatalf("not sorted newest-first: %+v", got)
	}
}

func TestMergeRemoteHistoryIgnoresUnavailableOrFailingRemote(t *testing.T) {
	local := []attempt.Entry{{Slug: "two-sum", Kind: "local", RemoteID: "1", CreatedAt: time.Unix(1, 0)}}

	if got := mergeRemoteHistory(nil, local); len(got) != 1 {
		t.Fatalf("nil remote should pass local through unchanged, got %+v", got)
	}
	if got := mergeRemoteHistory(remoteHistoryFake{available: false}, local); len(got) != 1 {
		t.Fatalf("unavailable remote should pass local through unchanged, got %+v", got)
	}
	if got := mergeRemoteHistory(remoteHistoryFake{available: true, err: errors.New("expired session")}, local); len(got) != 1 {
		t.Fatalf("remote error should pass local through unchanged, got %+v", got)
	}
}

func TestMergeRemoteHistoryCapsAtMax(t *testing.T) {
	var local []attempt.Entry
	var remoteEntries []attempt.Entry
	for i := 0; i < maxHistoryEntries+10; i++ {
		remoteEntries = append(remoteEntries, attempt.Entry{Slug: "two-sum", Kind: "remote", RemoteID: fmt.Sprint(i), CreatedAt: time.Unix(int64(i), 0)})
	}
	got := mergeRemoteHistory(remoteHistoryFake{available: true, entries: remoteEntries}, local)
	if len(got) != maxHistoryEntries {
		t.Fatalf("want capped at %d, got %d", maxHistoryEntries, len(got))
	}
	// newest first: the highest timestamps should survive the cap.
	if got[0].RemoteID != fmt.Sprint(len(remoteEntries)-1) {
		t.Fatalf("cap kept the wrong end of the list: %+v", got[0])
	}
}

func TestLoadHistoryMergesRemoteSubmissions(t *testing.T) {
	m := newTestModel(t)
	repo := &memoryHistory{entries: []attempt.Entry{{Slug: "two-sum", Kind: "submit", RemoteID: "1", CreatedAt: time.Unix(1, 0)}}}
	m.SetHistory(repo)
	m.SetRemoteHistory(remoteHistoryFake{available: true, entries: []attempt.Entry{
		{Slug: "two-sum", Kind: "remote", RemoteID: "2", Verdict: "Accepted", CreatedAt: time.Unix(2, 0)},
	}})
	msg := m.loadHistory()()
	loaded, ok := msg.(historyLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("loadHistory failed: %+v", msg)
	}
	if len(loaded.entries) != 2 || loaded.entries[0].RemoteID != "2" {
		t.Fatalf("remote submission not merged in: %+v", loaded.entries)
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
