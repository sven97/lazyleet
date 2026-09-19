package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestSelectionStateBoundsNormalizesReversedDrag(t *testing.T) {
	var s selectionState
	s.begin(10, 3)
	s.extend(2, 1) // released above and left of the anchor
	sx, sy, ex, ey := s.bounds()
	if sx != 2 || sy != 1 || ex != 10 || ey != 3 {
		t.Fatalf("bounds() = (%d,%d)-(%d,%d), want (2,1)-(10,3)", sx, sy, ex, ey)
	}
}

func TestSelectionStateBoundsForwardDragUnchanged(t *testing.T) {
	var s selectionState
	s.begin(1, 1)
	s.extend(8, 4)
	sx, sy, ex, ey := s.bounds()
	if sx != 1 || sy != 1 || ex != 8 || ey != 4 {
		t.Fatalf("bounds() = (%d,%d)-(%d,%d), want (1,1)-(8,4)", sx, sy, ex, ey)
	}
}

func TestSelectionStatePlainClickHasNoSelection(t *testing.T) {
	var s selectionState
	s.begin(5, 2)
	s.end(5, 2) // released at the same cell: no drag happened
	if s.hasSelection {
		t.Fatal("a press+release with no movement should not count as a selection")
	}
	if s.active {
		t.Fatal("end() should always clear active, even for a no-op click")
	}
}

func TestSelectionStateDragHasSelection(t *testing.T) {
	var s selectionState
	s.begin(5, 2)
	s.extend(9, 2)
	s.end(9, 2)
	if !s.hasSelection {
		t.Fatal("a drag that moved should count as a selection")
	}
}

func TestSelectionStateClear(t *testing.T) {
	var s selectionState
	s.begin(1, 1)
	s.extend(4, 4)
	s.clear()
	if s.active || s.hasSelection {
		t.Fatalf("clear() should reset everything, got %+v", s)
	}
	// extend/end on a cleared (inactive) selection must be a no-op.
	s.extend(9, 9)
	if s.curX != 0 || s.curY != 0 {
		t.Fatalf("extend() on an inactive selection should be a no-op, got cur=(%d,%d)", s.curX, s.curY)
	}
}

func TestSelectedTextSingleLine(t *testing.T) {
	lines := []string{"hello world"}
	var s selectionState
	s.begin(0, 0)
	s.extend(4, 0) // covers "hello" (columns 0-4 inclusive)
	got := selectedText(lines, s)
	if got != "hello" {
		t.Fatalf("selectedText() = %q, want %q", got, "hello")
	}
}

func TestSelectedTextMultiLine(t *testing.T) {
	// Standard multi-line selection semantics: first line from the start
	// column to its end, full middle lines, last line from its start to the
	// end column.
	lines := []string{
		"abcdefghij", // row 0
		"klmnopqrst", // row 1 (full row selected)
		"uvwxyzABCD", // row 2
	}
	var s selectionState
	s.begin(5, 0)  // "fghij"
	s.extend(2, 2) // through "uvw"
	got := selectedText(lines, s)
	want := "fghij\nklmnopqrst\nuvw"
	if got != want {
		t.Fatalf("selectedText() =\n%q\nwant\n%q", got, want)
	}
}

func TestSelectedTextReversedDrag(t *testing.T) {
	// Releasing above/left of the anchor must produce the same text as the
	// equivalent forward drag.
	lines := []string{
		"abcdefghij",
		"klmnopqrst",
		"uvwxyzABCD",
	}
	var s selectionState
	s.begin(2, 2)  // anchor at the end point
	s.extend(5, 0) // release at the start point
	got := selectedText(lines, s)
	want := "fghij\nklmnopqrst\nuvw"
	if got != want {
		t.Fatalf("reversed-drag selectedText() =\n%q\nwant\n%q", got, want)
	}
}

func TestSelectedTextTrimsTrailingPadding(t *testing.T) {
	// viewport lines are space-padded out to the pane width; a selection
	// dragged to the right margin shouldn't copy that padding.
	lines := []string{"hi        "} // "hi" + 8 spaces, 10 cells wide
	var s selectionState
	s.begin(0, 0)
	s.extend(9, 0) // drag all the way to the last column
	got := selectedText(lines, s)
	if got != "hi" {
		t.Fatalf("selectedText() = %q, want %q (trailing padding trimmed)", got, "hi")
	}
}

func TestSelectedTextStripsANSI(t *testing.T) {
	styled := "\x1b[1;31mred\x1b[0m plain"
	lines := []string{styled}
	var s selectionState
	s.begin(0, 0)
	s.extend(ansi.StringWidth(styled)-1, 0)
	got := selectedText(lines, s)
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("selectedText() should strip ANSI, got %q", got)
	}
	if got != "red plain" {
		t.Fatalf("selectedText() = %q, want %q", got, "red plain")
	}
}

func TestSelectedTextNoSelectionIsEmpty(t *testing.T) {
	var s selectionState
	if got := selectedText([]string{"abc"}, s); got != "" {
		t.Fatalf("selectedText() with no selection = %q, want empty", got)
	}
}

func TestApplySelectionHighlightWrapsOnlySelectedRange(t *testing.T) {
	withColorProfile(t, termenv.ANSI)
	var s selectionState
	s.begin(0, 0)
	s.extend(2, 0) // "abcd"[0:3] -> "abc"
	style := DefaultTheme().Selection
	out := applySelectionHighlight("abcd", s, style)

	plain := ansi.Strip(out)
	if plain != "abcd" {
		t.Fatalf("highlight must not change the visible text, got %q", plain)
	}
	// Reverse video (SGR 7) should appear somewhere in the highlighted part,
	// and the unselected "d" should not carry it.
	if !strings.Contains(out, "\x1b[7m") && !strings.Contains(out, ";7m") {
		t.Errorf("expected reverse-video SGR in highlighted output, got %q", out)
	}
	idxD := strings.Index(out, "d")
	idxReset := strings.LastIndex(out[:idxD], "\x1b[0m")
	if idxReset == -1 {
		t.Errorf("expected the highlight style to be reset before the unselected suffix, got %q", out)
	}
}

func TestApplySelectionHighlightNoSelectionReturnsUnchanged(t *testing.T) {
	var s selectionState
	view := "unchanged line"
	if got := applySelectionHighlight(view, s, DefaultTheme().Selection); got != view {
		t.Fatalf("applySelectionHighlight() with no selection = %q, want unchanged %q", got, view)
	}
}

func TestLocalCellAccountsForF2Border(t *testing.T) {
	// r mimics a numberedFrame pane: one cell of border on every edge (see
	// frame.go/innerSize) — screen cell (r.X+1, r.Y+1) is body-local (0,0).
	r := Rect{X: 10, Y: 5, W: 20, H: 10}

	lx, ly, ok := localCell(r.X+1, r.Y+1, r)
	if !ok || lx != 0 || ly != 0 {
		t.Fatalf("localCell(top-left body cell) = (%d,%d,%v), want (0,0,true)", lx, ly, ok)
	}
	// The border cell itself is outside the body.
	if _, _, ok := localCell(r.X, r.Y, r); ok {
		t.Fatal("localCell() on the border row/column should be ok=false")
	}
	// A cell right past the far edge of the body (into the right/bottom
	// border) should also be rejected.
	w, h := innerSize(r)
	if _, _, ok := localCell(r.X+1+w, r.Y+1, r); ok {
		t.Fatal("localCell() past the body's right edge should be ok=false")
	}
	if _, _, ok := localCell(r.X+1, r.Y+1+h, r); ok {
		t.Fatal("localCell() past the body's bottom edge should be ok=false")
	}
}

func TestClampCellClampsOutOfBoundsToNearestEdge(t *testing.T) {
	r := Rect{X: 10, Y: 5, W: 20, H: 10}
	w, h := innerSize(r)

	// Far above/left of the pane clamps to (0,0).
	lx, ly := clampCell(0, 0, r)
	if lx != 0 || ly != 0 {
		t.Fatalf("clampCell(far above-left) = (%d,%d), want (0,0)", lx, ly)
	}
	// Far below/right of the pane clamps to the last body cell.
	lx, ly = clampCell(1000, 1000, r)
	if lx != w-1 || ly != h-1 {
		t.Fatalf("clampCell(far below-right) = (%d,%d), want (%d,%d)", lx, ly, w-1, h-1)
	}
}
