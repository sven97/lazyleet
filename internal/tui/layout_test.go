package tui

import "testing"

func TestComputeTwoColumnGeometry(t *testing.T) {
	for _, termW := range []int{wsMinTwoColW, 80, 100, 120, 200, 240} {
		l := Compute(termW, 40, PaneCode, ModeNormal)
		if l.Tabbed {
			t.Fatalf("termW=%d: unexpected tabbed layout", termW)
		}
		// Statement is the whole left column; Code+Results split the right column.
		if l.Statement.X != 0 || l.Statement.Y != 0 {
			t.Errorf("termW=%d: statement not top-left: %+v", termW, l.Statement)
		}
		if l.Code.X != l.Statement.W || l.Results.X != l.Statement.W {
			t.Errorf("termW=%d: right column X mismatch: %+v", termW, l)
		}
		if l.Statement.W+l.Code.W != termW {
			t.Errorf("termW=%d: columns don't sum to width: %d+%d", termW, l.Statement.W, l.Code.W)
		}
		if l.Code.W != l.Results.W {
			t.Errorf("termW=%d: code/results widths differ: %d vs %d", termW, l.Code.W, l.Results.W)
		}
		if l.Results.Y != l.Code.H || l.Code.H+l.Results.H != l.Statement.H {
			t.Errorf("termW=%d: right column heights don't stack: %+v", termW, l)
		}
		if l.Statement.W < wsMinColWidth || l.Code.W < wsMinColWidth {
			t.Errorf("termW=%d: a column is under the minimum width", termW)
		}
	}
}

func TestComputeReservesStatusBar(t *testing.T) {
	l := Compute(120, 40, PaneCode, ModeNormal)
	if l.Status.Y != 39 || l.Status.W != 120 || l.Status.H != 1 {
		t.Errorf("status bar = %+v, want {Y:39 W:120 H:1}", l.Status)
	}
	if l.Statement.H != 39 {
		t.Errorf("statement height = %d, want 39 (termH - status bar)", l.Statement.H)
	}
}

func TestComputeNarrowFallsBackToTabbed(t *testing.T) {
	l := Compute(wsMinTwoColW-1, 40, PaneStatement, ModeNormal)
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
