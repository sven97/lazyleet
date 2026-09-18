package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/workspace"
)

// keyRune builds the tea.KeyMsg a bare, unmodified keypress of r produces —
// the same shape digitKey (keys.go) parses.
func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// --- browse mode -------------------------------------------------------

func TestBrowseDigitKeyJumpsFocusFromAnyPane(t *testing.T) {
	m, _ := bootBrowse(t) // wide terminal: two-column layout, all rects non-empty
	cases := []struct {
		digit rune
		want  Region
	}{
		{'1', RegionStatus}, {'2', RegionSources}, {'3', RegionList}, {'4', RegionDetail},
	}
	for _, tc := range cases {
		m.setFocus(RegionList) // known starting point, different from every target but 3
		step(&m, keyRune(tc.digit))
		if m.focus != tc.want {
			t.Errorf("digit %q: focus = %v, want %v", tc.digit, m.focus, tc.want)
		}
	}
}

func TestBrowseDigitKeyFocusingStatusShowsStatusDetail(t *testing.T) {
	m, _ := bootBrowse(t)
	m.setFocus(RegionList)
	step(&m, keyRune('1'))
	if m.focus != RegionStatus || !m.detailShowsStatus {
		t.Fatalf("pressing 1 should jump to Status and show its detail, focus=%v detailShowsStatus=%v", m.focus, m.detailShowsStatus)
	}
}

func TestBrowseDigitKeyIgnoredWhileHelpShown(t *testing.T) {
	m, _ := bootBrowse(t)
	m.setFocus(RegionList)
	m.showHelp = true
	step(&m, keyRune('1'))
	if m.focus != RegionList {
		t.Errorf("digit keys should be inert while help is showing, focus changed to %v", m.focus)
	}
	if !m.showHelp {
		t.Error("a bare digit shouldn't close the help overlay either")
	}
}

func TestBrowseDigitKeyGoesToTheFuzzyFilterWhileFiltering(t *testing.T) {
	m, _ := bootBrowse(t)
	m.setFocus(RegionList)
	m.filtering = true
	m.filter.Focus()
	step(&m, keyRune('1'))
	if m.focus != RegionList {
		t.Errorf("digit keys shouldn't jump focus while the fuzzy filter is active, focus=%v", m.focus)
	}
	if m.filter.Value() != "1" {
		t.Errorf("the digit should have gone to the filter textinput instead, value=%q", m.filter.Value())
	}
}

func TestBrowseDigitKeyIgnoredWhileTopicPickerOpen(t *testing.T) {
	m, _ := bootBrowse(t)
	m.setFocus(RegionList)
	updated, _ := m.openTopicPicker()
	m = updated.(*BrowseModel)
	if m.topicPicker == nil {
		t.Fatal("precondition: topic picker should be open")
	}
	step(&m, keyRune('1'))
	if m.focus != RegionList {
		t.Errorf("digit keys shouldn't jump focus while the topic picker is open, focus=%v", m.focus)
	}
}

func TestBrowseDigitKeySwitchesTheVisiblePaneInNarrowSingleLayout(t *testing.T) {
	f := newFakeData()
	m := NewBrowseModel(f)
	// Narrow enough to force ComputeBrowse's Single (one-pane-at-a-time) mode.
	step(&m, tea.WindowSizeMsg{Width: 40, Height: 20})
	step(&m, browseLoadedMsg{rows: f.rows})
	step(&m, plansLoadedMsg{plans: f.plans})
	if !m.layout.Single {
		t.Fatal("precondition: narrow terminal should trigger Single layout")
	}
	m.setFocus(RegionList)
	step(&m, keyRune('4'))
	if m.focus != RegionDetail {
		t.Errorf("digit-jump should switch the visible pane in Single layout, focus=%v", m.focus)
	}
	if r := m.layout.RectFor(m.focus); r.Empty() {
		t.Error("the newly-focused pane's rect should still be non-empty (it fills the whole work area in Single mode)")
	}
}

func TestBrowseDigitKeySwitchesFocusWhileZoomed(t *testing.T) {
	m, _ := bootBrowse(t)
	m.setFocus(RegionList)
	m.zoom = true
	m.relayout()
	if !m.layout.Single {
		t.Fatal("precondition: zoom should trigger Single layout")
	}
	step(&m, keyRune('2'))
	if m.focus != RegionSources {
		t.Errorf("digit-jump should work while zoomed, focus=%v", m.focus)
	}
	if !m.zoom {
		t.Error("digit-jump shouldn't itself exit zoom")
	}
}

func TestBrowseDigitKeyOutOfRangeIsANoop(t *testing.T) {
	m, _ := bootBrowse(t)
	m.setFocus(RegionList)
	step(&m, keyRune('9')) // browse only has panes 1-4
	if m.focus != RegionList {
		t.Errorf("an out-of-range digit should be a no-op, focus=%v", m.focus)
	}
}

// --- workspace mode ------------------------------------------------------

func newTestWorkspaceModel(t *testing.T) *WorkspaceModel {
	t.Helper()
	q, _ := leetcode.Fixture("two-sum")
	ws, err := workspace.Scaffold(t.TempDir(), q, "python3")
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewWorkspaceModel(ws, q, "true", false, 400, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

func TestWorkspaceDigitKeyJumpsFocusFromAnyPane(t *testing.T) {
	m := newTestWorkspaceModel(t)
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40}) // wide: two-column layout
	m = up.(*WorkspaceModel)

	cases := []struct {
		digit rune
		want  Pane
	}{
		{'1', PaneStatement}, {'2', PaneCode}, {'3', PaneResults},
	}
	for _, tc := range cases {
		m.focused = PaneCode
		m.relayout()
		up, _ := m.Update(keyRune(tc.digit))
		m = up.(*WorkspaceModel)
		if m.focused != tc.want {
			t.Errorf("digit %q: focused = %v, want %v", tc.digit, m.focused, tc.want)
		}
	}
}

func TestWorkspaceDigitKeyIgnoredWhileHelpShown(t *testing.T) {
	m := newTestWorkspaceModel(t)
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)
	m.focused = PaneCode
	m.showHelp = true
	up, _ = m.Update(keyRune('1'))
	m = up.(*WorkspaceModel)
	if m.focused != PaneCode {
		t.Errorf("digit keys should be inert while help is showing, focused=%v", m.focused)
	}
}

func TestWorkspaceDigitKeySwitchesWhichPaneIsZoomedWithoutExitingZoom(t *testing.T) {
	m := newTestWorkspaceModel(t)
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)
	m.focused = PaneCode
	m.mode = ModeZoom
	m.relayout()
	if !m.layout.Tabbed {
		t.Fatal("precondition: ModeZoom should force the tabbed layout")
	}

	up, _ = m.Update(keyRune('3'))
	m = up.(*WorkspaceModel)
	if m.focused != PaneResults {
		t.Errorf("digit-jump should switch which pane is zoomed, focused=%v", m.focused)
	}
	if m.mode != ModeZoom {
		t.Error("digit-jump shouldn't exit zoom, just retarget it")
	}
	if !m.layout.Tabbed {
		t.Error("should still be tabbed (zoomed) after the jump")
	}
}

func TestWorkspaceDigitKeyWorksInNarrowTabbedLayout(t *testing.T) {
	m := newTestWorkspaceModel(t)
	up, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20}) // narrow: forces Tabbed
	m = up.(*WorkspaceModel)
	if !m.layout.Tabbed {
		t.Fatal("precondition: narrow terminal should force the tabbed layout")
	}
	m.focused = PaneStatement

	up, _ = m.Update(keyRune('2'))
	m = up.(*WorkspaceModel)
	if m.focused != PaneCode {
		t.Errorf("digit-jump should switch panes in the narrow tabbed layout, focused=%v", m.focused)
	}
}

func TestWorkspaceDigitKeyOutOfRangeIsANoop(t *testing.T) {
	m := newTestWorkspaceModel(t)
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)
	m.focused = PaneCode
	up, _ = m.Update(keyRune('9')) // workspace only has panes 1-3
	m = up.(*WorkspaceModel)
	if m.focused != PaneCode {
		t.Errorf("an out-of-range digit should be a no-op, focused=%v", m.focused)
	}
}

func TestWorkspaceDigitKeyIgnoredWhileHintsOpen(t *testing.T) {
	m := newTestWorkspaceModel(t)
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)
	m.focused = PaneCode
	up, _ = m.openHints()
	m = up.(*WorkspaceModel)
	if !m.showHints {
		t.Fatal("precondition: hints should be open")
	}
	up, _ = m.Update(keyRune('1'))
	m = up.(*WorkspaceModel)
	if m.focused != PaneCode {
		t.Errorf("digit keys shouldn't jump focus while hints are open, focused=%v", m.focused)
	}
}
