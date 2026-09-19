package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"

	"github.com/sven97/lazyleet/internal/termimg"
)

const (
	srcAll = iota
	srcDaily
	srcPlan
)

type sourceItem struct {
	label string
	kind  int
	plan  PlanRef
}

// BrowseModel is the Bubble Tea model for browse mode: a sources sidebar, a
// fuzzy-filterable problem list, and a statement preview. Selecting a problem
// with Enter sets Chosen and either quits (standalone) or emits OpenProblemMsg
// (when embedded in AppModel).
type BrowseModel struct {
	data BrowseData
	th   Theme
	keys BrowseKeyMap

	width, height int
	ready         bool
	layout        BrowseLayout
	focus         Region
	// detailShowsStatus is true when the right-hand Detail pane holds the Status
	// pane's expanded info rather than a problem statement. It follows the last
	// left-column pane the user focused; focusing Detail itself (to scroll it)
	// leaves it alone, so clicking into the status text to read it doesn't swap
	// in a problem statement.
	detailShowsStatus bool
	zoom              bool
	showHelp          bool

	allRows     []BrowseRow
	sources     []sourceItem
	srcCursor   int
	activeSrc   int
	planCache   map[string][]string
	plansLoaded bool // plansLoadedMsg has landed at least once

	// pendingRestore is the source+problem remembered from the previous
	// session; set once from positionLoadedMsg, cleared once applied (or given
	// up on) by attemptRestore. See attemptRestore for the whole flow.
	pendingRestore *browseRestore

	view     []BrowseRow
	haystack []string
	filtered []int
	cursor   int
	top      int

	// structured list filter + sort (fuzzy filter is `filter` below)
	fltDiff     string
	fltStatus   string
	fltHidePaid bool
	fltTags     []string
	topicPicker *topicPicker
	sortMode    int

	filtering bool
	filter    textinput.Model

	previewSlug string
	previewMD   string
	previewVP   viewport.Model
	// detailSel is an in-progress or just-finished click-drag text selection
	// (F1) over the Detail pane's body — see selection.go. Cleared whenever
	// the pane's content or geometry changes under it.
	detailSel          selectionState
	previewErr         error
	previewLoading     bool
	previewImages      *statementImages
	previewCancel      context.CancelFunc // cancels the in-flight statement/image load
	previewContentSlug string             // slug whose statement is currently in the viewport
	// rendered preview bodies keyed by slug|width|hasImages (glamour is
	// slow; keep re-visits and re-renders instant)
	renderCache map[string]previewEntry
	// previewRenderedKey/Body identify the content currently sitting in
	// previewVP, so a refresh triggered by an unrelated background reload
	// (auth/daily/etc.) doesn't yank the scroll position back to the top when
	// the displayed content hasn't actually changed.
	previewRenderedKey  string
	previewRenderedBody string

	imgProto   termimg.Protocol
	imgDir     string
	imgWriter  *ImageWriter
	imgWritten string

	// version is main.versionString(), plumbed in via SetVersion so the
	// package-main-only build info can appear in the Status/About panel
	// without internal/tui importing cmd/lazyleet.
	version string

	auth AuthState

	daily       DailyInfo
	dailyErr    error
	dailyLoaded bool

	spin         spinner.Model
	syncing      bool
	autoSynced   bool // guards the one-shot progress sync on browse open
	progressing  bool // a background SyncProgress is in flight
	awaitSignIn  bool // auth just flipped to true; confirm once loadUser resolves
	statusMsg    string
	loadErr      error
	lastSync     time.Time // full problem catalog
	progressSync time.Time // signed-in user's solve status

	// Chosen is the slug the user opened, set just before tea.Quit
	// (standalone) or OpenProblemMsg (when embedded in AppModel).
	Chosen string

	// embedded means browse is owned by AppModel: Open emits OpenProblemMsg
	// instead of quitting the program.
	embedded bool
}

type browseLoadedMsg struct {
	rows         []BrowseRow
	lastSync     time.Time
	progressSync time.Time
	err          error
}
type plansLoadedMsg struct{ plans []PlanRef }
type planSlugsMsg struct {
	slug  string
	slugs []string
	err   error
}
type statementMsg struct {
	slug string
	md   string
	err  error
}
type syncDoneMsg struct {
	count int
	err   error
}
type progressDoneMsg struct {
	solved int
	err    error
}
type previewDebounceMsg struct{ slug string }

// NewBrowseModel builds a browse model over the given data source.
func NewBrowseModel(data BrowseData) *BrowseModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "fuzzy filter id or title"

	return &BrowseModel{
		data:        data,
		th:          DefaultTheme(),
		keys:        DefaultBrowseKeyMap(),
		focus:       RegionList,
		filter:      ti,
		spin:        sp,
		planCache:   map[string][]string{},
		renderCache: map[string]previewEntry{},
		sources: []sourceItem{
			{label: "All Problems", kind: srcAll},
			{label: "Daily Question", kind: srcDaily},
		},
	}
}

func (m *BrowseModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.loadProblems(), m.loadPlans(), m.loadAuth(), m.loadDaily(), m.loadPosition())
}

// browseRestore is the source+problem remembered from the previous session.
type browseRestore struct {
	sourceKey string
	slug      string
}

type positionLoadedMsg struct {
	sourceKey string
	slug      string
}

func (m *BrowseModel) loadPosition() tea.Cmd {
	return func() tea.Msg {
		src, slug := m.data.LoadPosition(context.Background())
		return positionLoadedMsg{sourceKey: src, slug: slug}
	}
}

type authLoadedMsg struct{ a AuthState }

func (m *BrowseModel) loadAuth() tea.Cmd {
	return func() tea.Msg { return authLoadedMsg{a: m.data.Auth(context.Background())} }
}

type userLoadedMsg struct {
	name string
	err  error
}

// loadUser resolves the signed-in username in the background; a slow or failing
// request preserves the account state and reports the verification failure.
func (m *BrowseModel) loadUser() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		name, err := m.data.CurrentUser(ctx)
		return userLoadedMsg{name: name, err: err}
	}
}

type dailyLoadedMsg struct {
	info DailyInfo
	err  error
}

func (m *BrowseModel) loadDaily() tea.Cmd {
	return func() tea.Msg {
		info, err := m.data.Daily(context.Background())
		return dailyLoadedMsg{info: info, err: err}
	}
}

// --- commands --------------------------------------------------------------

func (m *BrowseModel) loadProblems() tea.Cmd {
	return func() tea.Msg {
		rows, err := m.data.ListProblems(context.Background())
		t, _ := m.data.LastSync(context.Background())
		pt, _ := m.data.ProgressLastSync(context.Background())
		return browseLoadedMsg{rows: rows, lastSync: t, progressSync: pt, err: err}
	}
}

func (m *BrowseModel) loadPlans() tea.Cmd {
	return func() tea.Msg {
		plans, err := m.data.Plans(context.Background())
		if err != nil {
			return plansLoadedMsg{}
		}
		return plansLoadedMsg{plans: plans}
	}
}

func (m *BrowseModel) loadPlanSlugs(ref PlanRef) tea.Cmd {
	return func() tea.Msg {
		slugs, err := m.data.PlanSlugs(context.Background(), ref)
		return planSlugsMsg{slug: ref.Slug, slugs: slugs, err: err}
	}
}

func (m *BrowseModel) loadStatement(ctx context.Context, slug string) tea.Cmd {
	data := m.data
	return func() tea.Msg {
		md, err := data.LoadStatement(ctx, slug)
		return statementMsg{slug: slug, md: md, err: err}
	}
}

func (m *BrowseModel) syncCmd() tea.Cmd {
	return func() tea.Msg {
		n, err := m.data.Sync(context.Background())
		return syncDoneMsg{count: n, err: err}
	}
}

func (m *BrowseModel) progressCmd() tea.Cmd {
	return func() tea.Msg {
		n, err := m.data.SyncProgress(context.Background())
		return progressDoneMsg{solved: n, err: err}
	}
}

// rearmAutoSync clears the one-shot guard so maybeSyncProgress will fire
// again — used whenever something makes the cached progress worth
// re-checking (an auth-state flip, an explicit sync).
func (m *BrowseModel) rearmAutoSync() {
	m.autoSynced = false
}

// maybeSyncProgress kicks a one-shot background refresh of the signed-in user's
// solve status (a few requests, not the whole catalog) so the ✓ marks and
// per-plan counts reflect problems solved on the web. Runs once per session,
// only when signed in and the problem catalog is already cached. Called from
// both the browseLoadedMsg and authLoadedMsg handlers since either may land
// last.
func (m *BrowseModel) maybeSyncProgress() tea.Cmd {
	if m.autoSynced || m.progressing || m.syncing || !m.auth.Authed || len(m.allRows) == 0 {
		return nil
	}
	m.autoSynced = true
	m.progressing = true
	return tea.Batch(m.progressCmd(), m.spin.Tick)
}

func (m *BrowseModel) debouncePreview() tea.Cmd {
	slug := m.currentSlug()
	if slug == "" {
		return nil
	}
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return previewDebounceMsg{slug: slug}
	})
}

// --- update --------------------------------------------------------------

func (m *BrowseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if debugScroll {
		if _, isTick := msg.(spinner.TickMsg); !isTick {
			scrollLogf("browse Update <- %T  top=%d detailStatus=%v", msg, m.top, m.detailShowsStatus)
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.detailSel.clear() // pane geometry is about to change under it
		if m.topicPicker != nil {
			m.topicPicker.search.Width = max(1, m.width-18)
		}
		prevW := m.previewVP.Width
		m.relayout()
		m.ready = true
		if m.previewVP.Width != prevW {
			m.renderCache = map[string]previewEntry{}
		}
		return m, m.refreshDetail()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case browseLoadedMsg:
		m.loadErr = msg.err
		m.allRows = msg.rows
		m.refreshTopicChoices()
		if msg.err == nil && len(msg.rows) == 0 {
			m.statusMsg = "problem cache is empty — press s to sync"
		}
		if !msg.lastSync.IsZero() {
			m.lastSync = msg.lastSync
		}
		if !msg.progressSync.IsZero() {
			m.progressSync = msg.progressSync
		}
		selected := m.currentSlug()
		m.rebuildView()
		m.selectSlug(selected)
		// attemptRestore may move the cursor (e.g. onto a remembered problem);
		// call it before debouncePreview so the debounce captures where the
		// cursor actually ends up, not row 0.
		restoreCmd := m.attemptRestore()
		return m, tea.Batch(restoreCmd, m.debouncePreview(), m.maybeSyncProgress())

	case positionLoadedMsg:
		if msg.sourceKey != "" {
			m.pendingRestore = &browseRestore{sourceKey: msg.sourceKey, slug: msg.slug}
		}
		restoreCmd := m.attemptRestore()
		return m, tea.Batch(restoreCmd, m.debouncePreview())

	case authLoadedMsg:
		if m.auth.Authed != msg.a.Authed {
			m.rearmAutoSync()
			m.awaitSignIn = msg.a.Authed // false->true edge only
		}
		m.auth = msg.a
		var cmds []tea.Cmd
		if m.auth.Authed {
			cmds = append(cmds, m.loadUser())
		}
		cmds = append(cmds, m.maybeSyncProgress(), m.refreshDetail())
		return m, tea.Batch(cmds...)

	case userLoadedMsg:
		awaitSignIn := m.awaitSignIn
		m.awaitSignIn = false
		if msg.err != nil {
			m.statusMsg = "could not verify session: " + msg.err.Error()
		} else if msg.name == "" && m.auth.Authed {
			m.auth.Authed = false
			m.auth.User = ""
			m.statusMsg = "session expired — run `lazyleet auth`, then press s"
		} else {
			m.auth.User = msg.name
			// The Status pane already shows "✓ <user>" persistently once
			// authed — only echo it here (single-pane/zoomed layout, same
			// condition the footer's sync-age fallback uses) where that pane
			// isn't actually on screen to notice for you.
			if awaitSignIn && m.layout.Single && m.focus != RegionStatus {
				m.statusMsg = "signed in as " + msg.name
			}
		}
		return m, m.refreshDetail()

	case dailyLoadedMsg:
		m.dailyLoaded = true
		m.dailyErr = msg.err
		if msg.err == nil {
			m.daily = msg.info
		}
		if m.activeSourceKind() == srcDaily {
			m.rebuildView()
			return m, tea.Batch(m.debouncePreview(), m.refreshDetail())
		}
		return m, m.refreshDetail()

	case plansLoadedMsg:
		m.sources = m.sources[:2] // keep "All Problems" + "Daily Question"
		for _, p := range msg.plans {
			m.sources = append(m.sources, sourceItem{label: p.Name, kind: srcPlan, plan: p})
		}
		m.plansLoaded = true
		m.relayout() // the Sources pane is sized to the source count
		restoreCmd := m.attemptRestore()
		return m, tea.Batch(restoreCmd, m.refreshDetail())

	case planSlugsMsg:
		if msg.err != nil {
			m.statusMsg = "plan load failed: " + msg.err.Error()
			return m, nil
		}
		m.planCache[msg.slug] = msg.slugs
		if m.activeSourceIsPlan(msg.slug) {
			m.rebuildView()
			restoreCmd := m.attemptRestore()
			return m, tea.Batch(restoreCmd, m.debouncePreview())
		}
		return m, nil

	case previewDebounceMsg:
		if msg.slug == m.currentSlug() {
			m.savePosition()
		}
		if msg.slug == m.currentSlug() && msg.slug != m.previewSlug {
			if m.previewCancel != nil {
				m.previewCancel() // abandon the previous load
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			m.previewCancel = cancel
			m.previewSlug = msg.slug
			m.previewErr = nil
			m.previewLoading = true
			m.previewImages = nil
			return m, m.loadStatement(ctx, msg.slug)
		}
		return m, nil

	case statementMsg:
		if msg.slug != m.previewSlug {
			return m, nil
		}
		m.previewLoading = false
		m.previewErr = msg.err
		if msg.err != nil {
			return m, nil
		}
		m.previewMD = msg.md
		return m, tea.Batch(m.refreshDetail(), m.loadPreviewImagesCmd(msg.slug, msg.md))

	case previewImagesMsg:
		if msg.slug != m.previewSlug || len(msg.byURL) == 0 {
			return m, nil
		}
		m.previewImages = &statementImages{proto: m.imgProto, byURL: msg.byURL}
		return m, m.refreshDetail()

	case previewRenderedMsg:
		m.renderCache[msg.key] = previewEntry{content: msg.content, prefix: msg.prefix}
		if msg.key == m.previewKey() && !m.detailShowsStatus {
			m.previewVP.SetContent(msg.content)
			m.detailSel.clear()
			m.previewVP.GotoTop()
			m.previewRenderedKey = msg.key
			m.previewContentSlug = m.previewSlug
			m.imgWritten = queueImagePrefix(m.imgWriter, msg.prefix, m.imgWritten)
		}
		return m, nil

	case syncDoneMsg:
		m.syncing = false
		if msg.err != nil {
			m.statusMsg = "sync failed: " + msg.err.Error()
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("synced %d problems", msg.count)
		return m, tea.Batch(m.loadProblems(), m.loadAuth(), m.loadDaily())

	case progressDoneMsg:
		m.progressing = false
		if msg.err != nil {
			m.statusMsg = "progress sync failed: " + msg.err.Error()
			return m, nil
		}
		m.progressSync = time.Now()
		m.statusMsg = fmt.Sprintf("progress synced · %d solved", msg.solved)
		return m, tea.Batch(m.loadProblems(), m.loadDaily(), m.refreshDetail())

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	if m.topicPicker != nil {
		var cmd tea.Cmd
		m.topicPicker.search, cmd = m.topicPicker.search.Update(msg)
		return m, cmd
	}
	return m, nil
}

// regionAt returns the pane under the given terminal cell.
func (m *BrowseModel) regionAt(x, y int) (Region, bool) {
	if m.layout.Single {
		return m.focus, true
	}
	for _, r := range []Region{RegionStatus, RegionSources, RegionList, RegionDetail} {
		if m.layout.RectFor(r).Contains(x, y) {
			return r, true
		}
	}
	return 0, false
}

// listRowAt maps a terminal row to a filtered-list index, or -1.
func (m *BrowseModel) listRowAt(my int) int {
	top := m.layout.List.Y + 1 // border (the title lives in the border row now)
	if m.filtering {
		top++ // filter input line
	}
	top++ // column header
	row := my - top
	if row < 0 {
		return -1
	}
	idx := m.top + row
	if idx < 0 || idx >= len(m.filtered) {
		return -1
	}
	return idx
}

// sourceRowAt maps a click Y to a source index. The Sources body interleaves
// two header lines and a blank among the rows, so any click inside the pane
// snaps to the nearest source — a click anywhere on a source is a selection,
// same as the problem list.
func (m *BrowseModel) sourceRowAt(my int) int {
	if len(m.sources) == 0 {
		return -1
	}
	// Body layout: 0 "PROBLEMS", 1 All Problems, 2 Daily Question, 3 blank,
	// 4 "STUDY PLANS", 5 plan[0], 6 plan[1], … → plan[k] at line 3+k (k>=2).
	body := my - (m.layout.Sources.Y + 1) // border (the title lives in the border row now)
	if body < 0 {
		return -1
	}
	var idx int
	switch {
	case body <= 1: // "PROBLEMS" header or the All-Problems row
		idx = 0
	case body == 2: // Daily Question
		idx = 1
	case body <= 5: // blank / "STUDY PLANS" header / first plan row
		idx = 2
	default:
		idx = body - 3
	}
	if idx >= len(m.sources) {
		idx = len(m.sources) - 1
	}
	return idx
}

func (m *BrowseModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// A drag already in progress: route its motion/release to the Detail pane
	// it started in regardless of which region the cursor is over now
	// (dragging past the pane's border still extends the selection, matching
	// normal terminal drag-select) rather than re-resolving regionAt below.
	// Any other action (in practice, a stray press with no matching release —
	// mouse protocols always pair the two for a real drag) falls through to
	// normal click dispatch instead of being swallowed; begin()/relayout()
	// below reset detailSel.active cleanly either way.
	//
	// Deliberately checked BEFORE the filtering/help/topicPicker guard below:
	// a drag can start on the Detail pane and then have an overlay opened
	// over it via the keyboard (e.g. `?` or `/`) before the button is
	// released. If the release were swallowed by that guard instead,
	// detailSel.active would be left stuck true with no way to finish or
	// clear it until the next click — this lets an in-flight drag always
	// reach its release, regardless of what opened in the meantime.
	if m.detailSel.active && (msg.Action == tea.MouseActionMotion || msg.Action == tea.MouseActionRelease) {
		return m.continueDetailSelection(msg)
	}
	if m.filtering || m.showHelp || m.topicPicker != nil {
		return m, nil
	}
	reg, ok := m.regionAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}

	if tea.MouseEvent(msg).IsWheel() {
		up := msg.Button == tea.MouseButtonWheelUp
		// Only vertical wheel scrolls. A trackpad also emits WheelLeft/Right on a
		// slightly diagonal flick; treating those as "down" made the list jitter.
		if !up && msg.Button != tea.MouseButtonWheelDown {
			return m, nil
		}
		switch reg {
		case RegionList:
			if up {
				m.scrollList(-wheelScrollLines)
			} else {
				m.scrollList(wheelScrollLines)
			}
		case RegionSources:
			if up && m.srcCursor > 0 {
				m.srcCursor--
			} else if !up && m.srcCursor < len(m.sources)-1 {
				m.srcCursor++
			}
		case RegionDetail:
			m.detailSel.clear()
			if up {
				m.previewVP.ScrollUp(wheelScrollLines)
			} else {
				m.previewVP.ScrollDown(wheelScrollLines)
			}
		}
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	var cmd tea.Cmd
	if m.focus != reg {
		m.setFocus(reg)
		// Focusing Detail only makes it scrollable — its content and scroll
		// position stay put. Focusing a left pane may change what Detail shows.
		if reg != RegionDetail {
			cmd = m.refreshDetail()
		}
	}
	switch reg {
	case RegionList:
		if idx := m.listRowAt(msg.Y); idx >= 0 {
			if idx == m.cursor { // second click on the selected row → open
				return m.openCurrent()
			}
			m.cursor = idx
			m.clampCursor()
			cmd = tea.Batch(cmd, m.debouncePreview())
		}
	case RegionSources:
		if idx := m.sourceRowAt(msg.Y); idx >= 0 {
			m.srcCursor = idx
			cmd = tea.Batch(cmd, m.activateSource(idx))
		}
	case RegionDetail:
		if lx, ly, ok := localCell(msg.X, msg.Y, m.layout.Detail); ok {
			m.detailSel.begin(lx, ly)
		}
	}
	return m, cmd
}

// continueDetailSelection routes an in-progress Detail-pane drag's motion and
// release events (see handleMouse) — coordinates are clamped to the pane's
// body rather than re-validated, so dragging past the border still extends
// the selection instead of freezing it.
func (m *BrowseModel) continueDetailSelection(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	lx, ly := clampCell(msg.X, msg.Y, m.layout.Detail)
	if msg.Action == tea.MouseActionRelease {
		m.detailSel.end(lx, ly)
		m.copyDetailSelection()
		return m, nil
	}
	m.detailSel.extend(lx, ly)
	return m, nil
}

// copyDetailSelection finalizes a finished Detail-pane selection: copies its
// plain text to the system clipboard via OSC 52 and leaves a one-line
// confirmation in the status bar. A no-op for a plain click (no drag) — see
// selectionState.end.
func (m *BrowseModel) copyDetailSelection() {
	if !m.detailSel.hasSelection {
		return
	}
	text := selectedText(strings.Split(m.previewVP.View(), "\n"), m.detailSel)
	m.copyText(text)
}

// copyText writes text to the system clipboard (OSC 52, via the same
// *ImageWriter already wired in for out-of-band escape injection — see
// imgwriter.go) and leaves a one-line status-bar confirmation, reusing the
// same statusMsg mechanism run/submit/sync results already report through.
func (m *BrowseModel) copyText(text string) {
	if msg := copyToClipboard(m.imgWriter, text); msg != "" {
		m.statusMsg = msg
	}
}

func (m *BrowseModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.topicPicker != nil {
		return m.handleTopicKey(msg)
	}
	if m.filtering {
		switch {
		case key.Matches(msg, m.keys.Open):
			m.filtering = false
			m.filter.Blur()
			return m, nil
		case key.Matches(msg, m.keys.ClearFilt):
			m.filtering = false
			m.filter.Blur()
			m.filter.SetValue("")
			m.applyFilter()
			return m, m.debouncePreview()
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyFilter()
		return m, tea.Batch(cmd, m.debouncePreview())
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		m.savePosition()
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case m.showHelp && key.Matches(msg, m.keys.ClearFilt):
		m.showHelp = false
		return m, nil

	case key.Matches(msg, m.keys.Sync):
		if !m.syncing && !m.progressing {
			m.rearmAutoSync()
			m.syncing = true
			return m, tea.Batch(m.syncCmd(), m.spin.Tick)
		}
		return m, nil

	case key.Matches(msg, m.keys.Zoom):
		m.zoom = !m.zoom
		m.relayout()
		return m, m.refreshDetail()

	case key.Matches(msg, m.keys.NextPane):
		m.setFocus(m.focus.next())
		return m, m.refreshDetail()
	case key.Matches(msg, m.keys.PrevPane):
		m.setFocus(m.focus.prev())
		return m, m.refreshDetail()

	case key.Matches(msg, m.keys.Filter):
		if m.focus == RegionList {
			m.filtering = true
			m.filter.Focus()
			return m, nil
		}
	case key.Matches(msg, m.keys.FilterTags):
		return m.openTopicPicker()
	case key.Matches(msg, m.keys.FilterDiff):
		m.fltDiff = cycleDifficulty(m.fltDiff)
		return m.afterListChange()
	case key.Matches(msg, m.keys.FilterStatus):
		if !m.auth.Authed {
			m.statusMsg = "sign in (`lazyleet auth`), then press s to sync progress"
		}
		m.fltStatus = cycleStatus(m.fltStatus)
		return m.afterListChange()
	case key.Matches(msg, m.keys.FilterPaid):
		m.fltHidePaid = !m.fltHidePaid
		return m.afterListChange()
	case key.Matches(msg, m.keys.Sort):
		m.sortMode = (m.sortMode + 1) % sortModeCount
		return m.afterListChange()
	case key.Matches(msg, m.keys.ClearListFilter):
		if !m.listFilterDirty() {
			return m, nil
		}
		m.fltTags = nil
		m.fltDiff, m.fltStatus, m.fltHidePaid, m.sortMode = "", "", false, sortByID
		return m.afterListChange()
	}

	// F2 digit-jump: 1-4 focus Status/Sources/Problems/Detail straight away,
	// from any pane, mirroring lazygit's numbered panels. Gated on !showHelp
	// like the other modal/overlay guards above (topicPicker and filtering
	// already returned earlier in this function); guarded against an empty
	// rect so a still-narrow layout switches the visible pane rather than
	// no-oping (in practice ComputeBrowse never actually produces an empty
	// rect for any region — Single mode gives every region the full work
	// area — but this keeps the guard meaningful if that ever changes).
	if !m.showHelp {
		if n, ok := digitKey(msg); ok {
			if reg, ok := regionForNumber(n); ok && !m.layout.RectFor(reg).Empty() {
				m.setFocus(reg)
				return m, m.refreshDetail()
			}
		}
	}

	switch m.focus {
	case RegionStatus:
		return m, nil // the Status pane is static; focusing it shows detail info
	case RegionSources:
		return m.handleSourcesKey(msg)
	case RegionDetail:
		return m.handleDetailKey(msg)
	default:
		return m.handleListKey(msg)
	}
}

func (m *BrowseModel) handleSourcesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.srcCursor > 0 {
			m.srcCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.srcCursor < len(m.sources)-1 {
			m.srcCursor++
		}
	case key.Matches(msg, m.keys.Open):
		return m, m.activateSource(m.srcCursor)
	}
	return m, nil
}

// activateSource applies the source at idx (All Problems, the Daily Question, or
// a study plan), rebuilds the list, and moves focus there.
func (m *BrowseModel) activateSource(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.sources) {
		return nil
	}
	m.activeSrc = idx
	src := m.sources[idx]
	var cmd tea.Cmd
	switch src.kind {
	case srcPlan:
		if _, ok := m.planCache[src.plan.Slug]; !ok {
			cmd = m.loadPlanSlugs(src.plan)
		}
	case srcDaily:
		if !m.dailyLoaded {
			cmd = m.loadDaily()
		}
	}
	m.rebuildView()
	m.setFocus(RegionList)
	return tea.Batch(cmd, m.debouncePreview())
}

// activeSourceKind is the kind of the currently-applied source.
func (m *BrowseModel) activeSourceKind() int {
	if m.activeSrc < 0 || m.activeSrc >= len(m.sources) {
		return srcAll
	}
	return m.sources[m.activeSrc].kind
}

// sourceKey encodes the active source as the stable string BrowseData persists:
// "all", "daily", or "plan:<slug>". sourceIndexForKey is its inverse.
func (m *BrowseModel) sourceKey() string {
	switch m.activeSourceKind() {
	case srcDaily:
		return "daily"
	case srcPlan:
		if m.activeSrc >= 0 && m.activeSrc < len(m.sources) {
			return "plan:" + m.sources[m.activeSrc].plan.Slug
		}
	}
	return "all"
}

// sourceIndexForKey finds the source matching a key sourceKey produced, or -1
// if it no longer exists (e.g. a since-removed study plan).
func (m *BrowseModel) sourceIndexForKey(key string) int {
	planSlug, isPlan := strings.CutPrefix(key, "plan:")
	for i, s := range m.sources {
		switch {
		case key == "all" && s.kind == srcAll:
			return i
		case key == "daily" && s.kind == srcDaily:
			return i
		case isPlan && s.kind == srcPlan && s.plan.Slug == planSlug:
			return i
		}
	}
	return -1
}

// savePosition remembers the active source and selected problem so the next
// launch can resume here. Best-effort: a local, low-stakes write, not worth
// surfacing a failure for.
func (m *BrowseModel) savePosition() {
	_ = m.data.SavePosition(context.Background(), m.sourceKey(), m.currentSlug())
}

// attemptRestore applies a source+problem remembered from a previous session,
// once whatever it needs has actually loaded: the catalog always; for a study
// plan, the plan list (to find its index) and then that plan's slugs (to know
// where in it the remembered problem sits). It's cheap to call from every
// message handler that might have just made progress possible — it no-ops
// until pendingRestore is set and ready, and clears it once applied (or once
// the remembered source turns out to be gone).
func (m *BrowseModel) attemptRestore() tea.Cmd {
	r := m.pendingRestore
	if r == nil || len(m.allRows) == 0 {
		return nil
	}
	if strings.HasPrefix(r.sourceKey, "plan:") && !m.plansLoaded {
		return nil // wait for the plan list itself
	}
	idx := m.sourceIndexForKey(r.sourceKey)
	if idx < 0 {
		m.pendingRestore = nil // remembered source is gone — quietly give up
		return nil
	}
	var cmd tea.Cmd
	if idx != m.activeSrc {
		cmd = m.activateSource(idx)
	}
	if slug, ok := strings.CutPrefix(r.sourceKey, "plan:"); ok {
		if _, cached := m.planCache[slug]; !cached {
			return cmd // that plan's slugs haven't loaded — selectSlug on planSlugsMsg
		}
	}
	m.selectSlug(r.slug)
	m.pendingRestore = nil
	return cmd
}

// selectSlug moves the cursor to slug within the current filtered view, if
// it's present there. A no-op (never a crash) when it isn't — e.g. the daily
// source only ever holds today's problem, not whatever was daily last time.
func (m *BrowseModel) selectSlug(slug string) {
	if slug == "" {
		return
	}
	for i, fi := range m.filtered {
		if m.view[fi].Slug == slug {
			m.cursor = i
			m.clampCursor()
			return
		}
	}
}

// dailyRow returns the problem row for today's daily challenge, preferring the
// catalog row (carries solve status, AC%, tags) and falling back to a row
// synthesised from DailyInfo when the catalog doesn't have it.
func (m *BrowseModel) dailyRow() (BrowseRow, bool) {
	if m.daily.Slug == "" {
		return BrowseRow{}, false
	}
	for _, r := range m.allRows {
		if r.Slug == m.daily.Slug {
			return r, true
		}
	}
	return BrowseRow{Slug: m.daily.Slug, Title: m.daily.Title, Difficulty: m.daily.Difficulty}, true
}

func (m *BrowseModel) openCurrent() (tea.Model, tea.Cmd) {
	s := m.currentSlug()
	if s == "" {
		return m, nil
	}
	m.Chosen = s
	m.savePosition()
	if m.embedded {
		slug := s
		return m, func() tea.Msg { return OpenProblemMsg{Slug: slug} }
	}
	return m, tea.Quit
}

func (m *BrowseModel) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	prev := m.cursor
	switch {
	case key.Matches(msg, m.keys.Up):
		m.cursor--
	case key.Matches(msg, m.keys.Down):
		m.cursor++
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
	case key.Matches(msg, m.keys.Bottom):
		m.cursor = len(m.filtered) - 1
	case key.Matches(msg, m.keys.PageUp):
		m.cursor -= m.listRows()
	case key.Matches(msg, m.keys.PageDown):
		m.cursor += m.listRows()
	case key.Matches(msg, m.keys.Open):
		return m.openCurrent()
	default:
		return m, nil
	}
	m.clampCursor()
	if m.cursor != prev {
		return m, m.debouncePreview()
	}
	return m, nil
}

func (m *BrowseModel) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.detailSel.clear()
		m.previewVP.ScrollUp(2)
	case key.Matches(msg, m.keys.Down):
		m.detailSel.clear()
		m.previewVP.ScrollDown(2)
	case key.Matches(msg, m.keys.PageUp):
		m.detailSel.clear()
		m.previewVP.PageUp()
	case key.Matches(msg, m.keys.PageDown):
		m.detailSel.clear()
		m.previewVP.PageDown()
	case key.Matches(msg, m.keys.Copy):
		// F1 keyboard fallback: copy the whole pane's visible content when
		// there's no active drag selection to copy instead (e.g. terminals
		// without OSC 52 support).
		if m.detailSel.hasSelection {
			m.copyDetailSelection()
		} else {
			m.copyText(plainViewportText(m.previewVP.View()))
		}
	}
	return m, nil
}

// --- view model helpers ---------------------------------------------------

func (m *BrowseModel) activeSourceIsPlan(slug string) bool {
	if m.activeSrc <= 0 || m.activeSrc >= len(m.sources) {
		return false
	}
	s := m.sources[m.activeSrc]
	return s.kind == srcPlan && s.plan.Slug == slug
}

func (m *BrowseModel) rebuildView() {
	switch m.activeSourceKind() {
	case srcDaily:
		m.view = m.view[:0]
		if r, ok := m.dailyRow(); ok {
			m.view = append(m.view, r)
		}
	case srcPlan:
		ref := m.sources[m.activeSrc].plan
		slugs := m.planCache[ref.Slug]
		byslug := make(map[string]BrowseRow, len(m.allRows))
		for _, r := range m.allRows {
			byslug[r.Slug] = r
		}
		m.view = m.view[:0]
		for _, s := range slugs {
			if r, ok := byslug[s]; ok {
				m.view = append(m.view, r)
			} else {
				m.view = append(m.view, BrowseRow{Slug: s, Title: s, Difficulty: "?"})
			}
		}
	default: // srcAll
		m.view = m.allRows
	}
	m.view = applyListFilterSort(m.view, m.listFilterState(), m.sortMode)

	m.haystack = make([]string, len(m.view))
	for i, r := range m.view {
		m.haystack[i] = fmt.Sprintf("%d %s", r.FrontendID, r.Title)
	}
	m.cursor, m.top = 0, 0
	m.applyFilter()
}

// afterListChange rebuilds the view after a structured filter / sort change and
// refreshes the preview for the new selection.
func (m *BrowseModel) afterListChange() (tea.Model, tea.Cmd) {
	m.rebuildView()
	return m, m.debouncePreview()
}

func (m *BrowseModel) applyFilter() {
	q := strings.TrimSpace(m.filter.Value())
	if q == "" {
		m.filtered = m.filtered[:0]
		for i := range m.view {
			m.filtered = append(m.filtered, i)
		}
	} else {
		matches := fuzzy.Find(q, m.haystack)
		m.filtered = m.filtered[:0]
		for _, mt := range matches {
			m.filtered = append(m.filtered, mt.Index)
		}
	}
	m.clampCursor()
}

func (m *BrowseModel) clampCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	rows := m.listRows()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if rows > 0 && m.cursor >= m.top+rows {
		m.top = m.cursor - rows + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m *BrowseModel) currentSlug() string {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return ""
	}
	return m.view[m.filtered[m.cursor]].Slug
}

func (m *BrowseModel) currentRow() (BrowseRow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return BrowseRow{}, false
	}
	return m.view[m.filtered[m.cursor]], true
}

func (m *BrowseModel) listRows() int {
	// The frame eats border(2) — the title lives in the border row now, not
	// a separate line; the list body prints its own column-header line.
	// What's left is data rows.
	h := m.layout.List.H - 2 - 1
	if h < 1 {
		return 1
	}
	return h
}

// scrollList moves the list viewport by delta rows without touching the
// selection — mouse-wheel scrolling browses the list, arrow keys move the
// cursor. The cursor is allowed to scroll out of view.
func (m *BrowseModel) scrollList(delta int) {
	maxTop := len(m.filtered) - m.listRows()
	if maxTop < 0 {
		maxTop = 0
	}
	m.top += delta
	if m.top > maxTop {
		m.top = maxTop
	}
	if m.top < 0 {
		m.top = 0
	}
}

// wheelAtEdge reports whether a wheel event would do nothing: the pane under the
// cursor is already at the top (wheel-up) or bottom (wheel-down) of its content,
// or the event lands somewhere a wheel never scrolls. See WheelEdgeFilter.
func (m *BrowseModel) wheelAtEdge(msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return true // horizontal wheel never scrolls a vertical pane
	}
	if m.filtering || m.showHelp || m.topicPicker != nil {
		return true
	}
	reg, ok := m.regionAt(msg.X, msg.Y)
	if !ok {
		scrollLogf("browse wheelAtEdge: no region at x=%d y=%d single=%v", msg.X, msg.Y, m.layout.Single)
		return true
	}
	up := msg.Button == tea.MouseButtonWheelUp
	switch reg {
	case RegionList:
		maxTop := len(m.filtered) - m.listRows()
		if maxTop < 0 {
			maxTop = 0
		}
		scrollLogf("browse wheelAtEdge: RegionList up=%v top=%d maxTop=%d listRows=%d filtered=%d",
			up, m.top, maxTop, m.listRows(), len(m.filtered))
		if up {
			return m.top <= 0
		}
		return m.top >= maxTop
	case RegionDetail:
		if up {
			return m.previewVP.AtTop()
		}
		return m.previewVP.AtBottom()
	case RegionSources:
		if up {
			return m.srcCursor <= 0
		}
		return m.srcCursor >= len(m.sources)-1
	default: // RegionStatus — static, a wheel never moves it
		return true
	}
}

// sourcesRows is how many body lines the Sources pane needs: a "PROBLEMS"
// header, one line per source, and a blank + "STUDY PLANS" header.
func (m *BrowseModel) sourcesRows() int { return len(m.sources) + 3 }

// emptyListReason explains an empty problem list for the active source while
// its data is still loading (or failed), else "".
func (m *BrowseModel) emptyListReason() string {
	if m.activeSrc < 0 || m.activeSrc >= len(m.sources) {
		return ""
	}
	switch src := m.sources[m.activeSrc]; src.kind {
	case srcPlan:
		if _, ok := m.planCache[src.plan.Slug]; !ok {
			return "loading " + src.label + "…"
		}
	case srcDaily:
		if m.dailyErr != nil {
			return "today's daily challenge is unavailable"
		}
		if !m.dailyLoaded || m.daily.Slug == "" {
			return "loading today's daily challenge…"
		}
	}
	return ""
}

func (m *BrowseModel) relayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.layout = ComputeBrowse(m.width, m.height, m.focus, m.zoom, m.sourcesRows())
	m.focus = m.layout.Focused

	pw, ph := innerSize(m.layout.Detail)
	if m.previewVP.Width == 0 && m.previewVP.Height == 0 {
		m.previewVP = viewport.New(pw, ph)
		m.previewVP.MouseWheelEnabled = true
	} else {
		m.previewVP.Width, m.previewVP.Height = pw, ph
	}
	m.filter.Width = m.layout.List.W - 6
	m.clampCursor()
}

// setFocus moves focus to reg and keeps the Detail pane's context in sync:
// focusing a left-column pane decides whether Detail shows the Status info or a
// problem statement; focusing Detail itself leaves that choice untouched so the
// pane can be scrolled without its content changing under the cursor.
func (m *BrowseModel) setFocus(reg Region) {
	m.focus = reg
	switch reg {
	case RegionStatus:
		m.detailShowsStatus = true
	case RegionSources, RegionList:
		m.detailShowsStatus = false
	}
}

// refreshDetail updates the right-hand pane for the current focus: the Status
// pane's expanded info when it is focused, otherwise the problem statement.
func (m *BrowseModel) refreshDetail() tea.Cmd {
	if m.detailShowsStatus {
		body := m.statusDetailBody(m.previewVP.Width)
		if body != m.previewRenderedBody {
			m.previewVP.SetContent(body)
			m.detailSel.clear()
			m.previewVP.GotoTop()
			m.previewRenderedBody = body
		}
		return nil
	}
	return m.refreshPreviewContent()
}

// EnableImages turns on inline preview images. iw must be the same ImageWriter
// passed to tea.WithOutput.
func (m *BrowseModel) EnableImages(proto termimg.Protocol, cacheDir string, iw *ImageWriter) {
	m.imgProto = proto
	m.imgDir = cacheDir
	m.imgWriter = iw
}

// SetVersion supplies the build's version string (main.versionString()) for
// display in the Status/About panel. Optional: an unset version simply omits
// that line.
func (m *BrowseModel) SetVersion(v string) {
	m.version = v
}

type previewEntry struct {
	content string
	prefix  string
}

type previewRenderedMsg struct {
	key     string
	content string
	prefix  string
}

func (m *BrowseModel) previewKey() string {
	hasImg := m.previewImages != nil && len(m.previewImages.byURL) > 0
	return fmt.Sprintf("%s|%d|%t", m.previewSlug, m.previewVP.Width, hasImg)
}

// refreshPreviewContent updates the preview pane with the header (difficulty
// / AC% / tags, via renderProblemHeader — shared with the workspace Statement
// pane) followed by the glamour-rendered statement. The header is cheap and
// built inline; the statement is rendered off the UI goroutine (and cached),
// so navigating never blocks on it.
func (m *BrowseModel) refreshPreviewContent() tea.Cmd {
	if m.previewVP.Width < 1 {
		return nil
	}
	key := m.previewKey()
	if e, ok := m.renderCache[key]; ok {
		if key != m.previewRenderedKey {
			m.previewVP.SetContent(e.content)
			m.detailSel.clear()
			m.previewVP.GotoTop()
			m.previewRenderedKey = key
		}
		m.previewContentSlug = m.previewSlug
		m.imgWritten = queueImagePrefix(m.imgWriter, e.prefix, m.imgWritten)
		return nil
	}
	// render asynchronously
	md, width := m.previewMD, m.previewVP.Width
	imgs := m.previewImages
	header := ""
	if r, ok := m.currentRow(); ok {
		header = renderProblemHeader(m.th, r.Meta(), width)
	}
	return func() tea.Msg {
		body, prefix := renderStatementMD(newStatementRenderer(width), md, width, imgs)
		content := header + body
		return previewRenderedMsg{key: key, content: strings.TrimRight(content, "\n"), prefix: prefix}
	}
}

type previewImagesMsg struct {
	slug  string
	byURL map[string]*termimg.Image
}

func (m *BrowseModel) loadPreviewImagesCmd(slug, md string) tea.Cmd {
	if m.imgProto == termimg.ProtoNone {
		return nil
	}
	urls := imageURLs(md)
	if len(urls) == 0 {
		return nil
	}
	dir := m.imgDir
	return func() tea.Msg {
		out := make(map[string]*termimg.Image, len(urls))
		for _, u := range urls {
			if !allowedImageHost(u) {
				continue
			}
			data, err := termimg.Fetch(context.Background(), dir, u)
			if err != nil {
				continue
			}
			if im, err := termimg.Decode(data); err == nil {
				out[u] = im
			}
		}
		return previewImagesMsg{slug: slug, byURL: out}
	}
}

var _ tea.Model = (*BrowseModel)(nil)
