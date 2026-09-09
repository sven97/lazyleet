package tui

// Region identifies a browse-mode pane. The left column stacks Status, Sources
// and List top-to-bottom; the right column is a single Detail pane.
type Region int

const (
	RegionStatus Region = iota
	RegionSources
	RegionList
	RegionDetail
	regionCount
)

func (r Region) String() string {
	switch r {
	case RegionStatus:
		return "Status"
	case RegionSources:
		return "Sources"
	case RegionList:
		return "Problems"
	case RegionDetail:
		return "Detail"
	default:
		return "?"
	}
}

func (r Region) next() Region { return (r + 1) % regionCount }
func (r Region) prev() Region { return (r + regionCount - 1) % regionCount }

// BrowseLayout is the solved geometry for browse mode.
type BrowseLayout struct {
	Status  Rect // left column, top — auth / cache summary
	Sources Rect // left column, middle — All Problems + study plans
	List    Rect // left column, bottom — the problem list
	Detail  Rect // right column — statement, or status detail

	Footer  Rect // one-row shortcut bar along the bottom
	Focused Region
	Single  bool // narrow terminal or zoom: render only the focused pane
}

// RectFor returns the rectangle for a region.
func (l BrowseLayout) RectFor(r Region) Rect {
	switch r {
	case RegionStatus:
		return l.Status
	case RegionSources:
		return l.Sources
	case RegionList:
		return l.List
	case RegionDetail:
		return l.Detail
	default:
		return Rect{}
	}
}

const (
	brLeftPct     = 38 // left column width, % of terminal width
	brLeftMin     = 28
	brLeftMax     = 56
	brRightMin    = 30
	brStatusH     = 6 // border(2) + title(1) + 3 body lines (auth · cache · daily)
	brSourcesMaxH = 14
	brListMinH    = 5
	brMinTwoColW  = brLeftMin + brRightMin + 2
)

// ComputeBrowse solves the browse-mode layout. sourcesRows is how many rows the
// Sources pane wants for its content (used to size that pane). Pure function.
func ComputeBrowse(termW, termH int, focused Region, zoom bool, sourcesRows int) BrowseLayout {
	if termW < 1 {
		termW = 1
	}
	if termH < 1 {
		termH = 1
	}

	l := BrowseLayout{Focused: focused}

	workH := termH - statusBarHeight
	if workH < minWorkingHeight {
		workH = termH
		l.Footer = Rect{}
	} else {
		l.Footer = Rect{X: 0, Y: workH, W: termW, H: statusBarHeight}
	}
	work := Rect{X: 0, Y: 0, W: termW, H: workH}

	if zoom || termW < brMinTwoColW || workH < brStatusH+8 {
		l.Single = true
		l.Status, l.Sources, l.List, l.Detail = work, work, work, work
		return l
	}

	leftW := termW * brLeftPct / 100
	if leftW < brLeftMin {
		leftW = brLeftMin
	}
	if leftW > brLeftMax {
		leftW = brLeftMax
	}
	if termW-leftW < brRightMin {
		leftW = termW - brRightMin
	}
	rightW := termW - leftW

	const srcMinH = 6 // title + border + at least 3 body lines

	statusH := brStatusH
	srcH := sourcesRows + 3 // + title + border
	if srcH > brSourcesMaxH {
		srcH = brSourcesMaxH
	}
	if srcH < srcMinH {
		srcH = srcMinH
	}

	// Guarantee the list a usable height, shrinking Sources then Status.
	if listH := workH - statusH - srcH; listH < brListMinH {
		deficit := brListMinH - listH
		if take := min(deficit, srcH-srcMinH); take > 0 {
			srcH -= take
			deficit -= take
		}
		if deficit > 0 {
			statusH -= deficit
			if statusH < 3 {
				statusH = 3
			}
		}
	}
	listH := workH - statusH - srcH
	if listH < 1 {
		listH = 1
	}

	l.Status = Rect{X: 0, Y: 0, W: leftW, H: statusH}
	l.Sources = Rect{X: 0, Y: statusH, W: leftW, H: srcH}
	l.List = Rect{X: 0, Y: statusH + srcH, W: leftW, H: listH}
	l.Detail = Rect{X: leftW, Y: 0, W: rightW, H: workH}
	return l
}
