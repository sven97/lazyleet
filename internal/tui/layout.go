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

// ScreenMode is the workspace's overall layout mode (cf. lazygit screen modes).
type ScreenMode int

const (
	ModeNormal ScreenMode = iota // all panes visible (wide) or tabbed (narrow)
	ModeZoom                     // the focused pane fills the working area
)

// Rect is an inclusive-origin, exclusive-extent rectangle in terminal cells.
type Rect struct{ X, Y, W, H int }

// Empty reports whether the rect has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Layout is the result of solving the workspace layout for a terminal size.
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

// Rect returns the rectangle for a given pane.
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

// Layout thresholds. Below minThreeColWidth the three-column layout can't give
// each pane a usable width, so we fall back to tabs.
const (
	minPaneWidth     = 24
	minThreeColWidth = 3*minPaneWidth + 2 // + inter-pane gutters
	statusBarHeight  = 1
	minWorkingHeight = 3
	statementWeight  = 32
	codeWeight       = 40
	resultsWeight    = 28
)

// Compute solves the workspace layout. It is a pure function of the terminal
// size, the focused pane, and the screen mode — geometry never depends on
// scroll position, selection, or content.
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
		workH = termH // too short for a status bar; give everything to the panes
		l.Status = Rect{}
	} else {
		l.Status = Rect{X: 0, Y: workH, W: termW, H: statusBarHeight}
	}

	work := Rect{X: 0, Y: 0, W: termW, H: workH}

	if mode == ModeZoom || termW < minThreeColWidth {
		l.Tabbed = true
		l.Statement, l.Code, l.Results = work, work, work
		return l
	}

	// Three columns split by weight, with integer remainder handed to the
	// left-most columns so the widths sum exactly to termW.
	total := statementWeight + codeWeight + resultsWeight
	wStmt := termW * statementWeight / total
	wCode := termW * codeWeight / total
	wRes := termW - wStmt - wCode

	// Guarantee each column its minimum, stealing from the widest neighbour.
	wStmt, wCode, wRes = enforceMinimums(wStmt, wCode, wRes, termW)

	l.Statement = Rect{X: 0, Y: 0, W: wStmt, H: workH}
	l.Code = Rect{X: wStmt, Y: 0, W: wCode, H: workH}
	l.Results = Rect{X: wStmt + wCode, Y: 0, W: wRes, H: workH}
	return l
}

func enforceMinimums(a, b, c, total int) (int, int, int) {
	widths := []int{a, b, c}
	for i := range widths {
		if widths[i] >= minPaneWidth {
			continue
		}
		need := minPaneWidth - widths[i]
		// take from the largest other column
		j := largestOther(widths, i)
		if widths[j]-need < minPaneWidth {
			need = widths[j] - minPaneWidth
		}
		widths[i] += need
		widths[j] -= need
	}
	// fix any rounding drift
	if sum := widths[0] + widths[1] + widths[2]; sum != total {
		widths[1] += total - sum
	}
	return widths[0], widths[1], widths[2]
}

func largestOther(w []int, skip int) int {
	best := -1
	for i := range w {
		if i == skip {
			continue
		}
		if best == -1 || w[i] > w[best] {
			best = i
		}
	}
	return best
}
