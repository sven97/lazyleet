package tui

import "testing"

func fixtureRows() []BrowseRow {
	return []BrowseRow{
		{FrontendID: 1, Slug: "a", Title: "A", Difficulty: "Easy", ACRate: 55, Status: "ac"},
		{FrontendID: 2, Slug: "b", Title: "B", Difficulty: "Medium", ACRate: 40, Status: "notac"},
		{FrontendID: 3, Slug: "c", Title: "C", Difficulty: "Hard", ACRate: 30, PaidOnly: true},
		{FrontendID: 4, Slug: "d", Title: "D", Difficulty: "Medium", ACRate: 48, Status: ""},
	}
}

func slugsOf(rows []BrowseRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Slug
	}
	return out
}

func TestApplyListFilterSort_Difficulty(t *testing.T) {
	got := applyListFilterSort(fixtureRows(), listFilter{difficulty: "Medium"}, sortByID)
	if s := slugsOf(got); len(s) != 2 || s[0] != "b" || s[1] != "d" {
		t.Fatalf("Medium filter -> %v", s)
	}
}

func TestApplyListFilterSort_Status(t *testing.T) {
	solved := applyListFilterSort(fixtureRows(), listFilter{status: "ac"}, sortByID)
	if s := slugsOf(solved); len(s) != 1 || s[0] != "a" {
		t.Fatalf("solved -> %v", s)
	}
	unsolved := applyListFilterSort(fixtureRows(), listFilter{status: "unsolved"}, sortByID)
	if s := slugsOf(unsolved); len(s) != 3 || s[0] != "b" {
		t.Fatalf("unsolved -> %v", s)
	}
	attempted := applyListFilterSort(fixtureRows(), listFilter{status: "attempted"}, sortByID)
	if s := slugsOf(attempted); len(s) != 1 || s[0] != "b" {
		t.Fatalf("attempted -> %v", s)
	}
}

func TestApplyListFilterSort_HidePaid(t *testing.T) {
	got := applyListFilterSort(fixtureRows(), listFilter{hidePaid: true}, sortByID)
	for _, r := range got {
		if r.PaidOnly {
			t.Fatalf("paid row leaked: %+v", r)
		}
	}
	if len(got) != 3 {
		t.Fatalf("hidePaid -> %d rows, want 3", len(got))
	}
}

func TestApplyListFilterSort_Sort(t *testing.T) {
	asc := slugsOf(applyListFilterSort(fixtureRows(), listFilter{}, sortByACAsc))
	if asc[0] != "c" || asc[len(asc)-1] != "a" {
		t.Fatalf("AC%% asc -> %v", asc)
	}
	desc := slugsOf(applyListFilterSort(fixtureRows(), listFilter{}, sortByACDesc))
	if desc[0] != "a" || desc[len(desc)-1] != "c" {
		t.Fatalf("AC%% desc -> %v", desc)
	}
	byDiff := slugsOf(applyListFilterSort(fixtureRows(), listFilter{}, sortByDiffAsc))
	// Easy(a), Medium(b,d by id), Hard(c)
	if want := []string{"a", "b", "d", "c"}; !equalStrs(byDiff, want) {
		t.Fatalf("by difficulty -> %v, want %v", byDiff, want)
	}
}

func TestApplyListFilterSort_DoesNotMutateInput(t *testing.T) {
	rows := fixtureRows()
	_ = applyListFilterSort(rows, listFilter{}, sortByACDesc)
	if rows[0].Slug != "a" || rows[3].Slug != "d" {
		t.Fatalf("input slice was reordered: %v", slugsOf(rows))
	}
}

func TestCycleHelpers(t *testing.T) {
	if cycleDifficulty("") != "Easy" || cycleDifficulty("Hard") != "" {
		t.Error("cycleDifficulty wrap")
	}
	if cycleStatus("") != "unsolved" || cycleStatus("attempted") != "" {
		t.Error("cycleStatus wrap")
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
