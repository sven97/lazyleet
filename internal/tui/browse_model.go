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
	srcPlan
)

type sourceItem struct {
	label string
	kind  int
	plan  PlanRef
}

// BrowseModel is the Bubble Tea model for browse mode: a sources sidebar, a
// fuzzy-filterable problem list, and a statement preview. Selecting a problem
// with Enter sets Chosen and quits so the caller can open the workspace.
type BrowseModel struct {
	data BrowseData
	th   Theme
	keys BrowseKeyMap

	width, height int
	ready         bool
	layout        BrowseLayout
	focus         Region
	zoom          bool
	showHelp      bool

	allRows   []BrowseRow
	sources   []sourceItem
	srcCursor int
	activeSrc int
	planCache map[string][]string

	view     []BrowseRow
	haystack []string
	filtered []int
	cursor   int
	top      int

	// structured list filter + sort (fuzzy filter is `filter` below)
	fltDiff     string
	fltStatus   string
	fltHidePaid bool
	sortMode    int

	filtering bool
	filter    textinput.Model

	previewSlug        string
	previewMD          string
	previewVP          viewport.Model
	previewErr         error
	previewLoading     bool
	previewImages      *statementImages
	previewCancel      context.CancelFunc // cancels the in-flight statement/image load
	previewContentSlug string             // slug whose statement is currently in the viewport
	previewTab         int                // 0 = statement, 1 = topics
	// rendered preview bodies keyed by slug|width|tab|hasImages (glamour is
	// slow; keep re-visits and re-renders instant)
	renderCache map[string]previewEntry

	imgProto   termimg.Protocol
	imgDir     string
	imgWriter  *ImageWriter
	imgWritten string

	auth AuthState

	spin      spinner.Model
	syncing   bool
	statusMsg string
	loadErr   error
	lastSync  time.Time

	// Chosen is the slug the user opened, set just before tea.Quit.
	Chosen string
}

type browseLoadedMsg struct {
	rows     []BrowseRow
	lastSync time.Time
	err      error
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
		sources:     []sourceItem{{label: "All Problems", kind: srcAll}},
	}
}

func (m *BrowseModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.loadProblems(), m.loadPlans(), m.loadAuth())
}

type authLoadedMsg struct{ a AuthState }

func (m *BrowseModel) loadAuth() tea.Cmd {
	return func() tea.Msg { return authLoadedMsg{a: m.data.Auth(context.Background())} }
}

// --- commands --------------------------------------------------------------

func (m *BrowseModel) loadProblems() tea.Cmd {
	return func() tea.Msg {
		rows, err := m.data.ListProblems(context.Background())
		t, _ := m.data.LastSync(context.Background())
		return browseLoadedMsg{rows: rows, lastSync: t, err: err}
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
		if msg.err == nil && len(msg.rows) == 0 {
			m.statusMsg = "problem cache is empty — press s to sync"
		}
		if !msg.lastSync.IsZero() {
			m.lastSync = msg.lastSync
		}
		m.rebuildView()
		return m, m.debouncePreview()

	case authLoadedMsg:
		m.auth = msg.a
		return m, nil

	case plansLoadedMsg:
		m.sources = m.sources[:1] // keep "All Problems"
		for _, p := range msg.plans {
			m.sources = append(m.sources, sourceItem{label: p.Name, kind: srcPlan, plan: p})
		}
		m.relayout() // the Sources pane is sized to the source count
		return m, m.refreshDetail()

	case planSlugsMsg:
		if msg.err == nil {
			m.planCache[msg.slug] = msg.slugs
		}
		if m.activeSourceIsPlan(msg.slug) {
			m.rebuildView()
			return m, m.debouncePreview()
		}
		return m, nil

	case previewDebounceMsg:
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
		if msg.key == m.previewKey() && m.focus != RegionStatus {
			m.previewVP.SetContent(msg.content)
			m.previewVP.GotoTop()
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
		return m, m.loadProblems()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
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
	top := m.layout.List.Y + 2 // border + title
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

// sourceRowAt maps a terminal row to a source index, or -1. Body layout:
// line0 "PROBLEMS", line1 source[0], line2 blank, line3 "STUDY PLANS",
// line4 source[1], line5 source[2], …
func (m *BrowseModel) sourceRowAt(my int) int {
	body := my - (m.layout.Sources.Y + 2) // border + title
	switch {
	case body == 1:
		if len(m.sources) > 0 {
			return 0
		}
	case body >= 4:
		if idx := body - 3; idx < len(m.sources) {
			return idx
		}
	}
	return -1
}

func (m *BrowseModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.filtering || m.showHelp {
		return m, nil
	}
	reg, ok := m.regionAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}

	if tea.MouseEvent(msg).IsWheel() {
		up := msg.Button == tea.MouseButtonWheelUp
		switch reg {
		case RegionList:
			prev := m.cursor
			if up {
				m.cursor -= 2
			} else {
				m.cursor += 2
			}
			m.clampCursor()
			if m.cursor != prev {
				return m, m.debouncePreview()
			}
		case RegionSources:
			if up && m.srcCursor > 0 {
				m.srcCursor--
			} else if !up && m.srcCursor < len(m.sources)-1 {
				m.srcCursor++
			}
		case RegionDetail:
			if up {
				m.previewVP.ScrollUp(3)
			} else {
				m.previewVP.ScrollDown(3)
			}
		}
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	var cmd tea.Cmd
	if m.focus != reg {
		m.focus = reg
		cmd = m.refreshDetail()
	}
	switch reg {
	case RegionList:
		if idx := m.listRowAt(msg.Y); idx >= 0 {
			if idx == m.cursor { // second click on the selected row → open
				if s := m.currentSlug(); s != "" {
					m.Chosen = s
					return m, tea.Quit
				}
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
	}
	return m, cmd
}

func (m *BrowseModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case m.showHelp && key.Matches(msg, m.keys.ClearFilt):
		m.showHelp = false
		return m, nil

	case key.Matches(msg, m.keys.Sync):
		if !m.syncing {
			m.syncing = true
			m.statusMsg = "syncing…"
			return m, tea.Batch(m.syncCmd(), m.spin.Tick)
		}
		return m, nil

	case key.Matches(msg, m.keys.Zoom):
		m.zoom = !m.zoom
		m.relayout()
		return m, m.refreshDetail()

	case key.Matches(msg, m.keys.NextPane):
		m.focus = m.focus.next()
		return m, m.refreshDetail()
	case key.Matches(msg, m.keys.PrevPane):
		m.focus = m.focus.prev()
		return m, m.refreshDetail()

	case key.Matches(msg, m.keys.Filter):
		if m.focus == RegionList {
			m.filtering = true
			m.filter.Focus()
			return m, nil
		}
	case key.Matches(msg, m.keys.PreviewTab):
		if m.focus == RegionStatus {
			return m, nil
		}
		m.previewTab = (m.previewTab + 1) % 2
		return m, m.refreshDetail()

	case key.Matches(msg, m.keys.FilterDiff):
		m.fltDiff = cycleDifficulty(m.fltDiff)
		return m.afterListChange()
	case key.Matches(msg, m.keys.FilterStatus):
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
		m.fltDiff, m.fltStatus, m.fltHidePaid, m.sortMode = "", "", false, sortByID
		return m.afterListChange()
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

// activateSource applies the source at idx (All Problems, or a study plan),
// rebuilds the list, and moves focus there.
func (m *BrowseModel) activateSource(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.sources) {
		return nil
	}
	m.activeSrc = idx
	src := m.sources[idx]
	var cmd tea.Cmd
	if src.kind == srcPlan {
		if _, ok := m.planCache[src.plan.Slug]; !ok {
			cmd = m.loadPlanSlugs(src.plan)
		}
	}
	m.rebuildView()
	m.focus = RegionList
	return tea.Batch(cmd, m.debouncePreview())
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
		if s := m.currentSlug(); s != "" {
			m.Chosen = s
			return m, tea.Quit
		}
		return m, nil
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
		m.previewVP.ScrollUp(2)
	case key.Matches(msg, m.keys.Down):
		m.previewVP.ScrollDown(2)
	case key.Matches(msg, m.keys.PageUp):
		m.previewVP.PageUp()
	case key.Matches(msg, m.keys.PageDown):
		m.previewVP.PageDown()
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
	if m.activeSrc <= 0 || m.activeSrc >= len(m.sources) || m.sources[m.activeSrc].kind == srcAll {
		m.view = m.allRows
	} else {
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
	h := m.layout.List.H - 2 - 1 // border + header
	if h < 1 {
		return 1
	}
	return h
}

// sourcesRows is how many body lines the Sources pane needs: a "PROBLEMS"
// header, one line per source, and a blank + "STUDY PLANS" header.
func (m *BrowseModel) sourcesRows() int { return len(m.sources) + 3 }

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

// refreshDetail updates the right-hand pane for the current focus: the Status
// pane's expanded info when it is focused, otherwise the problem statement.
func (m *BrowseModel) refreshDetail() tea.Cmd {
	if m.focus == RegionStatus {
		m.previewVP.SetContent(m.statusDetailBody())
		m.previewVP.GotoTop()
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
	return fmt.Sprintf("%s|%d|%d|%t", m.previewSlug, m.previewVP.Width, m.previewTab, hasImg)
}

// refreshPreviewContent updates the preview pane. The topics tab is cheap and
// rendered inline; the statement tab is glamour-rendered off the UI goroutine
// (and cached), so navigating never blocks on it.
func (m *BrowseModel) refreshPreviewContent() tea.Cmd {
	if m.previewVP.Width < 1 {
		return nil
	}
	if m.previewTab == 1 {
		m.previewVP.SetContent(m.topicsBody())
		return nil
	}
	key := m.previewKey()
	if e, ok := m.renderCache[key]; ok {
		m.previewVP.SetContent(e.content)
		m.previewVP.GotoTop()
		m.previewContentSlug = m.previewSlug
		m.imgWritten = queueImagePrefix(m.imgWriter, e.prefix, m.imgWritten)
		return nil
	}
	// render asynchronously
	md, width := m.previewMD, m.previewVP.Width
	imgs := m.previewImages
	return func() tea.Msg {
		body, prefix := renderStatementMD(newStatementRenderer(width), md, width, imgs)
		return previewRenderedMsg{key: key, content: strings.TrimRight(body, "\n"), prefix: prefix}
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

func (m *BrowseModel) topicsBody() string {
	r, ok := m.currentRow()
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", m.th.Title.Render(r.Title))
	fmt.Fprintf(&b, "difficulty  %s\n", DifficultyStyle(r.Difficulty).Render(r.Difficulty))
	fmt.Fprintf(&b, "acceptance  %.1f%%\n", r.ACRate)
	if r.PaidOnly {
		b.WriteString(m.th.ErrorText.Render("paid-only\n"))
	}
	b.WriteString("\ntopics\n")
	if len(r.Tags) == 0 {
		b.WriteString(m.th.Muted.Render("  (run sync for topic tags)\n"))
	}
	for _, t := range r.Tags {
		fmt.Fprintf(&b, "  · %s\n", t)
	}
	return b.String()
}

var _ tea.Model = (*BrowseModel)(nil)
