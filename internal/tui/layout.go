package tui

// Pane identifies a workspace pane.
type Pane int

const (
	PaneStatement Pane = iota
	PaneCode
	PaneResults
	paneCount
)

func (p Pane) String() string {
	switch p {
	case PaneStatement:
		return "Statement"
	case PaneCode:
		return "Code"
	case PaneResults:
		return "Results"
	default:
		return "?"
	}
}

// Next / Prev cycle through the panes.
func (p Pane) Next() Pane { return (p + 1) % paneCount }
func (p Pane) Prev() Pane { return (p + paneCount - 1) % paneCount }

// Number is the pane's 1-based digit-key/border-title number: PaneStatement=1
// … PaneResults=3.
func (p Pane) Number() int { return int(p) + 1 }

// paneForNumber is Number's inverse: the pane a pressed digit jumps to (or,
// while zoomed, switches the zoom to), or ok=false if n isn't a valid pane
// number in workspace mode.
func paneForNumber(n int) (p Pane, ok bool) {
	if n < 1 || n > int(paneCount) {
		return 0, false
	}
	return Pane(n - 1), true
}

// ScreenMode is the workspace's overall layout mode (cf. lazygit screen modes).
type ScreenMode int

const (
	ModeNormal ScreenMode = iota // both columns visible (wide) or tabbed (narrow)
	ModeZoom                     // the focused pane fills the working area
)

// Rect is an inclusive-origin, exclusive-extent rectangle in terminal cells.
type Rect struct{ X, Y, W, H int }

// Empty reports whether the rect has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Contains reports whether the 0-indexed cell (x, y) lies inside the rect.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Layout is the solved workspace layout: Statement fills the left column;
// Code (top) and Results (bottom) split the right column.
type Layout struct {
	Statement Rect
	Code      Rect
	Results   Rect
	Status    Rect // one-row shortcut/status bar along the bottom

	// Tabbed is true when the working area shows only the focused pane (narrow
	// terminal or ModeZoom); Statement/Code/Results then all equal the working
	// area and the caller renders just Focused.
	Tabbed  bool
	Focused Pane
}

// RectFor returns the rectangle for a given pane.
func (l Layout) RectFor(p Pane) Rect {
	switch p {
	case PaneStatement:
		return l.Statement
	case PaneCode:
		return l.Code
	case PaneResults:
		return l.Results
	default:
		return Rect{}
	}
}

const (
	statusBarHeight  = 1
	minWorkingHeight = 3

	wsLeftPct       = 44 // Statement column width, % of terminal width
	wsCodePct       = 60 // Code height, % of the right column
	wsMinColWidth   = 26
	wsMinPaneHeight = 4
	wsMinTwoColW    = 2*wsMinColWidth + 2
)

// Compute solves the workspace layout. Pure function of terminal size, focused
// pane, and screen mode.
func Compute(termW, termH int, focused Pane, mode ScreenMode) Layout {
	if termW < 1 {
		termW = 1
	}
	if termH < 1 {
		termH = 1
	}

	l := Layout{Focused: focused}

	workH := termH - statusBarHeight
	if workH < minWorkingHeight {
		workH = termH
		l.Status = Rect{}
	} else {
		l.Status = Rect{X: 0, Y: workH, W: termW, H: statusBarHeight}
	}
	work := Rect{X: 0, Y: 0, W: termW, H: workH}

	if mode == ModeZoom || termW < wsMinTwoColW || workH < 2*wsMinPaneHeight {
		l.Tabbed = true
		l.Statement, l.Code, l.Results = work, work, work
		return l
	}

	leftW := termW * wsLeftPct / 100
	if leftW < wsMinColWidth {
		leftW = wsMinColWidth
	}
	if termW-leftW < wsMinColWidth {
		leftW = termW - wsMinColWidth
	}
	rightW := termW - leftW

	codeH := workH * wsCodePct / 100
	if codeH < wsMinPaneHeight {
		codeH = wsMinPaneHeight
	}
	if workH-codeH < wsMinPaneHeight {
		codeH = workH - wsMinPaneHeight
	}

	l.Statement = Rect{X: 0, Y: 0, W: leftW, H: workH}
	l.Code = Rect{X: leftW, Y: 0, W: rightW, H: codeH}
	l.Results = Rect{X: leftW, Y: codeH, W: rightW, H: workH - codeH}
	return l
}
