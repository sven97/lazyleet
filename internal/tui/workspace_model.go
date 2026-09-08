package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/runner"
	"github.com/sven97/lazyleet/internal/termimg"
	"github.com/sven97/lazyleet/internal/testcase"
	"github.com/sven97/lazyleet/internal/workspace"
)

// WorkspaceModel is the Bubble Tea model for Tier C: statement, a read-only
// mirror of the solution file, and local run results, side by side, with a
// file-watch + run-on-save loop.
type WorkspaceModel struct {
	ws   *workspace.Workspace
	q    leetcode.Question
	th   Theme
	keys KeyMap

	editor    string
	runOnSave bool
	debounce  time.Duration

	width, height int
	ready         bool
	focused       Pane
	mode          ScreenMode
	layout        Layout

	statement viewport.Model
	code      viewport.Model
	results   viewport.Model

	stmtRenderer *glamour.TermRenderer
	stmtWidth    int
	stmtImages   *statementImages
	imgProto     termimg.Protocol
	imgDir       string
	codeSrc      string
	ranSrc       string // codeSrc as of the last local run started

	runner    runner.Runner
	runnerErr error // why there is no local runner for this language, if so
	running   bool
	runGen    int
	spin      spinner.Model
	lastRun   *runner.Result
	lastErr   error

	// remote run / submit against LeetCode
	remote        RemoteJudge
	remoteRunning bool
	remoteKind    string // "run" | "submit"
	remoteOut     *RemoteOutcome
	remoteErr     error
	showRemote    bool // Results pane is showing the remote outcome, not local

	watcher   *workspace.Watcher
	statusMsg string
}

type fileChangedMsg struct{}
type runDebounceMsg struct{ gen int }
type runFinishedMsg struct {
	res runner.Result
	err error
}
type editorFinishedMsg struct{ err error }
type remoteDoneMsg struct {
	out RemoteOutcome
	err error
}

// NewWorkspaceModel builds the model. It starts the file watcher; call Close
// (via the returned model after Run) is not needed — the watcher is closed when
// the program exits through tea.Quit handling.
func NewWorkspaceModel(ws *workspace.Workspace, q leetcode.Question, editor string, runOnSave bool, debounceMs int, remote RemoteJudge) (*WorkspaceModel, error) {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := &WorkspaceModel{
		ws:        ws,
		q:         q,
		th:        DefaultTheme(),
		keys:      DefaultKeyMap(),
		editor:    editor,
		runOnSave: runOnSave,
		debounce:  time.Duration(debounceMs) * time.Millisecond,
		focused:   PaneCode,
		mode:      ModeNormal,
		spin:      sp,
		remote:    remote,
	}

	if r, ok := runner.For(ws.Lang); ok {
		if r.Available() {
			m.runner = r
		} else {
			m.runnerErr = fmt.Errorf("%s toolchain not found on PATH; press R to run on LeetCode instead", ws.Lang)
		}
	} else {
		m.runnerErr = fmt.Errorf("no local runner for %s yet; press R to run on LeetCode instead", ws.Lang)
	}

	src, err := ws.ReadSolution()
	if err != nil {
		return nil, err
	}
	m.codeSrc = src

	w, err := workspace.Watch(ws.SolutionPath)
	if err != nil {
		return nil, err
	}
	m.watcher = w

	return m, nil
}

// EnableImages turns on inline statement images using the given protocol and
// on-disk cache directory. Call before running the program.
func (m *WorkspaceModel) EnableImages(proto termimg.Protocol, cacheDir string) {
	m.imgProto = proto
	m.imgDir = cacheDir
}

// Init implements tea.Model.
func (m *WorkspaceModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick, m.waitForFileChange()}
	if c := m.loadImagesCmd(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

type imagesLoadedMsg struct{ byURL map[string]*termimg.Image }

// loadImagesCmd fetches and decodes the statement's images off the UI thread.
func (m *WorkspaceModel) loadImagesCmd() tea.Cmd {
	if m.imgProto == termimg.ProtoNone {
		return nil
	}
	urls := imageURLs(m.q.Statement)
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
			im, err := termimg.Decode(data)
			if err != nil {
				continue
			}
			out[u] = im
		}
		return imagesLoadedMsg{byURL: out}
	}
}

func allowedImageHost(rawURL string) bool {
	for _, h := range []string{
		"assets.leetcode.com", "assets.leetcode-cn.com",
		"leetcode.com", "leetcode.cn", "pic.leetcode.cn", "pic.leetcode-cn.com",
	} {
		if strings.Contains(rawURL, "://"+h+"/") {
			return true
		}
	}
	return false
}

func (m *WorkspaceModel) waitForFileChange() tea.Cmd {
	return func() tea.Msg {
		<-m.watcher.Events
		return fileChangedMsg{}
	}
}

func (m *WorkspaceModel) runCmd() tea.Cmd {
	ws := m.ws
	meta := m.q.Meta
	return func() tea.Msg {
		cases, err := ws.ReadCases()
		if err != nil {
			return runFinishedMsg{err: fmt.Errorf("read test cases: %w", err)}
		}
		res, err := m.runner.Run(context.Background(), runner.Spec{
			Lang:         ws.Lang,
			SolutionPath: ws.SolutionPath,
			Meta:         meta,
			Cases:        cases,
		})
		return runFinishedMsg{res: res, err: err}
	}
}

func (m *WorkspaceModel) editCmd() tea.Cmd {
	// Route through the shell so an editor command with flags ("code -w") works.
	line := fmt.Sprintf("%s %q", m.editor, m.ws.SolutionPath)
	c := exec.Command("sh", "-c", line)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return tea.ExecProcess(c, func(err error) tea.Msg { return editorFinishedMsg{err: err} })
}

func (m *WorkspaceModel) startRun() (tea.Model, tea.Cmd) {
	if m.runner == nil {
		m.statusMsg = m.runnerErr.Error()
		return m, nil
	}
	m.runGen++
	m.running = true
	m.lastErr = nil
	m.showRemote = false
	m.ranSrc = m.codeSrc
	m.statusMsg = ""
	m.refreshResults()
	return m, tea.Batch(m.runCmd(), m.spin.Tick)
}

func (m *WorkspaceModel) startRemote(kind string) (tea.Model, tea.Cmd) {
	if m.remote == nil || !m.remote.Available() {
		m.statusMsg = "run/submit on LeetCode needs `lazyleet auth`"
		return m, nil
	}
	if m.remoteRunning {
		return m, nil
	}
	m.remoteRunning = true
	m.remoteKind = kind
	m.remoteErr = nil
	m.showRemote = true
	m.statusMsg = ""
	m.refreshResults()
	return m, tea.Batch(m.remoteCmd(kind), m.spin.Tick)
}

func (m *WorkspaceModel) remoteCmd(kind string) tea.Cmd {
	ws := m.ws
	remote := m.remote
	input := m.remoteDataInput()
	return func() tea.Msg {
		code, err := ws.ReadSolution()
		if err != nil {
			return remoteDoneMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		var out RemoteOutcome
		if kind == "submit" {
			out, err = remote.Submit(ctx, code)
		} else {
			out, err = remote.Run(ctx, code, input)
		}
		return remoteDoneMsg{out: out, err: err}
	}
}

// remoteDataInput renders the local test cases as LeetCode's "Run Code" input:
// every argument literal on its own line, cases concatenated.
func (m *WorkspaceModel) remoteDataInput() string {
	cases, err := m.ws.ReadCases()
	if err != nil || len(cases) == 0 {
		return strings.TrimSpace(m.q.ExampleTestcases)
	}
	var lines []string
	for _, c := range cases {
		lines = append(lines, c.In...)
	}
	return strings.Join(lines, "\n")
}

// importFailingCase appends the remote judge's failing test case to
// testcases.jsonl and kicks off a local run.
func (m *WorkspaceModel) importFailingCase() (tea.Model, tea.Cmd) {
	if m.remoteOut == nil || strings.TrimSpace(m.remoteOut.LastCase) == "" {
		m.statusMsg = "no failing case to import"
		return m, nil
	}
	arity := m.q.Meta.Arity()
	if arity <= 0 {
		arity = 1
	}
	cases, err := testcase.FromLeetCodeExample(m.remoteOut.LastCase, arity)
	if err != nil || len(cases) == 0 {
		m.statusMsg = "could not parse the failing case"
		return m, nil
	}
	if err := m.ws.AppendCases(cases); err != nil {
		m.statusMsg = "import failed: " + err.Error()
		return m, nil
	}
	m.statusMsg = fmt.Sprintf("imported %d case(s) → testcases.jsonl", len(cases))
	return m.startRun()
}

// Update implements tea.Model.
func (m *WorkspaceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.relayout()
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		vp := m.focusedViewport()
		var cmd tea.Cmd
		*vp, cmd = vp.Update(msg)
		return m, cmd

	case fileChangedMsg:
		return m.handleFileChange()

	case runDebounceMsg:
		if msg.gen == m.runGen && !m.running {
			return m.startRun()
		}
		return m, nil

	case runFinishedMsg:
		m.running = false
		m.lastErr = msg.err
		if msg.err == nil {
			r := msg.res
			m.lastRun = &r
		}
		m.refreshResults()
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			m.statusMsg = "editor exited: " + msg.err.Error()
		}
		m.reloadCode()
		if m.runOnSave {
			return m.startRun()
		}
		return m, nil

	case imagesLoadedMsg:
		if len(msg.byURL) > 0 {
			m.stmtImages = &statementImages{proto: m.imgProto, byURL: msg.byURL}
			m.refreshStatement()
		}
		return m, nil

	case remoteDoneMsg:
		m.remoteRunning = false
		m.remoteErr = msg.err
		if msg.err == nil {
			o := msg.out
			m.remoteOut = &o
			m.statusMsg = m.remoteKind + ": " + o.Verdict
		}
		m.refreshResults()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if m.running || m.remoteRunning {
			m.refreshResults()
		}
		return m, cmd
	}

	return m, nil
}

func (m *WorkspaceModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.watcher.Close()
		return m, tea.Quit
	case key.Matches(msg, m.keys.Back):
		m.watcher.Close()
		return m, tea.Quit // Phase 2: return to browse mode instead
	case key.Matches(msg, m.keys.NextPane):
		m.focused = m.focused.Next()
		m.relayout()
		return m, nil
	case key.Matches(msg, m.keys.PrevPane):
		m.focused = m.focused.Prev()
		m.relayout()
		return m, nil
	case key.Matches(msg, m.keys.Zoom):
		if m.mode == ModeZoom {
			m.mode = ModeNormal
		} else {
			m.mode = ModeZoom
		}
		m.relayout()
		return m, nil
	case key.Matches(msg, m.keys.Edit):
		return m, m.editCmd()
	case key.Matches(msg, m.keys.Run):
		return m.startRun()
	case key.Matches(msg, m.keys.RunLC):
		return m.startRemote("run")
	case key.Matches(msg, m.keys.Submit):
		return m.startRemote("submit")
	case key.Matches(msg, m.keys.Import):
		return m.importFailingCase()
	case key.Matches(msg, m.keys.Tests):
		m.statusMsg = "test-case manager lands in Phase 4 — edit testcases.jsonl for now"
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.focusedViewport().ScrollUp(2)
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.focusedViewport().ScrollDown(2)
		return m, nil
	case key.Matches(msg, m.keys.PageUp):
		m.focusedViewport().PageUp()
		return m, nil
	case key.Matches(msg, m.keys.PageDown):
		m.focusedViewport().PageDown()
		return m, nil
	}
	return m, nil
}

func (m *WorkspaceModel) handleFileChange() (tea.Model, tea.Cmd) {
	m.reloadCode()
	m.statusMsg = "saved · " + time.Now().Format("15:04:05")
	cmds := []tea.Cmd{m.waitForFileChange()}
	if m.runOnSave {
		m.runGen++
		gen := m.runGen
		cmds = append(cmds, tea.Tick(m.debounce, func(time.Time) tea.Msg {
			return runDebounceMsg{gen: gen}
		}))
	}
	return m, tea.Batch(cmds...)
}

// --- rendering ---------------------------------------------------------------

func (m *WorkspaceModel) focusedViewport() *viewport.Model {
	switch m.focused {
	case PaneStatement:
		return &m.statement
	case PaneResults:
		return &m.results
	default:
		return &m.code
	}
}

func (m *WorkspaceModel) relayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.layout = Compute(m.width, m.height, m.focused, m.mode)

	set := func(vp *viewport.Model, r Rect) {
		iw, ih := innerSize(r)
		if vp.Width == 0 && vp.Height == 0 {
			*vp = viewport.New(iw, ih)
			vp.MouseWheelEnabled = true
		} else {
			vp.Width, vp.Height = iw, ih
		}
	}
	if m.layout.Tabbed {
		set(m.focusedViewport(), m.layout.RectFor(m.focused))
	} else {
		set(&m.statement, m.layout.Statement)
		set(&m.code, m.layout.Code)
		set(&m.results, m.layout.Results)
	}

	m.refreshStatement()
	m.refreshCode()
	m.refreshResults()
}

// innerSize is the content area inside a pane's border and title row.
func innerSize(r Rect) (w, h int) {
	w = r.W - 2 // border
	h = r.H - 3 // border (2) + title row (1)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return
}

func (m *WorkspaceModel) refreshStatement() {
	w := m.statement.Width
	if w < 1 {
		return
	}
	if m.stmtRenderer == nil || m.stmtWidth != w {
		r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(w))
		if err == nil {
			m.stmtRenderer = r
			m.stmtWidth = w
		}
	}
	body := renderStatementMD(m.stmtRenderer, m.q.Statement, w, m.stmtImages)
	m.statement.SetContent(strings.TrimRight(body, "\n"))
}

func (m *WorkspaceModel) refreshCode() {
	if m.code.Width < 1 {
		return
	}
	m.code.SetContent(gutter(highlightCode(m.codeSrc, m.ws.Lang), m.th.Muted))
}

func (m *WorkspaceModel) refreshResults() {
	if m.results.Width < 1 {
		return
	}
	var body string
	if m.showRemote {
		body = renderRemote(m.th, m.remoteKind, m.remoteOut, m.remoteErr, m.remoteRunning, m.spin.View(), m.results.Width)
	} else {
		body = renderResults(m.th, m.lastRun, m.lastErr, m.running, m.spin.View(), m.results.Width)
	}
	m.results.SetContent(body)
}

// codeChangedSinceRun reports whether the mirrored solution differs from what
// the last local run executed. False before any run.
func (m *WorkspaceModel) codeChangedSinceRun() bool {
	return m.ranSrc != "" && m.codeSrc != m.ranSrc
}

func (m *WorkspaceModel) reloadCode() {
	src, err := m.ws.ReadSolution()
	if err != nil {
		m.statusMsg = "reload failed: " + err.Error()
		return
	}
	m.codeSrc = src
	m.refreshCode()
}

// View implements tea.Model.
func (m *WorkspaceModel) View() string {
	if !m.ready {
		return "loading workspace…"
	}

	var body string
	if m.layout.Tabbed {
		body = m.renderPane(m.focused, m.layout.RectFor(m.focused), true)
	} else {
		cols := lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderPane(PaneStatement, m.layout.Statement, m.focused == PaneStatement),
			m.renderPane(PaneCode, m.layout.Code, m.focused == PaneCode),
			m.renderPane(PaneResults, m.layout.Results, m.focused == PaneResults),
		)
		body = cols
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderStatusBar())
}

func (m *WorkspaceModel) renderPane(p Pane, r Rect, focused bool) string {
	if r.Empty() {
		return ""
	}
	var vp viewport.Model
	switch p {
	case PaneStatement:
		vp = m.statement
	case PaneResults:
		vp = m.results
	default:
		vp = m.code
	}

	title := m.paneTitle(p, focused)
	inner := lipgloss.JoinVertical(lipgloss.Left, title, vp.View())

	border := m.th.PaneBorder
	if focused {
		border = m.th.PaneBorderFocused
	}
	return border.Width(r.W - 2).Height(r.H - 2).Render(inner)
}

func (m *WorkspaceModel) paneTitle(p Pane, focused bool) string {
	ts := m.th.Title
	if focused {
		ts = m.th.TitleFocused
	}
	if m.layout.Tabbed {
		var parts []string
		for _, q := range []Pane{PaneStatement, PaneCode, PaneResults} {
			label := " " + q.String() + " "
			if q == p {
				parts = append(parts, m.th.TitleFocused.Render("["+q.String()+"]"))
			} else {
				parts = append(parts, m.th.Title.Render(label))
			}
		}
		return strings.Join(parts, " ")
	}

	switch p {
	case PaneStatement:
		return ts.Render(fmt.Sprintf("%s  ", m.q.Title)) +
			DifficultyStyle(m.q.Difficulty).Render(m.q.Difficulty)
	case PaneCode:
		title := ts.Render(fmt.Sprintf("solution.%s", extOf(m.ws.SolutionPath)))
		if m.codeChangedSinceRun() {
			title += m.th.ErrorText.Render(" ● unrun")
		}
		return title
	case PaneResults:
		return ts.Render("Results")
	}
	return ts.Render(p.String())
}

func (m *WorkspaceModel) renderStatusBar() string {
	if m.layout.Status.Empty() {
		return ""
	}
	w := m.width

	left := ""
	if m.running {
		left = m.th.Spinner.Render(m.spin.View()) + " running "
	} else if m.statusMsg != "" {
		left = m.statusMsg + "  "
	}

	var segs []string
	for _, h := range m.keys.shortcutHints() {
		segs = append(segs, m.th.StatusKey.Render(h.key)+m.th.StatusBar.Render(" "+h.desc))
	}
	hints := strings.Join(segs, m.th.StatusDivider.Render(" │ "))

	line := left + hints
	if lipgloss.Width(line) > w {
		line = truncate(line, w)
	}
	return m.th.StatusBar.Width(w).Render(line)
}

func extOf(path string) string {
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return ""
	}
	return path[i+1:]
}

var _ tea.Model = (*WorkspaceModel)(nil)
