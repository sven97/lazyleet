package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func caseKey(m *WorkspaceModel, k string) {
	m.handleCaseKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
}

func TestCaseManagerAddEditDeleteAndCancel(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 28})
	pressRune(&m, 't')
	if m.cases == nil || !strings.Contains(m.View(), "Test cases") {
		t.Fatal("t did not open manager")
	}
	count := len(m.cases.snapshot.Cases)
	caseKey(m, "a")
	m.cases.inputs.SetValue("[1,2,3]\n4")
	m.cases.expected.SetValue("[0,2]")
	m.handleCaseKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	cases, err := m.ws.ReadCases()
	if err != nil || len(cases) != count+1 || cases[count].Out != "[0,2]" {
		t.Fatalf("add: %+v %v", cases, err)
	}
	caseKey(m, "e")
	m.cases.expected.SetValue("[1,2]")
	m.handleCaseKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	cases, _ = m.ws.ReadCases()
	if cases[count].Out != "[1,2]" {
		t.Fatal("edit not persisted")
	}
	caseKey(m, "e")
	m.cases.expected.SetValue("[99]")
	m.handleCaseKey(tea.KeyMsg{Type: tea.KeyEsc})
	cases, _ = m.ws.ReadCases()
	if cases[count].Out != "[1,2]" {
		t.Fatal("cancel persisted")
	}
	caseKey(m, "d")
	m.handleCaseKey(tea.KeyMsg{Type: tea.KeyEsc})
	cases, _ = m.ws.ReadCases()
	if len(cases) != count+1 {
		t.Fatal("delete cancellation failed")
	}
	caseKey(m, "d")
	caseKey(m, "y")
	cases, _ = m.ws.ReadCases()
	if len(cases) != count {
		t.Fatal("delete not persisted")
	}
	m.handleCaseKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.cases != nil {
		t.Fatal("esc did not return to workspace")
	}
}

func TestCaseManagerValidationAndConflictRetainDraft(t *testing.T) {
	m := newTestModel(t)
	m.openCaseManager()
	m.editCase(-1)
	before, _ := os.ReadFile(m.ws.TestsPath)
	for _, tc := range []struct{ inputs, out string }{{"[1,]", ""}, {"[1,2]", ""}, {"[1,2]\n3", "invalid"}} {
		m.cases.inputs.SetValue(tc.inputs)
		m.cases.expected.SetValue(tc.out)
		m.saveCase()
		if !m.cases.editing || m.cases.message == "" {
			t.Fatal("invalid draft accepted")
		}
		after, _ := os.ReadFile(m.ws.TestsPath)
		if string(after) != string(before) {
			t.Fatal("validation failure modified file")
		}
	}
	m.cases.inputs.SetValue("[1,2]\n3")
	m.cases.expected.SetValue("[0,1]")
	changed := append(before, []byte("# external edit\n")...)
	if err := os.WriteFile(m.ws.TestsPath, changed, 0644); err != nil {
		t.Fatal(err)
	}
	m.saveCase()
	if !m.cases.editing || !strings.Contains(m.cases.message, "changed outside") {
		t.Fatal("conflict not shown")
	}
	after, _ := os.ReadFile(m.ws.TestsPath)
	if string(after) != string(changed) {
		t.Fatal("external changes overwritten")
	}
}

func TestCaseManagerInputKeysStayInFormAndLayoutFits(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m.openCaseManager()
	m.editCase(-1)
	pressRune(&m, 'q')
	pressRune(&m, 'b')
	pressRune(&m, 't')
	if m.cases == nil || m.cases.inputs.Value() != "qbt" {
		t.Fatal("form keys leaked to workspace")
	}
	for _, size := range [][2]int{{70, 20}, {40, 14}, {120, 40}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("view exceeds %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		if !strings.Contains(view, "ctrl+s") {
			t.Fatal("save hint missing")
		}
	}
}

func TestCaseManagerKeepsFailingCaseImport(t *testing.T) {
	m := newTestModel(t)
	m.remoteOut = &RemoteOutcome{LastCase: "[1,5,9]\n14"}
	m.openCaseManager()
	before := len(m.cases.snapshot.Cases)
	_, cmd := m.handleCaseKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	runCmd(&m, cmd)
	if len(m.cases.snapshot.Cases) != before+1 || !strings.Contains(m.cases.message, "imported 1") {
		t.Fatal("manager import did not refresh list")
	}
}

func TestCaseEditDiscardsInFlightResults(t *testing.T) {
	m := newTestModel(t)
	m.running = true
	m.openCaseManager()
	m.editCase(-1)
	m.cases.inputs.SetValue("[1,2]\n3")
	m.saveCase()
	m.Update(runFinishedMsg{}) // completion from before the edit
	if m.lastRun != nil || !m.casesUnrun || m.running {
		t.Fatal("old run appeared current after case edit")
	}
}
