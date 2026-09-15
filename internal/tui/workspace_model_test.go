package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/workspace"
)

// fakeRemote is a scripted RemoteJudge for tests.
type fakeRemote struct {
	available bool
	run       RemoteOutcome
	submit    RemoteOutcome
	runCalls  int
	subCalls  int
}

func (f *fakeRemote) Available() bool { return f.available }
func (f *fakeRemote) Run(context.Context, string, string) (RemoteOutcome, error) {
	f.runCalls++
	return f.run, nil
}
func (f *fakeRemote) Submit(context.Context, string) (RemoteOutcome, error) {
	f.subCalls++
	return f.submit, nil
}

func newTestModel(t *testing.T) *WorkspaceModel {
	return newTestModelWithRemote(t, nil)
}

func newTestModelWithRemote(t *testing.T, remote RemoteJudge) *WorkspaceModel {
	t.Helper()
	q, ok := leetcode.Fixture("two-sum")
	if !ok {
		t.Fatal("two-sum fixture missing")
	}
	ws, err := workspace.Scaffold(t.TempDir(), q, "python3")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	m, err := NewWorkspaceModel(ws, q, "true", false, 400, remote)
	if err != nil {
		t.Fatalf("NewWorkspaceModel: %v", err)
	}
	t.Cleanup(func() { m.watcher.Close() })
	return m
}

func TestWorkspaceModelRendersThreePanes(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*WorkspaceModel)

	view := m.View()
	for _, want := range []string{"Two Sum", "Easy", "solution.py", "Results"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n---\n%s", want, view)
		}
	}
}

func TestWorkspaceModelPaneCycling(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*WorkspaceModel)

	if m.focused != PaneCode {
		t.Fatalf("initial focus = %v, want Code", m.focused)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(*WorkspaceModel)
	if m.focused != PaneResults {
		t.Fatalf("after Tab focus = %v, want Results", m.focused)
	}
}

func TestWorkspaceModelZoomTogglesTabbed(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*WorkspaceModel)
	if m.layout.Tabbed {
		t.Fatal("wide layout should not start tabbed")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	m = updated.(*WorkspaceModel)
	if !m.layout.Tabbed {
		t.Fatal("z should zoom to a tabbed (single-pane) layout")
	}
}

func TestWorkspaceHelpTogglesAndCloses(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*WorkspaceModel)

	if strings.Contains(m.View(), "import the last failing case") {
		t.Fatal("help shouldn't show before ? is pressed")
	}

	pressRune(&m, '?')
	view := m.View()
	for _, want := range []string{"import the last failing case", "toggle this help"} {
		if !strings.Contains(view, want) {
			t.Errorf("help view missing %q:\n%s", want, view)
		}
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*WorkspaceModel)
	if m.showHelp {
		t.Fatal("esc should close help")
	}
}

func TestWorkspaceStatusBarAdvertisesImport(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*WorkspaceModel)

	bar := m.renderStatusBar()
	if !strings.Contains(bar, "import") {
		t.Errorf("status bar should advertise the import shortcut:\n%s", bar)
	}
}

func pressRune(m **WorkspaceModel, r rune) tea.Cmd {
	updated, cmd := (*m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	*m = updated.(*WorkspaceModel)
	return cmd
}

// runCmd executes a command tree, feeding results back into the model. It drops
// spinner ticks (which would recurse forever) and follow-up commands from run /
// remote handlers are still delivered.
func runCmd(m **WorkspaceModel, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch v := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range v {
			runCmd(m, c)
		}
	case spinner.TickMsg, nil:
		// ignore
	default:
		updated, next := (*m).Update(v)
		*m = updated.(*WorkspaceModel)
		runCmd(m, next)
	}
}

func TestWorkspaceRemoteUnavailableShowsHint(t *testing.T) {
	m := newTestModelWithRemote(t, &fakeRemote{available: false})
	pressRune(&m, 'R')
	if !strings.Contains(m.statusMsg, "lazyleet auth") {
		t.Fatalf("statusMsg = %q, want an auth hint", m.statusMsg)
	}
	if m.showRemote {
		t.Fatal("should not switch to the remote view when unavailable")
	}
}

func TestWorkspaceRemoteSubmitAcceptedRendersVerdict(t *testing.T) {
	fr := &fakeRemote{
		available: true,
		submit: RemoteOutcome{
			Kind: "submit", Verdict: "Accepted", Accepted: true,
			Passed: 57, Total: 57, Runtime: "12 ms", RuntimePct: 95.3, Memory: "17 MB",
		},
	}
	m := newTestModelWithRemote(t, fr)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	runCmd(&m, pressRune(&m, 's'))

	if fr.subCalls != 1 {
		t.Fatalf("Submit called %d times", fr.subCalls)
	}
	if !m.showRemote || m.remoteOut == nil || !m.remoteOut.Accepted {
		t.Fatalf("remote outcome not recorded: showRemote=%v out=%+v", m.showRemote, m.remoteOut)
	}
	if v := m.results.View(); !strings.Contains(v, "Accepted") || !strings.Contains(v, "12 ms") {
		t.Fatalf("results pane missing verdict:\n%s", v)
	}
}

// TestWorkspaceRemoteRunShowsInputFromWhatWasSent guards a real bug: the
// Results pane showed no "in" line at all for any case of a Run Code result,
// because it tried to reconstruct inputs by parsing the response's LastCase
// apart, and LeetCode doesn't reliably populate that field for a Run Code
// request (only for a real submission's one failing case). The fix uses what
// was actually sent (the local test-case file, read at send time) instead.
func TestWorkspaceRemoteRunShowsInputFromWhatWasSent(t *testing.T) {
	fr := &fakeRemote{
		available: true,
		run: RemoteOutcome{
			Kind: "run", Verdict: "Wrong Answer (sample)", Total: 2,
			// No LastCase — matches what a real Run Code response looks like.
			Expected: []string{"[0,1]", "[1,2]"},
			Actual:   []string{"[0,1]", "[9,9]"},
		},
	}
	m := newTestModelWithRemote(t, fr)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	runCmd(&m, pressRune(&m, 'R'))

	v := m.results.View()
	for _, want := range []string{"[2,7,11,15], 9", "[3,2,4], 6"} {
		if !strings.Contains(v, want) {
			t.Errorf("results pane missing input %q:\n%s", want, v)
		}
	}
}

func TestWorkspaceImportFailingCaseAppendsAndRuns(t *testing.T) {
	fr := &fakeRemote{
		available: true,
		run: RemoteOutcome{
			Kind: "run", Verdict: "Wrong Answer (sample)", Accepted: false,
			LastCase: "[1,5,9]\n14",
		},
	}
	m := newTestModelWithRemote(t, fr)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	runCmd(&m, pressRune(&m, 'R')) // remote run -> Wrong Answer with a failing case
	if m.remoteOut == nil || m.remoteOut.LastCase == "" {
		t.Fatal("expected a failing case from the remote run")
	}

	before, _ := m.ws.ReadCases()
	runCmd(&m, pressRune(&m, 'i')) // import it
	after, err := m.ws.ReadCases()
	if err != nil {
		t.Fatalf("ReadCases: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("case count %d -> %d, want +1", len(before), len(after))
	}
	last := after[len(after)-1]
	if len(last.In) != 2 || last.In[0] != "[1,5,9]" || last.In[1] != "14" {
		t.Fatalf("imported case wrong: %+v", last)
	}
}

// TestWorkspaceImportConfirmationSurvivesTheAutoRun guards against a
// regression where importFailingCase's "imported N case(s)" statusMsg was set
// and then immediately wiped by the very startRun() call it triggers
// (startRun clears statusMsg as part of a normal run) — the user pressed `i`
// and got no visible confirmation that anything happened.
func TestWorkspaceImportConfirmationSurvivesTheAutoRun(t *testing.T) {
	fr := &fakeRemote{
		available: true,
		run:       RemoteOutcome{Kind: "run", LastCase: "[1,5,9]\n14"},
	}
	m := newTestModelWithRemote(t, fr)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	runCmd(&m, pressRune(&m, 'R'))
	runCmd(&m, pressRune(&m, 'i'))

	if !strings.Contains(m.statusMsg, "imported 1 case") {
		t.Fatalf("statusMsg = %q, want it to report the import once the auto-run settles", m.statusMsg)
	}
}

// TestWorkspaceImportDuplicateCaseIsReportedHonestly guards against
// AppendCases's dedup (by exact input match) being silently indistinguishable
// from a real import in the status message.
func TestWorkspaceImportDuplicateCaseIsReportedHonestly(t *testing.T) {
	fr := &fakeRemote{
		available: true,
		// Matches one of Two Sum's seeded example cases exactly.
		run: RemoteOutcome{Kind: "run", LastCase: "[2,7,11,15]\n9"},
	}
	m := newTestModelWithRemote(t, fr)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	before, _ := m.ws.ReadCases()
	runCmd(&m, pressRune(&m, 'R'))
	runCmd(&m, pressRune(&m, 'i'))
	after, _ := m.ws.ReadCases()

	if len(after) != len(before) {
		t.Fatalf("case count %d -> %d, want unchanged (duplicate input)", len(before), len(after))
	}
	if strings.Contains(m.statusMsg, "imported 1 case") {
		t.Errorf("statusMsg = %q, shouldn't claim an import that was really a no-op", m.statusMsg)
	}
}
