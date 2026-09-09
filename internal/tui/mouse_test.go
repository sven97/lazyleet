package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/workspace"
)

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}
func wheel(x, y int, up bool) tea.MouseMsg {
	b := tea.MouseButtonWheelDown
	if up {
		b = tea.MouseButtonWheelUp
	}
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: b}
}

func TestBrowseRegionAt(t *testing.T) {
	m, _ := bootBrowse(t)
	center := func(r Rect) (int, int) { return r.X + r.W/2, r.Y + r.H/2 }

	for _, tc := range []struct {
		name string
		rect Rect
		want Region
	}{
		{"status", m.layout.Status, RegionStatus},
		{"sources", m.layout.Sources, RegionSources},
		{"list", m.layout.List, RegionList},
		{"detail", m.layout.Detail, RegionDetail},
	} {
		x, y := center(tc.rect)
		got, ok := m.regionAt(x, y)
		if !ok || got != tc.want {
			t.Errorf("regionAt%s -> %v (ok=%v), want %v", tc.name, got, ok, tc.want)
		}
	}
}

func TestBrowseClickListRowSelectsThenOpens(t *testing.T) {
	m, _ := bootBrowse(t)
	// row 1 of the list (0-indexed within m.filtered): terminal Y for that row.
	y := m.layout.List.Y + 3 + 1 // border + title + header, then row index 1
	x := m.layout.List.X + 5

	step(&m, press(x, y))
	if m.focus != RegionList {
		t.Fatalf("click should focus the list, got %v", m.focus)
	}
	if m.cursor != 1 {
		t.Fatalf("click should move cursor to row 1, got %d", m.cursor)
	}
	// second click on the same (now-selected) row opens it
	updated, cmd := m.Update(press(x, y))
	m = updated.(*BrowseModel)
	if m.Chosen != "valid-parentheses" || cmd == nil {
		t.Fatalf("second click should open the row: Chosen=%q cmd=%v", m.Chosen, cmd)
	}
}

func TestBrowseWheelOverListScrollsWithoutMovingCursor(t *testing.T) {
	m, _ := bootBrowse(t)
	// Shrink the terminal so the 3-row fixture list can't fit in its pane.
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 16})
	if m.listRows() >= len(m.filtered) {
		t.Fatalf("test needs an overflowing list: listRows=%d filtered=%d", m.listRows(), len(m.filtered))
	}
	cx, cy := m.layout.List.X+3, m.layout.List.Y+m.layout.List.H/2

	step(&m, wheel(cx, cy, false)) // down
	if m.top == 0 {
		t.Fatalf("wheel-down over the list should scroll the list (m.top)")
	}
	if m.cursor != 0 {
		t.Fatalf("wheel must not move the selection, cursor=%d", m.cursor)
	}
	step(&m, wheel(cx, cy, true)) // up
	if m.top != 0 {
		t.Fatalf("wheel-up should scroll back to the top, got m.top=%d", m.top)
	}
	if m.cursor != 0 {
		t.Fatalf("wheel must not move the selection, cursor=%d", m.cursor)
	}
}

func TestBrowseClickSourceActivatesIt(t *testing.T) {
	m, f := bootBrowse(t)
	// sources body: line1 = source[0] "All Problems", line4 = source[1] "Starter"
	y := m.layout.Sources.Y + 2 + 4
	x := m.layout.Sources.X + 3
	step(&m, planSlugsMsg{slug: "starter", slugs: f.planMap["starter"]})
	step(&m, press(x, y))
	if m.activeSrc != 1 {
		t.Fatalf("clicking the plan row should activate it, activeSrc=%d", m.activeSrc)
	}
	if len(m.view) != 2 || m.view[0].Slug != "valid-parentheses" {
		t.Fatalf("plan not applied: %+v", m.view)
	}
}

func TestWorkspaceClickFocusesPane(t *testing.T) {
	q, _ := leetcode.Fixture("two-sum")
	ws, _ := workspace.Scaffold(t.TempDir(), q, "python3")
	m, err := NewWorkspaceModel(ws, q, "true", false, 400, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.watcher.Close()
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)

	sx, sy := m.layout.Statement.X+m.layout.Statement.W/2, m.layout.Statement.Y+2
	up, _ = m.Update(press(sx, sy))
	m = up.(*WorkspaceModel)
	if m.focused != PaneStatement {
		t.Fatalf("click on the statement pane should focus it, got %v", m.focused)
	}
	rx, ry := m.layout.Results.X+2, m.layout.Results.Y+2
	up, _ = m.Update(press(rx, ry))
	m = up.(*WorkspaceModel)
	if m.focused != PaneResults {
		t.Fatalf("click on the results pane should focus it, got %v", m.focused)
	}
}
