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
	"github.com/charmbracelet/glamour"
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

	previewSlug    string
	previewMD      string
	previewVP      viewport.Model
	previewErr     error
	previewLoading bool
	previewImages  *statementImages
	stmtRenderer   *glamour.TermRenderer
	stmtWidth      int
	previewTab     int // 0 = statement, 1 = topics

	imgProto   termimg.Protocol
	imgDir     string
	imgWriter  *ImageWriter
	imgWritten string

	spin      spinner.Model
	syncing   bool
	statusMsg string
	loadErr   error
	lastSync  time.Time

	// Chosen is the slug the user opened, set just before tea.Quit.
	Chosen string
}

type browseLoadedMsg struct {
	rows []BrowseRow
	err  error
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
		data:      data,
		th:        DefaultTheme(),
		keys:      DefaultBrowseKeyMap(),
		focus:     RegionList,
		filter:    ti,
		spin:      sp,
		planCache: map[string][]string{},
		sources:   []sourceItem{{label: "All Problems", kind: srcAll}},
	}
}

func (m *BrowseModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.loadProblems(), m.loadPlans())
}

// --- commands --------------------------------------------------------------

func (m *BrowseModel) loadProblems() tea.Cmd {
	return func() tea.Msg {
		rows, err := m.data.ListProblems(context.Background())
		return browseLoadedMsg{rows: rows, err: err}
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

func (m *BrowseModel) loadStatement(slug string) tea.Cmd {
	return func() tea.Msg {
		md, err := m.data.LoadStatement(context.Background(), slug)
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
		m.relayout()
		m.ready = true
		return m, nil

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
		if t, ok := m.data.LastSync(context.Background()); ok {
			m.lastSync = t
		}
		m.rebuildView()
		return m, m.debouncePreview()

	case plansLoadedMsg:
		m.sources = m.sources[:1] // keep "All Problems"
		for _, p := range msg.plans {
			m.sources = append(m.sources, sourceItem{label: p.Name, kind: srcPlan, plan: p})
		}
		return m, nil

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
			m.previewSlug = msg.slug
			m.previewErr = nil
			m.previewLoading = true
			m.previewImages = nil
			return m, m.loadStatement(msg.slug)
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
		m.renderPreview(msg.md)
		return m, m.loadPreviewImagesCmd(msg.slug, msg.md)

	case previewImagesMsg:
		if msg.slug != m.previewSlug || len(msg.byURL) == 0 {
			return m, nil
		}
		m.previewImages = &statementImages{proto: m.imgProto, byURL: msg.byURL}
		m.refreshPreviewContent()
		return m, nil

	case syncDoneMsg:
		m.syncing = false
		if msg.err != nil {
			m.statusMsg = "sync failed: " + msg.err.Error()
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("synced %d problems", msg.count)
		return m, m.loadProblems()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
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
		return m, nil

	case key.Matches(msg, m.keys.NextPane):
		m.focus = m.focus.next(m.layout.ShowSidebar, m.layout.ShowPreview)
		return m, nil
	case key.Matches(msg, m.keys.PrevPane):
		m.focus = m.focus.prev(m.layout.ShowSidebar, m.layout.ShowPreview)
		return m, nil

	case key.Matches(msg, m.keys.Filter):
		if m.focus == RegionList {
			m.filtering = true
			m.filter.Focus()
			return m, nil
		}
	case key.Matches(msg, m.keys.PreviewTab):
		m.previewTab = (m.previewTab + 1) % 2
		m.refreshPreviewContent()
		return m, nil

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
	case RegionSidebar:
		return m.handleSidebarKey(msg)
	case RegionPreview:
		return m.handlePreviewKey(msg)
	default:
		return m.handleListKey(msg)
	}
}

func (m *BrowseModel) handleSidebarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.activeSrc = m.srcCursor
		src := m.sources[m.activeSrc]
		var cmd tea.Cmd
		if src.kind == srcPlan {
			if _, ok := m.planCache[src.plan.Slug]; !ok {
				cmd = m.loadPlanSlugs(src.plan)
			}
		}
		m.rebuildView()
		m.focus = RegionList
		return m, tea.Batch(cmd, m.debouncePreview())
	}
	return m, nil
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

func (m *BrowseModel) handlePreviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m *BrowseModel) relayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.layout = ComputeBrowse(m.width, m.height, m.focus, m.zoom)
	m.focus = m.layout.Focused

	pw, ph := innerSize(m.layout.Preview)
	if m.previewVP.Width == 0 && m.previewVP.Height == 0 {
		m.previewVP = viewport.New(pw, ph)
		m.previewVP.MouseWheelEnabled = true
	} else {
		m.previewVP.Width, m.previewVP.Height = pw, ph
	}
	m.filter.Width = m.layout.List.W - 6
	m.refreshPreviewContent()
	m.clampCursor()
}

// EnableImages turns on inline preview images. iw must be the same ImageWriter
// passed to tea.WithOutput.
func (m *BrowseModel) EnableImages(proto termimg.Protocol, cacheDir string, iw *ImageWriter) {
	m.imgProto = proto
	m.imgDir = cacheDir
	m.imgWriter = iw
}

func (m *BrowseModel) renderPreview(md string) {
	w := m.previewVP.Width
	if w < 1 {
		w = 60
	}
	if m.stmtRenderer == nil || m.stmtWidth != w {
		if r := newStatementRenderer(w); r != nil {
			m.stmtRenderer, m.stmtWidth = r, w
		}
	}
	m.previewMD = md
	m.refreshPreviewContent()
}

func (m *BrowseModel) refreshPreviewContent() {
	if m.previewVP.Width < 1 {
		return
	}
	if m.previewTab == 1 {
		m.previewVP.SetContent(m.topicsBody())
		return
	}
	body := renderStatementMD(m.stmtRenderer, m.previewMD, m.previewVP.Width, m.previewImages)
	m.previewVP.SetContent(strings.TrimRight(body, "\n"))
	m.imgWritten = queueImagePrefix(m.imgWriter, m.previewImages, m.imgWritten)
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
