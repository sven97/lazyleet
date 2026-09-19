package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// This file backs F1a: click-drag text selection over a viewport.Model-backed
// pane's rendered body (browse RegionDetail, workspace PaneStatement/
// PaneCode/PaneResults, and the Hints panel). See BrowseModel.handleMouse /
// WorkspaceModel.handleMouse for how a drag gesture is detected and routed
// here, and clipboard.go for what happens to the resulting plain text.

// selectionState tracks an in-progress or just-finished click-drag text
// selection over one viewport's rendered body. Coordinates are pane-local
// cells: (0,0) is the body's top-left. They index directly into a rendered
// viewport.View() split on "\n" — that call always yields exactly vp.Height
// lines of vp.Width cells each, so no further translation is needed once a
// screen coordinate has been converted via localCell/clampCell (which handle
// the F2 border offset) or a raw body-relative rect (see WorkspaceModel's
// Hints handling, which has no border to subtract).
type selectionState struct {
	active           bool // true from press until release
	hasSelection     bool // true once a real (non-empty) drag has happened
	anchorX, anchorY int
	curX, curY       int
}

// begin starts a new selection at the pressed cell, discarding any previous
// one (in progress or persisted).
func (s *selectionState) begin(x, y int) {
	*s = selectionState{active: true, anchorX: x, anchorY: y, curX: x, curY: y}
}

// extend updates the drag's current cell while the button is still held.
func (s *selectionState) extend(x, y int) {
	if !s.active {
		return
	}
	s.curX, s.curY = x, y
	if x != s.anchorX || y != s.anchorY {
		s.hasSelection = true
	}
}

// end finalizes the drag at the released cell. A press+release with no
// movement in between (a plain click, e.g. one that's just focusing the
// pane) leaves hasSelection false — it's not treated as a one-cell
// selection, so it doesn't trigger a surprise clipboard copy on every click.
func (s *selectionState) end(x, y int) {
	if !s.active {
		return
	}
	s.curX, s.curY = x, y
	s.active = false
	if x != s.anchorX || y != s.anchorY {
		s.hasSelection = true
	}
}

// clear discards any in-progress or persisted selection. Called whenever the
// pane's content or geometry changes under it (scroll, resize, a fresh
// render) — selectionState's coordinates are only meaningful against the
// exact rendered lines they were captured from.
func (s *selectionState) clear() { *s = selectionState{} }

// bounds returns the selection's start/end in reading order (top-left to
// bottom-right), normalizing a reversed drag (release above or left of the
// anchor) so callers never have to special-case direction.
func (s selectionState) bounds() (startX, startY, endX, endY int) {
	ax, ay, cx, cy := s.anchorX, s.anchorY, s.curX, s.curY
	if ay > cy || (ay == cy && ax > cx) {
		return cx, cy, ax, ay
	}
	return ax, ay, cx, cy
}

// selectedText returns the plain-text, ANSI-stripped content the selection
// covers over lines (a rendered viewport's View(), split on "\n"). Standard
// multi-line text-selection semantics: the first line runs from the start
// column to its end, full middle lines are taken whole, and the last line
// runs from its start to the end column. Each line's trailing padding
// (viewport lines are space-padded out to the pane's width by lipgloss) is
// trimmed, so a selection dragged out to the right margin doesn't copy a
// wall of blank space.
func selectedText(lines []string, s selectionState) string {
	if !s.hasSelection || len(lines) == 0 {
		return ""
	}
	sx, sy, ex, ey := s.bounds()
	sy = clampInt(sy, 0, len(lines)-1)
	ey = clampInt(ey, 0, len(lines)-1)

	out := make([]string, 0, ey-sy+1)
	for row := sy; row <= ey; row++ {
		line := lines[row]
		w := ansi.StringWidth(line)
		from, to := 0, w
		if row == sy {
			from = clampInt(sx, 0, w)
		}
		if row == ey {
			to = clampInt(ex+1, 0, w) // +1: the released cell itself is included
		}
		if from > to {
			from = to
		}
		out = append(out, strings.TrimRight(ansi.Strip(ansi.Cut(line, from, to)), " "))
	}
	return strings.Join(out, "\n")
}

// applySelectionHighlight returns view (a viewport's View() output) with the
// selection's cell range re-rendered in style — a highlight over exactly the
// selected cells, matching normal terminal drag-select. Styling within the
// selected range is flattened to style rather than layered underneath it,
// which is what keeps the highlight visually unambiguous over already-colored
// text (a difficulty badge, a diff line, glamour's own link coloring, ...).
//
// Known limitation: prefix/suffix keep their original ANSI verbatim (as
// intended, for text outside the highlight), but if a selection boundary
// falls in the middle of an OSC 8-wrapped hyperlink's display text (see
// hyperlink/linkifyURLs), the link's zero-width open marker can end up alone
// in prefix with its close marker alone in suffix, with the stripped-and-
// restyled mid in between carrying neither. Terminals that support OSC 8
// tolerate this differently; it doesn't corrupt the visible text or the
// selection's copied bytes (selectedText strips ANSI outright), only the
// clickable region a partially-selected link presents while highlighted.
// Not fixed here — doing so needs OSC-8-span-aware cutting, not just
// cell-width-aware cutting, and no real terminal was available in this
// sandbox to verify a fix against.
func applySelectionHighlight(view string, s selectionState, style lipgloss.Style) string {
	if !s.hasSelection {
		return view
	}
	lines := strings.Split(view, "\n")
	sx, sy, ex, ey := s.bounds()
	sy = clampInt(sy, 0, len(lines)-1)
	ey = clampInt(ey, 0, len(lines)-1)

	for row := sy; row <= ey; row++ {
		line := lines[row]
		w := ansi.StringWidth(line)
		from, to := 0, w
		if row == sy {
			from = clampInt(sx, 0, w)
		}
		if row == ey {
			to = clampInt(ex+1, 0, w)
		}
		if from >= to {
			continue
		}
		prefix := ansi.Cut(line, 0, from)
		mid := ansi.Strip(ansi.Cut(line, from, to))
		suffix := ansi.Cut(line, to, w)
		lines[row] = prefix + style.Render(mid) + suffix
	}
	return strings.Join(lines, "\n")
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// rawLocalCell converts an absolute screen cell (x, y) to a cell local to r,
// with no border offset — used for panes drawn without F2's bordered-frame
// convention (the Hints overlay, see WorkspaceModel.hintsBodyRect). ok is
// false when the cell falls outside r.
func rawLocalCell(x, y int, r Rect) (lx, ly int, ok bool) {
	lx, ly = x-r.X, y-r.Y
	if lx < 0 || ly < 0 || lx >= r.W || ly >= r.H {
		return 0, 0, false
	}
	return lx, ly, true
}

// rawClampCell is rawLocalCell but clamps an out-of-bounds cell to the
// nearest edge of r instead of rejecting it, so a drag that moves past r's
// bounds still extends the selection to that edge rather than freezing it.
func rawClampCell(x, y int, r Rect) (lx, ly int) {
	lx, ly = x-r.X, y-r.Y
	switch {
	case r.W <= 0 || lx < 0:
		lx = 0
	case lx >= r.W:
		lx = r.W - 1
	}
	switch {
	case r.H <= 0 || ly < 0:
		ly = 0
	case ly >= r.H:
		ly = r.H - 1
	}
	return
}

// localCell converts an absolute screen cell (x, y) to a body-local cell
// inside pane rect r — (0,0) is one cell inside r's top-left corner, past the
// border F2's numberedFrame/joinFrame draws on every edge (see innerSize in
// workspace_model.go). ok is false when the cell falls on the border itself
// or outside r entirely, so a click there never starts a selection.
func localCell(x, y int, r Rect) (lx, ly int, ok bool) {
	w, h := innerSize(r)
	return rawLocalCell(x, y, Rect{X: r.X + 1, Y: r.Y + 1, W: w, H: h})
}

// clampCell is localCell but clamps an out-of-body cell to the nearest body
// edge instead of rejecting it, so dragging past a pane's border still
// extends the selection to that edge — matching normal terminal drag-select,
// where the selection follows the cursor even past the window's own bounds.
func clampCell(x, y int, r Rect) (lx, ly int) {
	w, h := innerSize(r)
	return rawClampCell(x, y, Rect{X: r.X + 1, Y: r.Y + 1, W: w, H: h})
}
