package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type fakeBrowseData struct {
	rows    []BrowseRow
	plans   []PlanRef
	planMap map[string][]string
	stmts   map[string]string
	synced  int
}

func (f *fakeBrowseData) ListProblems(context.Context) ([]BrowseRow, error) { return f.rows, nil }
func (f *fakeBrowseData) Auth(context.Context) AuthState {
	return AuthState{Authed: false, Region: "com"}
}
func (f *fakeBrowseData) LastSync(context.Context) (time.Time, bool) {
	return time.Now().Add(-2 * time.Hour), true
}
func (f *fakeBrowseData) Sync(context.Context) (int, error) { f.synced++; return len(f.rows), nil }
func (f *fakeBrowseData) LoadStatement(_ context.Context, slug string) (string, error) {
	return f.stmts[slug], nil
}
func (f *fakeBrowseData) Plans(context.Context) ([]PlanRef, error) { return f.plans, nil }
func (f *fakeBrowseData) PlanSlugs(_ context.Context, ref PlanRef) ([]string, error) {
	return f.planMap[ref.Slug], nil
}

func newFakeData() *fakeBrowseData {
	return &fakeBrowseData{
		rows: []BrowseRow{
			{FrontendID: 1, Slug: "two-sum", Title: "Two Sum", Difficulty: "Easy", ACRate: 52, Status: "ac", Tags: []string{"array"}},
			{FrontendID: 20, Slug: "valid-parentheses", Title: "Valid Parentheses", Difficulty: "Easy", ACRate: 41},
			{FrontendID: 42, Slug: "trapping-rain-water", Title: "Trapping Rain Water", Difficulty: "Hard", ACRate: 61},
		},
		plans:   []PlanRef{{Slug: "starter", Name: "Starter", Official: false}},
		planMap: map[string][]string{"starter": {"valid-parentheses", "two-sum"}},
		stmts:   map[string]string{"two-sum": "# Two Sum\n\nGiven an array.", "valid-parentheses": "# Valid Parentheses"},
	}
}

func bootBrowse(t *testing.T) (*BrowseModel, *fakeBrowseData) {
	t.Helper()
	f := newFakeData()
	m := NewBrowseModel(f)
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, browseLoadedMsg{rows: f.rows})
	step(&m, plansLoadedMsg{plans: f.plans})
	return m, f
}

func step(m **BrowseModel, msg tea.Msg) {
	updated, _ := (*m).Update(msg)
	*m = updated.(*BrowseModel)
}

// drain runs a command (recursing into tea.Batch) and feeds each resulting
// message back into the model. Time-based cmds (tea.Tick) return nil here.
func drain(m **BrowseModel, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch v := msg.(type) {
	case tea.BatchMsg:
		for _, c := range v {
			drain(m, c)
		}
	case nil:
	default:
		step(m, msg)
	}
}

func TestBrowseRendersProblemRows(t *testing.T) {
	m, _ := bootBrowse(t)
	v := m.View()
	for _, want := range []string{"Two Sum", "Valid Parentheses", "Trapping Rain Water", "Problems"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q\n%s", want, v)
		}
	}
}

func TestBrowseCursorAndOpen(t *testing.T) {
	m, _ := bootBrowse(t)
	if got := m.currentSlug(); got != "two-sum" {
		t.Fatalf("initial slug = %q", got)
	}
	step(&m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.currentSlug(); got != "valid-parentheses" {
		t.Fatalf("after Down slug = %q", got)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*BrowseModel)
	if m.Chosen != "valid-parentheses" {
		t.Fatalf("Chosen = %q, want valid-parentheses", m.Chosen)
	}
	if cmd == nil {
		t.Fatal("expected a quit command from Open")
	}
}

func TestBrowseFuzzyFilter(t *testing.T) {
	m, _ := bootBrowse(t)
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.filtering {
		t.Fatal("`/` should enter filtering mode")
	}
	for _, r := range "rain" {
		step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.filtered) != 1 || m.currentSlug() != "trapping-rain-water" {
		t.Fatalf("filter 'rain' -> %d rows, slug %q", len(m.filtered), m.currentSlug())
	}
	step(&m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.filtering || len(m.filtered) != 3 {
		t.Fatalf("esc should clear filter: filtering=%v rows=%d", m.filtering, len(m.filtered))
	}
}

func TestBrowseSelectPlanReordersAndCounts(t *testing.T) {
	m, f := bootBrowse(t)
	// focus the Sources pane (list -> sources is one shift+tab), move to the plan, open it
	step(&m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focus != RegionSources {
		t.Fatalf("focus = %v, want Sources", m.focus)
	}
	step(&m, tea.KeyMsg{Type: tea.KeyDown}) // to "Starter"
	step(&m, planSlugsMsg{slug: "starter", slugs: f.planMap["starter"]})
	step(&m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.view) != 2 || m.view[0].Slug != "valid-parentheses" {
		t.Fatalf("plan view = %+v, want plan order", m.view)
	}
	solved, total := m.planProgress()
	if solved != 1 || total != 2 {
		t.Fatalf("progress = %d/%d, want 1/2", solved, total)
	}
}

func TestBrowseStructuredFilterAndSort(t *testing.T) {
	m, _ := bootBrowse(t)
	// fixture rows: two-sum(Easy,ac), valid-parentheses(Easy), trapping-rain-water(Hard)
	if len(m.view) != 3 {
		t.Fatalf("start with %d rows", len(m.view))
	}

	// d -> Easy
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.fltDiff != "Easy" || len(m.view) != 2 {
		t.Fatalf("after d: diff=%q rows=%d", m.fltDiff, len(m.view))
	}
	// d d -> Medium, Hard  (cycle to Hard)
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.fltDiff != "Hard" || len(m.view) != 1 || m.view[0].Slug != "trapping-rain-water" {
		t.Fatalf("after d×3: diff=%q view=%v", m.fltDiff, m.view)
	}
	// c -> clear
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.listFilterDirty() || len(m.view) != 3 {
		t.Fatalf("after c: dirty=%v rows=%d", m.listFilterDirty(), len(m.view))
	}

	// f -> unsolved
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.fltStatus != "unsolved" {
		t.Fatalf("after f: status=%q", m.fltStatus)
	}
	for _, r := range m.view {
		if r.Status == "ac" {
			t.Fatalf("solved row %s leaked into unsolved filter", r.Slug)
		}
	}

	// S -> cycle sort off default
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	if m.sortMode == sortByID {
		t.Fatal("S should advance the sort mode")
	}
}

func TestBrowsePreviewShowsLoadingUntilCurrentRendered(t *testing.T) {
	m, _ := bootBrowse(t)

	// fresh: nothing rendered for the selected row yet
	if !strings.Contains(m.detailBody(), "loading") {
		t.Fatalf("expected a loading state before any statement is rendered:\n%s", m.detailBody())
	}

	// deliver + render the statement for the current row
	m.previewSlug = m.currentSlug()
	updated, cmd := m.Update(statementMsg{slug: m.currentSlug(), md: "# Two Sum\n\nGiven an array."})
	m = updated.(*BrowseModel)
	runBrowseCmd(&m, cmd) // refreshPreviewContent -> previewRenderedMsg

	if m.previewContentSlug != "two-sum" {
		t.Fatalf("previewContentSlug = %q, want two-sum", m.previewContentSlug)
	}
	if strings.Contains(m.detailBody(), "loading") {
		t.Fatalf("current statement rendered; should not show loading:\n%s", m.detailBody())
	}

	// navigate to another row — preview must not keep showing the old one
	step(&m, tea.KeyMsg{Type: tea.KeyDown})
	if !strings.Contains(m.detailBody(), "loading") {
		t.Fatalf("after moving to an unrendered row, expected loading, got:\n%s", m.detailBody())
	}
}

// runBrowseCmd executes a browse command tree, feeding results back into the
// model (spinner ticks dropped).
func runBrowseCmd(m **BrowseModel, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch v := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range v {
			runBrowseCmd(m, c)
		}
	case spinner.TickMsg, nil:
	default:
		updated, next := (*m).Update(v)
		*m = updated.(*BrowseModel)
		runBrowseCmd(m, next)
	}
}

func TestBrowseSyncFlow(t *testing.T) {
	f := newFakeData()
	f.rows = nil
	m := NewBrowseModel(f)
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, browseLoadedMsg{rows: nil})
	if !strings.Contains(m.View(), "sync") {
		t.Errorf("empty cache view should mention sync:\n%s", m.View())
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(*BrowseModel)
	if !m.syncing {
		t.Fatal("`s` should set syncing")
	}
	if cmd == nil {
		t.Fatal("`s` should return a sync command")
	}
	drain(&m, cmd) // executes the batched cmd tree, incl. syncCmd
	if f.synced != 1 {
		t.Fatalf("Sync called %d times, want 1", f.synced)
	}
	if m.syncing {
		t.Fatal("syncing should clear after syncDoneMsg")
	}
}
