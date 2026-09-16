package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type fakeBrowseData struct {
	rows           []BrowseRow
	plans          []PlanRef
	planMap        map[string][]string
	stmts          map[string]string
	synced         int
	authed         bool
	user           string
	progressCalls  int
	progressSolved int
	progressSync   time.Time
	daily          DailyInfo
	dailyErr       error
	dailyCalls     int

	// position persistence: initSource/initSlug seed what LoadPosition returns
	// (simulating a remembered previous session); saved* record what the model
	// writes back via SavePosition.
	initSource  string
	initSlug    string
	savedSource string
	savedSlug   string
	saveCalls   int
}

func (f *fakeBrowseData) ListProblems(context.Context) ([]BrowseRow, error) { return f.rows, nil }
func (f *fakeBrowseData) Auth(context.Context) AuthState {
	return AuthState{Authed: f.authed, Region: "com"}
}
func (f *fakeBrowseData) CurrentUser(context.Context) (string, error) { return f.user, nil }
func (f *fakeBrowseData) LastSync(context.Context) (time.Time, bool) {
	return time.Now().Add(-2 * time.Hour), true
}
func (f *fakeBrowseData) ProgressLastSync(context.Context) (time.Time, bool) {
	return f.progressSync, !f.progressSync.IsZero()
}
func (f *fakeBrowseData) Sync(context.Context) (int, error) { f.synced++; return len(f.rows), nil }
func (f *fakeBrowseData) SyncProgress(context.Context) (int, error) {
	f.progressCalls++
	return f.progressSolved, nil
}
func (f *fakeBrowseData) LoadStatement(_ context.Context, slug string) (string, error) {
	return f.stmts[slug], nil
}
func (f *fakeBrowseData) Plans(context.Context) ([]PlanRef, error) { return f.plans, nil }
func (f *fakeBrowseData) PlanSlugs(_ context.Context, ref PlanRef) ([]string, error) {
	return f.planMap[ref.Slug], nil
}
func (f *fakeBrowseData) Daily(context.Context) (DailyInfo, error) {
	f.dailyCalls++
	return f.daily, f.dailyErr
}
func (f *fakeBrowseData) LoadPosition(context.Context) (string, string) {
	return f.initSource, f.initSlug
}
func (f *fakeBrowseData) SavePosition(_ context.Context, sourceKey, slug string) error {
	f.saveCalls++
	f.savedSource, f.savedSlug = sourceKey, slug
	return nil
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
		daily: DailyInfo{
			Date: "2026-09-08", Slug: "valid-parentheses", Title: "Valid Parentheses",
			Difficulty: "Easy", Done: false, Streak: 3,
		},
	}
}

func bootBrowse(t *testing.T) (*BrowseModel, *fakeBrowseData) {
	t.Helper()
	f := newFakeData()
	m := NewBrowseModel(f)
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, browseLoadedMsg{rows: f.rows})
	step(&m, plansLoadedMsg{plans: f.plans})
	step(&m, dailyLoadedMsg{info: f.daily})
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
	step(&m, tea.KeyMsg{Type: tea.KeyDown}) // past "Daily Question"
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

func TestBrowseViewNeverOverflowsTerminal(t *testing.T) {
	f := newFakeData()
	f.rows = nil
	for i := 1; i <= 400; i++ {
		f.rows = append(f.rows, BrowseRow{FrontendID: i, Slug: "p", Title: "Problem Title", Difficulty: "Easy"})
	}
	for _, h := range []int{24, 30, 45, 60, 90} {
		m := NewBrowseModel(f)
		step(&m, tea.WindowSizeMsg{Width: 150, Height: h})
		step(&m, browseLoadedMsg{rows: f.rows})
		step(&m, plansLoadedMsg{plans: f.plans})
		if got := lipgloss.Height(m.View()); got != h {
			t.Errorf("termH=%d: View() rendered %d lines, want exactly %d (a taller view pushes panes off-screen)", h, got, h)
		}
	}
}

func TestBrowseAutoSyncsProgressWhenSignedIn(t *testing.T) {
	f := newFakeData()
	f.authed = true
	f.progressSolved = 7
	m := NewBrowseModel(f)
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})

	// auth lands first, rows not loaded yet -> must not fire
	step(&m, authLoadedMsg{a: AuthState{Authed: true}})
	if f.progressCalls != 0 {
		t.Fatalf("progress sync fired before the catalog loaded (%d calls)", f.progressCalls)
	}

	updated, cmd := m.Update(browseLoadedMsg{rows: f.rows})
	m = updated.(*BrowseModel)
	if !m.progressing {
		t.Fatal("expected a background progress sync once rows + auth are ready")
	}
	drain(&m, cmd)
	if f.progressCalls != 1 {
		t.Fatalf("SyncProgress called %d times, want 1", f.progressCalls)
	}
	if m.progressing {
		t.Fatal("progressing should clear after progressDoneMsg")
	}

	// one-shot: a later reload must not re-trigger it
	step(&m, browseLoadedMsg{rows: f.rows})
	if f.progressCalls != 1 {
		t.Fatalf("progress sync should be one-shot, got %d calls", f.progressCalls)
	}
}

func TestBrowseSkipsProgressSyncWhenAnonymous(t *testing.T) {
	m, f := bootBrowse(t) // fake Auth() is anonymous by default
	step(&m, authLoadedMsg{a: AuthState{Authed: false}})
	if f.progressCalls != 0 {
		t.Fatalf("anonymous session must not sync progress, got %d calls", f.progressCalls)
	}
}

func TestBrowseDailySourceShowsOnlyTodaysQuestion(t *testing.T) {
	m, _ := bootBrowse(t) // sources: All(0) Daily(1) Starter(2)
	if m.sources[1].kind != srcDaily {
		t.Fatalf("expected a Daily Question source at index 1, got kind %d", m.sources[1].kind)
	}
	drain(&m, m.activateSource(1))
	if m.activeSourceKind() != srcDaily {
		t.Fatalf("activeSourceKind = %d, want srcDaily", m.activeSourceKind())
	}
	if len(m.view) != 1 || m.view[0].Slug != "valid-parentheses" {
		t.Fatalf("daily list = %+v, want just today's question", m.view)
	}
	// the catalog row is preferred, so real metadata comes through
	if m.view[0].Title != "Valid Parentheses" {
		t.Fatalf("daily row title = %q", m.view[0].Title)
	}
}

func TestBrowseStatusShowsDailyStreak(t *testing.T) {
	m, _ := bootBrowse(t)
	v := m.View()
	if !strings.Contains(v, "daily") || !strings.Contains(v, "streak 3") {
		t.Fatalf("status pane should show the daily streak:\n%s", v)
	}

	// completing it flips the marker
	step(&m, dailyLoadedMsg{info: DailyInfo{
		Date: "2026-09-08", Slug: "valid-parentheses", Title: "Valid Parentheses",
		Difficulty: "Easy", Done: true, Streak: 4,
	}})
	if !strings.Contains(m.dailyStatusLine(40), "done") {
		t.Fatalf("daily line should read done: %q", m.dailyStatusLine(40))
	}
}

func TestStatusDetailBodyShowsAccountSyncsAndDaily(t *testing.T) {
	f := newFakeData()
	f.authed = true
	f.user = "sven"
	f.rows[0].Status = "ac" // 1 of 3 solved
	f.progressSync = time.Now().Add(-3 * time.Hour)

	m := NewBrowseModel(f)
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, authLoadedMsg{a: AuthState{Authed: true}})
	step(&m, userLoadedMsg{name: "sven"})
	step(&m, browseLoadedMsg{
		rows:         f.rows,
		lastSync:     time.Now().Add(-26 * time.Hour),
		progressSync: f.progressSync,
	})
	step(&m, dailyLoadedMsg{info: DailyInfo{
		Date:       time.Now().Format("2006-01-02"),
		FrontendID: 3157,
		Slug:       "count-nodes-equal-to-average-of-subtree",
		Title:      "Count Nodes Equal to Average of Subtree",
		Difficulty: "Medium", Done: true, Streak: 3,
	}})

	body := m.statusDetailBody()
	for _, want := range []string{
		"Account", "sven",
		"Catalog", "1d ago", // full-catalog freshness
		"Progress", "1 / 3", "3h ago", // solve status + its own freshness
		"Daily", "3157. Count Nodes Equal to Average of Subtree",
		"Medium", "streak 3 days",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("statusDetailBody missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "auth.json") || strings.Contains(body, "lazyleet.db") {
		t.Errorf("full cache-file paths should be gone:\n%s", body)
	}
}

func TestBrowseStatusPaneLeadsWithSolvedCountWhenAuthed(t *testing.T) {
	f := newFakeData()
	f.authed = true
	f.rows[0].Status = "ac" // 1 of 3 solved

	m := NewBrowseModel(f)
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, authLoadedMsg{a: AuthState{Authed: true, Region: "com"}})
	step(&m, browseLoadedMsg{rows: f.rows, lastSync: time.Now().Add(-2 * time.Hour)})

	got := m.statusPaneBody(40)
	if !strings.Contains(got, "1/3 solved") {
		t.Errorf("authed status pane should lead with solved count, got:\n%s", got)
	}
	if strings.Contains(got, "com") {
		t.Errorf("the default region shouldn't take up space in the compact pane:\n%s", got)
	}
}

func TestBrowseStatusPaneFlagsStaleSync(t *testing.T) {
	m, f := bootBrowse(t) // anonymous
	step(&m, browseLoadedMsg{rows: f.rows, lastSync: time.Now().Add(-48 * time.Hour)})

	got := m.statusPaneBody(40)
	if !strings.Contains(got, "press s") {
		t.Errorf("a multi-day-old catalog sync should nudge a resync:\n%s", got)
	}

	fresh, ff := bootBrowse(t)
	step(&fresh, browseLoadedMsg{rows: ff.rows, lastSync: time.Now().Add(-2 * time.Hour)})
	if got := fresh.statusPaneBody(40); strings.Contains(got, "press s") {
		t.Errorf("a fresh sync shouldn't be flagged:\n%s", got)
	}
}

func TestBrowseFooterOmitsSyncAgeWhenStatusPaneVisible(t *testing.T) {
	m, f := bootBrowse(t) // wide window -> two-column layout, Status pane on screen
	step(&m, browseLoadedMsg{rows: f.rows, lastSync: time.Now().Add(-48 * time.Hour)})
	if m.layout.Single {
		t.Fatal("a 150-wide window should use the two-column layout")
	}

	if bar := m.renderStatusBar(); strings.Contains(bar, "synced") {
		t.Errorf("footer shouldn't repeat the Status pane's own sync info:\n%s", bar)
	}

	// Zoom onto the list: the Status pane is off-screen, so the footer is the
	// only place left to see sync freshness — it should show up there instead.
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	if !m.layout.Single {
		t.Fatal("z should zoom to a single-pane layout")
	}
	bar := m.renderStatusBar()
	if !strings.Contains(bar, "synced") {
		t.Errorf("footer should show sync info once the Status pane is zoomed away:\n%s", bar)
	}
	if !strings.Contains(bar, "2d ago") {
		t.Errorf("footer sync age wrong:\n%s", bar)
	}
}

func TestStatusDetailBodyAnonymous(t *testing.T) {
	m, _ := bootBrowse(t) // anonymous
	body := m.statusDetailBody()
	if !strings.Contains(body, "run `lazyleet auth`") {
		t.Errorf("anonymous status should prompt to sign in:\n%s", body)
	}
	if !strings.Contains(body, "sign in to track") {
		t.Errorf("Progress section should tell an anonymous user to sign in:\n%s", body)
	}
}

func TestBrowseDailyUnavailableIsSurfaced(t *testing.T) {
	f := newFakeData()
	f.dailyErr = context.DeadlineExceeded
	m := NewBrowseModel(f)
	step(&m, tea.WindowSizeMsg{Width: 150, Height: 40})
	step(&m, browseLoadedMsg{rows: f.rows})
	step(&m, plansLoadedMsg{plans: f.plans})
	step(&m, dailyLoadedMsg{info: DailyInfo{}, err: f.dailyErr})

	drain(&m, m.activateSource(1))
	if got := m.emptyListReason(); !strings.Contains(got, "unavailable") {
		t.Fatalf("emptyListReason = %q, want an 'unavailable' message", got)
	}
}

func TestExpiredSessionAndVerificationFailure(t *testing.T) {
	m, _ := bootBrowse(t)
	m.auth = AuthState{Authed: true, User: "old-user"}
	step(&m, userLoadedMsg{err: context.DeadlineExceeded})
	if !m.auth.Authed || !strings.Contains(m.statusMsg, "could not verify") {
		t.Fatal("network failure misreported as logout")
	}
	step(&m, userLoadedMsg{})
	if m.auth.Authed || m.auth.User != "" || !strings.Contains(m.statusMsg, "lazyleet auth") {
		t.Fatal("expired session not cleared with recovery guidance")
	}
}

func TestProgressReloadPreservesSelection(t *testing.T) {
	m, f := bootBrowse(t)
	m.selectSlug("valid-parentheses")
	rows := append([]BrowseRow(nil), f.rows...)
	rows[1].Status = "ac"
	step(&m, browseLoadedMsg{rows: rows})
	if m.currentSlug() != "valid-parentheses" {
		t.Fatal("refresh moved selection")
	}
	row, _ := m.currentRow()
	if !row.Solved() {
		t.Fatal("refresh did not update solve indicator")
	}
}

func TestAnonymousProgressFilterExplainsSignIn(t *testing.T) {
	m, _ := bootBrowse(t)
	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !strings.Contains(m.statusMsg, "lazyleet auth") || !strings.Contains(m.statusMsg, "press s") {
		t.Fatal("missing sign-in and refresh guidance")
	}
}

func TestManualSyncReloadsAccountAndDaily(t *testing.T) {
	m, f := bootBrowse(t)
	f.authed, f.user = true, "new-user"
	f.daily.Done, f.daily.Streak = true, 4
	_, cmd := m.Update(syncDoneMsg{count: len(f.rows)})
	drain(&m, cmd)
	if !m.auth.Authed {
		t.Fatal("sync did not pick up new credentials")
	}
	if !m.daily.Done || m.daily.Streak != 4 {
		t.Fatalf("daily not refreshed: %+v", m.daily)
	}
}
