package tui

import (
	"sort"
	"strings"
)

// browse list sort modes (cycled with the Sort key).
const (
	sortByID    = iota // frontend id (for "All"), or plan order (for a plan)
	sortByACAsc        // acceptance % ascending (hardest-accepted first)
	sortByACDesc
	sortByDiffAsc
	sortModeCount
)

func sortLabel(m int) string {
	switch m {
	case sortByACAsc:
		return "AC%↑"
	case sortByACDesc:
		return "AC%↓"
	case sortByDiffAsc:
		return "difficulty"
	default:
		return "" // sortByID — the natural order, no chip
	}
}

// listFilter is the browse list's structured (non-fuzzy) filter.
type listFilter struct {
	difficulty string // "", "Easy", "Medium", "Hard"
	status     string // "", "ac", "unsolved", "attempted"
	hidePaid   bool
}

func (f listFilter) chips() []string {
	var c []string
	if f.difficulty != "" {
		c = append(c, f.difficulty)
	}
	switch f.status {
	case "ac":
		c = append(c, "solved")
	case "unsolved":
		c = append(c, "unsolved")
	case "attempted":
		c = append(c, "attempted")
	}
	if f.hidePaid {
		c = append(c, "no-paid")
	}
	return c
}

func cycleDifficulty(cur string) string {
	switch cur {
	case "":
		return "Easy"
	case "Easy":
		return "Medium"
	case "Medium":
		return "Hard"
	default:
		return ""
	}
}

func cycleStatus(cur string) string {
	switch cur {
	case "":
		return "unsolved"
	case "unsolved":
		return "ac"
	case "ac":
		return "attempted"
	default:
		return ""
	}
}

func diffRank(d string) int {
	switch d {
	case "Easy":
		return 0
	case "Medium":
		return 1
	case "Hard":
		return 2
	default:
		return 3
	}
}

// applyListFilterSort returns a filtered, sorted copy of rows. The input order
// is preserved for sortByID.
func applyListFilterSort(rows []BrowseRow, f listFilter, sortMode int) []BrowseRow {
	out := make([]BrowseRow, 0, len(rows))
	for _, r := range rows {
		if f.difficulty != "" && r.Difficulty != f.difficulty {
			continue
		}
		if f.hidePaid && r.PaidOnly {
			continue
		}
		switch f.status {
		case "ac":
			if r.Status != "ac" {
				continue
			}
		case "unsolved":
			if r.Status == "ac" {
				continue
			}
		case "attempted":
			if r.Status != "notac" {
				continue
			}
		}
		out = append(out, r)
	}

	switch sortMode {
	case sortByACAsc:
		sort.SliceStable(out, func(i, j int) bool { return out[i].ACRate < out[j].ACRate })
	case sortByACDesc:
		sort.SliceStable(out, func(i, j int) bool { return out[i].ACRate > out[j].ACRate })
	case sortByDiffAsc:
		sort.SliceStable(out, func(i, j int) bool {
			if di, dj := diffRank(out[i].Difficulty), diffRank(out[j].Difficulty); di != dj {
				return di < dj
			}
			return out[i].FrontendID < out[j].FrontendID
		})
	}
	return out
}

// filterSummary renders the active filter + sort as a compact suffix.
func (m *BrowseModel) filterSummary() string {
	parts := m.listFilterState().chips()
	if s := sortLabel(m.sortMode); s != "" {
		parts = append(parts, "↕"+s)
	}
	if len(parts) == 0 {
		return ""
	}
	return "· " + strings.Join(parts, " · ")
}

func (m *BrowseModel) listFilterState() listFilter {
	return listFilter{difficulty: m.fltDiff, status: m.fltStatus, hidePaid: m.fltHidePaid}
}

func (m *BrowseModel) listFilterDirty() bool {
	return m.fltDiff != "" || m.fltStatus != "" || m.fltHidePaid || m.sortMode != sortByID
}
