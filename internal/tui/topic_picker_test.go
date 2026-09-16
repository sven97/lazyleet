package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func topicRune(m *BrowseModel, s string) { m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }

func TestTopicFilterCombinesAllTagsWithOtherFilters(t *testing.T) {
	rows := fixtureRows()
	rows[0].Tags = []string{"array", "hash-table"}
	rows[1].Tags = []string{"array"}
	rows[2].Tags = []string{"hash-table"}
	rows[3].Tags = []string{"array", "hash-table"}
	got := applyListFilterSort(rows, listFilter{tags: []string{"array", "hash-table"}}, sortByACAsc)
	if !equalStrs(slugsOf(got), []string{"d", "a"}) {
		t.Fatalf("all-tag match + sort: %v", slugsOf(got))
	}
	got = applyListFilterSort(rows, listFilter{tags: []string{"array"}, difficulty: "Medium", status: "unsolved", hidePaid: true}, sortByID)
	if !equalStrs(slugsOf(got), []string{"b", "d"}) {
		t.Fatalf("combined filters: %v", slugsOf(got))
	}
	if len(applyListFilterSort(rows, listFilter{tags: []string{"unknown"}}, sortByID)) != 0 {
		t.Fatal("unknown tag matched")
	}
	if !equalStrs(slugsOf(rows), []string{"a", "b", "c", "d"}) {
		t.Fatal("input rows mutated")
	}
}

func TestTopicPickerDraftApplyCancelAndClear(t *testing.T) {
	m, f := bootBrowse(t)
	f.rows[0].Tags = []string{"array", "hash-table"}
	f.rows[1].Tags = []string{"stack"}
	f.rows[2].Tags = []string{"array", "two-pointers"}
	step(&m, browseLoadedMsg{rows: f.rows})
	topicRune(m, "t")
	if m.topicPicker == nil {
		t.Fatal("picker not opened")
	}
	topicRune(m, "array")
	topicRune(m, " ")
	if len(m.fltTags) != 0 || len(m.filtered) != 3 {
		t.Fatal("draft changed live filters")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.topicPicker != nil || !equalStrs(m.fltTags, []string{"array"}) || len(m.filtered) != 2 {
		t.Fatal("tag selection did not apply")
	}
	if !strings.Contains(m.filterSummary(), "array") {
		t.Fatal("active tag not displayed")
	}
	topicRune(m, "t")
	topicRune(m, "hash")
	topicRune(m, " ")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !equalStrs(m.fltTags, []string{"array"}) {
		t.Fatal("cancel changed applied tags")
	}
	topicRune(m, "t")
	topicRune(m, "hash")
	topicRune(m, " ")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.filtered) != 1 || m.currentSlug() != "two-sum" {
		t.Fatal("multiple tags did not combine")
	}
	topicRune(m, "c")
	if len(m.fltTags) != 0 || len(m.filtered) != 3 || m.listFilterDirty() {
		t.Fatal("clear did not reset tags")
	}
}

func TestTopicFilterWorksWithinPlanDailyAndFuzzySearch(t *testing.T) {
	m, f := bootBrowse(t)
	f.rows[0].Tags = []string{"array"}
	f.rows[1].Tags = []string{"stack"}
	step(&m, browseLoadedMsg{rows: f.rows})
	m.fltTags = []string{"array"}
	drain(&m, m.activateSource(2))
	if len(m.filtered) != 1 || m.currentSlug() != "two-sum" {
		t.Fatal("plan did not respect tag filter")
	}
	m.filter.SetValue("parentheses")
	m.applyFilter()
	if len(m.filtered) != 0 {
		t.Fatal("fuzzy search ignored tag filter")
	}
	m.filter.SetValue("")
	drain(&m, m.activateSource(1))
	if len(m.filtered) != 0 {
		t.Fatal("daily source ignored tag filter")
	}
	m.fltTags = []string{"stack"}
	m.rebuildView()
	if len(m.filtered) != 1 || m.currentSlug() != "valid-parentheses" {
		t.Fatal("daily match missing")
	}
}

func TestTopicPickerHandlesEmptySearchSyncAndResize(t *testing.T) {
	m, f := bootBrowse(t)
	for i := range f.rows {
		f.rows[i].Tags = nil
	}
	step(&m, browseLoadedMsg{rows: f.rows})
	topicRune(m, "t")
	if !strings.Contains(ansi.Strip(m.View()), "No cached topics") {
		t.Fatal("empty cache not explained")
	}
	topicRune(m, " ") // empty list must be safe
	topicRune(m, "zzzz")
	f.rows[0].Tags = []string{"array", "array", "hash-table"}
	step(&m, browseLoadedMsg{rows: f.rows})
	if !strings.Contains(ansi.Strip(m.View()), "No matching topics") {
		t.Fatal("empty search not explained")
	}
	m.topicPicker.search.SetValue("")
	if m.topicPicker.choices[0].count != 1 {
		t.Fatal("duplicate tags inflated count")
	}
	topicRune(m, "q")
	if m.topicPicker == nil || m.topicPicker.search.Value() != "q" {
		t.Fatal("search key leaked to quit")
	}
	m.topicPicker.search.SetValue("")
	for _, size := range [][2]int{{40, 14}, {80, 24}, {120, 40}} {
		step(&m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("picker exceeds %v", size)
		}
		if !strings.Contains(ansi.Strip(view), "enter apply") {
			t.Fatal("apply control not visible")
		}
	}
}

func TestRemovedTagsRemainRemovable(t *testing.T) {
	m, _ := bootBrowse(t)
	m.fltTags = []string{"vanished"}
	topicRune(m, "t")
	topicRune(m, "vanished")
	choices := m.topicPicker.matches()
	if len(choices) != 1 || choices[0].count != 0 {
		t.Fatal("applied missing tag disappeared")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.fltTags) != 0 {
		t.Fatal("clear draft did not remove missing tag")
	}
}
