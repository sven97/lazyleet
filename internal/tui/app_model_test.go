package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/workspace"
)

func TestAppModelOpenAndBackPreservesBrowseFilter(t *testing.T) {
	bm, _ := bootBrowse(t)
	// Apply a fuzzy filter so we can assert it survives the round-trip.
	step(&bm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "rain" {
		step(&bm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	step(&bm, tea.KeyMsg{Type: tea.KeyEsc}) // leave filtering mode, keep value? Esc clears in browse
	// Re-apply: `/` then type, then Enter to leave filtering without clearing.
	step(&bm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "rain" {
		step(&bm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	step(&bm, tea.KeyMsg{Type: tea.KeyEnter}) // leave filtering mode keeping filter
	if bm.currentSlug() != "trapping-rain-water" {
		t.Fatalf("precondition: filtered slug = %q", bm.currentSlug())
	}
	filterVal := bm.filter.Value()
	cursor := bm.cursor

	factory := func(slug string) (*WorkspaceModel, error) {
		q, ok := leetcode.Fixture("two-sum")
		if !ok {
			t.Fatal("fixture missing")
		}
		// Use two-sum fixture even if slug differs — factory only needs a valid model.
		_ = slug
		ws, err := workspace.Scaffold(t.TempDir(), q, "python3")
		if err != nil {
			return nil, err
		}
		return NewWorkspaceModel(ws, q, "true", false, 400, nil)
	}

	am := NewAppModel(bm, factory)
	updated, cmd := am.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	am = updated.(*AppModel)
	if cmd != nil {
		// drain init-ish cmds lightly
	}

	// Open current problem
	updated, cmd = am.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am = updated.(*AppModel)
	if cmd == nil {
		t.Fatal("expected OpenProblemMsg command")
	}
	msg := cmd()
	op, ok := msg.(OpenProblemMsg)
	if !ok {
		t.Fatalf("got %T, want OpenProblemMsg", msg)
	}
	updated, cmd = am.Update(op)
	am = updated.(*AppModel)
	if !am.InWorkspace() {
		t.Fatal("expected workspace mode after OpenProblemMsg")
	}
	if cmd != nil {
		// workspace Init — ignore
	}

	// Back to browse
	updated, cmd = am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	am = updated.(*AppModel)
	if cmd == nil {
		t.Fatal("expected BackToBrowseMsg command")
	}
	msg = cmd()
	if _, ok := msg.(BackToBrowseMsg); !ok {
		t.Fatalf("got %T, want BackToBrowseMsg", msg)
	}
	updated, _ = am.Update(msg)
	am = updated.(*AppModel)
	if am.InWorkspace() {
		t.Fatal("expected browse mode after BackToBrowseMsg")
	}

	bm = am.Browse()
	if bm.filter.Value() != filterVal {
		t.Fatalf("filter = %q, want %q (preserved)", bm.filter.Value(), filterVal)
	}
	if bm.cursor != cursor {
		t.Fatalf("cursor = %d, want %d (preserved)", bm.cursor, cursor)
	}
	if bm.currentSlug() != "trapping-rain-water" {
		t.Fatalf("slug = %q after return, want trapping-rain-water", bm.currentSlug())
	}
}

func TestAppModelQuitFromBrowseExits(t *testing.T) {
	bm, _ := bootBrowse(t)
	am := NewAppModel(bm, nil)
	updated, _ := am.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	am = updated.(*AppModel)
	updated, cmd := am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	am = updated.(*AppModel)
	if cmd == nil {
		t.Fatal("expected quit cmd from browse")
	}
	// tea.Quit is opaque; just ensure we did not open workspace
	if am.InWorkspace() {
		t.Fatal("q from browse should not open workspace")
	}
}

func TestWorkspaceBackStandaloneStillQuits(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*WorkspaceModel)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	_ = updated.(*WorkspaceModel)
	if cmd == nil {
		t.Fatal("standalone workspace back should quit")
	}
	// Should NOT be BackToBrowseMsg when returnToBrowse is false
	msg := cmd()
	if _, ok := msg.(BackToBrowseMsg); ok {
		t.Fatal("standalone workspace must not emit BackToBrowseMsg")
	}
}
