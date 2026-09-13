package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBrowseRestoresLastSelectionInAllSource(t *testing.T) {
	f := newFakeData()
	f.initSource = "all"
	f.initSlug = "trapping-rain-water" // last row — proves the cursor actually moved
	m := NewBrowseModel(f)

	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, browseLoadedMsg{rows: f.rows})
	step(&m, plansLoadedMsg{plans: f.plans})
	step(&m, positionLoadedMsg{sourceKey: "all", slug: "trapping-rain-water"})

	if got := m.currentSlug(); got != "trapping-rain-water" {
		t.Fatalf("currentSlug() = %q, want the remembered problem", got)
	}
	if m.pendingRestore != nil {
		t.Fatal("pendingRestore should be cleared once applied")
	}
}

func TestBrowseRestoreIsOrderIndependent(t *testing.T) {
	f := newFakeData()
	f.initSource = "all"
	f.initSlug = "trapping-rain-water"
	m := NewBrowseModel(f)

	// positionLoadedMsg arrives before the catalog this time.
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, positionLoadedMsg{sourceKey: "all", slug: "trapping-rain-water"})
	step(&m, browseLoadedMsg{rows: f.rows})
	step(&m, plansLoadedMsg{plans: f.plans})

	if got := m.currentSlug(); got != "trapping-rain-water" {
		t.Fatalf("currentSlug() = %q, want the remembered problem", got)
	}
}

func TestBrowseRestoresLastStudyPlanAndProblem(t *testing.T) {
	f := newFakeData()
	m := NewBrowseModel(f)

	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, positionLoadedMsg{sourceKey: "plan:starter", slug: "two-sum"})
	step(&m, browseLoadedMsg{rows: f.rows})
	if m.pendingRestore == nil {
		t.Fatal("restore must wait for the plan list before it can find \"starter\"")
	}

	step(&m, plansLoadedMsg{plans: f.plans})
	if m.activeSourceKind() != srcPlan {
		t.Fatalf("the plan source should already be active while its slugs load, kind=%d", m.activeSourceKind())
	}
	if m.pendingRestore == nil {
		t.Fatal("restore must wait for that plan's slugs before it can place the cursor")
	}

	step(&m, planSlugsMsg{slug: "starter", slugs: f.planMap["starter"]})
	if got := m.currentSlug(); got != "two-sum" {
		t.Fatalf("currentSlug() = %q, want two-sum", got)
	}
	if m.pendingRestore != nil {
		t.Fatal("pendingRestore should be cleared once applied")
	}
}

func TestBrowseRestoreGivesUpOnAMissingSource(t *testing.T) {
	f := newFakeData()
	f.initSource = "plan:does-not-exist"
	f.initSlug = "two-sum"
	m := NewBrowseModel(f)

	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, positionLoadedMsg{sourceKey: f.initSource, slug: f.initSlug})
	step(&m, browseLoadedMsg{rows: f.rows})
	step(&m, plansLoadedMsg{plans: f.plans})

	if m.pendingRestore != nil {
		t.Fatal("a since-removed source should be given up on, not left pending")
	}
	if m.activeSourceKind() != srcAll {
		t.Fatalf("should fall back to the default All Problems source, got kind=%d", m.activeSourceKind())
	}
}

func TestBrowseSavesPositionOnQuit(t *testing.T) {
	m, f := bootBrowse(t)
	step(&m, tea.KeyMsg{Type: tea.KeyDown}) // cursor -> valid-parentheses

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if f.saveCalls == 0 {
		t.Fatal("quitting should save the current position")
	}
	if f.savedSource != "all" || f.savedSlug != "valid-parentheses" {
		t.Fatalf("saved (%q, %q), want (\"all\", \"valid-parentheses\")", f.savedSource, f.savedSlug)
	}
}

func TestBrowseSavesPositionOnOpen(t *testing.T) {
	m, f := bootBrowse(t)
	step(&m, tea.KeyMsg{Type: tea.KeyDown})
	step(&m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.Chosen != "valid-parentheses" {
		t.Fatalf("Chosen = %q", m.Chosen)
	}
	if f.savedSlug != "valid-parentheses" {
		t.Fatalf("opening a problem should save it as the last position, got %q", f.savedSlug)
	}
}

func TestBrowseSavesPositionOnDebounceSettle(t *testing.T) {
	m, f := bootBrowse(t)
	step(&m, tea.KeyMsg{Type: tea.KeyDown})
	step(&m, previewDebounceMsg{slug: m.currentSlug()})

	if f.savedSource != "all" || f.savedSlug != "valid-parentheses" {
		t.Fatalf("saved (%q, %q), want (\"all\", \"valid-parentheses\")", f.savedSource, f.savedSlug)
	}
}
