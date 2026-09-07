package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/workspace"
)

func newTestModel(t *testing.T) *WorkspaceModel {
	t.Helper()
	q, ok := leetcode.Fixture("two-sum")
	if !ok {
		t.Fatal("two-sum fixture missing")
	}
	ws, err := workspace.Scaffold(t.TempDir(), q, "python3")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	m, err := NewWorkspaceModel(ws, q, "true", false, 400)
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
