package tui

import "testing"

// TestKeyMapShortcutHintsPerPane locks down the workspace status bar's
// context-sensitive hint set: at most 3 hints for the focused pane, plus a
// trailing, always-present `?` help hint (not counted against the 3).
func TestKeyMapShortcutHintsPerPane(t *testing.T) {
	k := DefaultKeyMap()
	cases := []struct {
		pane Pane
		want []hint
	}{
		{
			PaneStatement,
			[]hint{
				{"e", "edit"},
				{"H", "hints"},
				{"z", "zoom"},
				{"?", "help"},
			},
		},
		{
			PaneCode,
			[]hint{
				{"e", "edit"},
				{"r", "run local"},
				{"R", "run @LC"},
				{"?", "help"},
			},
		},
		{
			PaneResults,
			[]hint{
				{"s", "submit"},
				{"i", "import"},
				{"a", "history"},
				{"?", "help"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.pane.String(), func(t *testing.T) {
			got := k.shortcutHints(c.pane)
			assertHints(t, got, c.want)
		})
	}
}

// TestBrowseKeyMapShortcutHintsPerRegion locks down the browse status bar's
// context-sensitive hint set, including the filtering override which must
// keep winning regardless of which region is focused.
func TestBrowseKeyMapShortcutHintsPerRegion(t *testing.T) {
	k := DefaultBrowseKeyMap()
	cases := []struct {
		name      string
		filtering bool
		region    Region
		want      []hint
	}{
		{
			"RegionStatus",
			false,
			RegionStatus,
			[]hint{
				{"s", "sync"},
				{"tab", "next pane"},
				{"z", "zoom"},
				{"?", "help"},
			},
		},
		{
			"RegionSources",
			false,
			RegionSources,
			[]hint{
				{"↑/k ↓/j", "select"},
				{"↵", "open workspace"},
				{"tab", "next pane"},
				{"?", "help"},
			},
		},
		{
			"RegionList",
			false,
			RegionList,
			[]hint{
				{"↵", "open workspace"},
				{"/", "fuzzy filter"},
				{"S", "cycle sort"},
				{"?", "help"},
			},
		},
		{
			"RegionDetail",
			false,
			RegionDetail,
			[]hint{
				{"↑/k ↓/j", "scroll"},
				{"tab", "next pane"},
				{"z", "zoom"},
				{"?", "help"},
			},
		},
		{
			// Filtering overrides everything else, regardless of the
			// (nominally impossible while filtering) focused region.
			"filtering overrides region",
			true,
			RegionList,
			[]hint{
				{"↵", "apply"},
				{"esc", "cancel"},
				{"type", "to filter"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := k.shortcutHints(c.filtering, c.region)
			assertHints(t, got, c.want)
		})
	}
}

// TestShortcutHintsStayWithinBudget asserts the max-3-contextual-hints +
// trailing-help budget holds for every Pane/Region value, not just the cases
// spelled out above — a guardrail against a future hint creeping the bar back
// toward the old fixed long list.
func TestShortcutHintsStayWithinBudget(t *testing.T) {
	wk := DefaultKeyMap()
	for p := Pane(0); p < paneCount; p++ {
		got := wk.shortcutHints(p)
		if len(got) > 4 {
			t.Errorf("KeyMap.shortcutHints(%s) returned %d hints, want <=4 (3 contextual + help): %v", p, len(got), got)
		}
		if last := got[len(got)-1]; last.key != "?" {
			t.Errorf("KeyMap.shortcutHints(%s) last hint = %v, want trailing help (\"?\")", p, last)
		}
	}

	bk := DefaultBrowseKeyMap()
	for r := Region(0); r < regionCount; r++ {
		got := bk.shortcutHints(false, r)
		if len(got) > 4 {
			t.Errorf("BrowseKeyMap.shortcutHints(false, %s) returned %d hints, want <=4 (3 contextual + help): %v", r, len(got), got)
		}
		if last := got[len(got)-1]; last.key != "?" {
			t.Errorf("BrowseKeyMap.shortcutHints(false, %s) last hint = %v, want trailing help (\"?\")", r, last)
		}
	}
}

func assertHints(t *testing.T, got, want []hint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d hints %v, want %d hints %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hint[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
