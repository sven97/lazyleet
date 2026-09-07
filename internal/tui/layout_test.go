package tui

import "testing"

func TestComputeThreeColumnWidthsSumAndMinimums(t *testing.T) {
	for _, termW := range []int{minThreeColWidth, 80, 100, 120, 200, 240} {
		l := Compute(termW, 40, PaneCode, ModeNormal)
		if l.Tabbed {
			t.Fatalf("termW=%d: unexpected tabbed layout", termW)
		}
		sum := l.Statement.W + l.Code.W + l.Results.W
		if sum != termW {
			t.Errorf("termW=%d: column widths sum to %d", termW, sum)
		}
		for _, r := range []Rect{l.Statement, l.Code, l.Results} {
			if r.W < minPaneWidth {
				t.Errorf("termW=%d: pane width %d < min %d", termW, r.W, minPaneWidth)
			}
		}
		if l.Statement.X != 0 || l.Code.X != l.Statement.W || l.Results.X != l.Statement.W+l.Code.W {
			t.Errorf("termW=%d: panes not laid left-to-right: %+v", termW, l)
		}
	}
}

func TestComputeReservesStatusBar(t *testing.T) {
	l := Compute(120, 40, PaneCode, ModeNormal)
	if l.Status.Y != 39 || l.Status.W != 120 || l.Status.H != 1 {
		t.Errorf("status bar = %+v, want {Y:39 W:120 H:1}", l.Status)
	}
	if l.Code.H != 39 {
		t.Errorf("pane height = %d, want 39 (termH - status bar)", l.Code.H)
	}
}

func TestComputeNarrowFallsBackToTabbed(t *testing.T) {
	l := Compute(minThreeColWidth-1, 40, PaneStatement, ModeNormal)
	if !l.Tabbed {
		t.Fatal("narrow terminal should use the tabbed layout")
	}
	if l.Statement != l.Code || l.Code != l.Results {
		t.Error("tabbed layout should give every pane the same rect")
	}
}

func TestComputeZoomIsTabbedEvenWhenWide(t *testing.T) {
	if !Compute(200, 50, PaneResults, ModeZoom).Tabbed {
		t.Fatal("ModeZoom should always be tabbed")
	}
}

func TestComputeTinyTerminalDoesNotPanic(t *testing.T) {
	for _, d := range [][2]int{{0, 0}, {1, 1}, {5, 2}, {200, 1}} {
		_ = Compute(d[0], d[1], PaneCode, ModeNormal)
	}
}

func TestPaneCycle(t *testing.T) {
	if PaneStatement.Next() != PaneCode || PaneResults.Next() != PaneStatement {
		t.Error("Next() should wrap Results -> Statement")
	}
	if PaneStatement.Prev() != PaneResults {
		t.Error("Prev() should wrap Statement -> Results")
	}
}
