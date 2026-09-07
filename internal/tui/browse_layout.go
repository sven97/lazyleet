package tui

// Region identifies a browse-mode pane.
type Region int

const (
	RegionSidebar Region = iota
	RegionList
	RegionPreview
	regionCount
)

func (r Region) String() string {
	switch r {
	case RegionSidebar:
		return "Sources"
	case RegionList:
		return "Problems"
	case RegionPreview:
		return "Preview"
	default:
		return "?"
	}
}

// next / prev cycle only through the regions currently visible.
func (r Region) next(showSidebar, showPreview bool) Region {
	for i := 0; i < int(regionCount); i++ {
		r = (r + 1) % regionCount
		if (r != RegionSidebar || showSidebar) && (r != RegionPreview || showPreview) {
			return r
		}
	}
	return r
}

func (r Region) prev(showSidebar, showPreview bool) Region {
	for i := 0; i < int(regionCount); i++ {
		r = (r + regionCount - 1) % regionCount
		if (r != RegionSidebar || showSidebar) && (r != RegionPreview || showPreview) {
			return r
		}
	}
	return r
}

// BrowseLayout is the solved geometry for browse mode.
type BrowseLayout struct {
	Sidebar Rect
	List    Rect
	Preview Rect
	Status  Rect

	ShowSidebar bool
	ShowPreview bool
	Focused     Region
}

// browse layout thresholds
const (
	sidebarMin       = 18
	sidebarMax       = 30
	listMin          = 34
	previewMin       = 40
	dropPreviewBelow = listMin + previewMin // total width under this loses the preview
	dropSidebarBelow = listMin + 6
)

// ComputeBrowse solves the browse-mode layout. Pure function of size, focus,
// and zoom.
func ComputeBrowse(termW, termH int, focused Region, zoom bool) BrowseLayout {
	if termW < 1 {
		termW = 1
	}
	if termH < 1 {
		termH = 1
	}

	l := BrowseLayout{Focused: focused, ShowSidebar: true, ShowPreview: true}

	workH := termH - statusBarHeight
	if workH < minWorkingHeight {
		workH = termH
		l.Status = Rect{}
	} else {
		l.Status = Rect{X: 0, Y: workH, W: termW, H: statusBarHeight}
	}

	if zoom {
		full := Rect{X: 0, Y: 0, W: termW, H: workH}
		l.Sidebar, l.List, l.Preview = full, full, full
		// keep Show* true so region cycling still works; caller renders Focused
		return l
	}

	// Decide which optional panes fit.
	if termW < dropPreviewBelow {
		l.ShowPreview = false
	}
	if termW < dropSidebarBelow {
		l.ShowSidebar = false
	}
	if focused == RegionPreview && !l.ShowPreview {
		l.Focused = RegionList
	}
	if focused == RegionSidebar && !l.ShowSidebar {
		l.Focused = RegionList
	}

	x := 0
	rem := termW

	if l.ShowSidebar {
		sw := termW / 5
		if sw < sidebarMin {
			sw = sidebarMin
		}
		if sw > sidebarMax {
			sw = sidebarMax
		}
		l.Sidebar = Rect{X: 0, Y: 0, W: sw, H: workH}
		x += sw
		rem -= sw
	}

	if l.ShowPreview {
		pw := rem * 2 / 5
		if pw < previewMin {
			pw = previewMin
		}
		if rem-pw < listMin {
			pw = rem - listMin
		}
		l.List = Rect{X: x, Y: 0, W: rem - pw, H: workH}
		l.Preview = Rect{X: x + (rem - pw), Y: 0, W: pw, H: workH}
	} else {
		l.List = Rect{X: x, Y: 0, W: rem, H: workH}
	}
	return l
}
