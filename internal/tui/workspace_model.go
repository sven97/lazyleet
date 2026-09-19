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

	"github.com/sven97/lazyleet/internal/attempt"
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

	// sel is an in-progress or just-finished click-drag text selection (F1)
	// over whichever of Statement/Code/Results it started in (selPane) — see
	// selection.go. hintsSel is the analogous state for the Hints overlay
	// (hints.go), kept separate since it isn't one of the three numbered
	// panes and has its own (border-less) screen geometry.
	sel      selectionState
	selPane  Pane
	hintsSel selectionState

	stmtRenderer *glamour.TermRenderer
	stmtWidth    int
	stmtImages   *statementImages
	imgProto     termimg.Protocol
	imgDir       string
	imgWriter    *ImageWriter
	imgWritten   string
	codeSrc      string
	ranSrc       string // codeSrc as of the last local run started

	runner        runner.Runner
	runnerErr     error // why there is no local runner for this language, if so
	running       bool
	runGen        int
	spin          spinner.Model
	lastRun       *runner.Result
	casesRevision int
	casesUnrun    bool
	lastErr       error

	// remote run / submit against LeetCode
	remote        RemoteJudge
	remoteRunning bool
	remoteKind    string // "run" | "submit"
	remoteOut     *RemoteOutcome
	remoteErr     error
	showRemote    bool // Results pane is showing the remote outcome, not local
	// remoteRunCases is exactly what was sent as a Run Code request's data
	// input, captured at send time. Rendering pairs it with the response's
	// Expected/Actual by index for the input column — reconstructing it by
	// parsing the response back apart (LeetCode's last_testcase isn't
	// reliably populated for a Run Code request the way it is for a real
	// submission's one failing case) turned out not to work.
	remoteRunCases []testcase.Case

	watcher            *workspace.Watcher
	statusMsg          string
	showHelp           bool
	showHints          bool
	hintsRevealed      int
	hints              viewport.Model
	hintsRenderer      *glamour.TermRenderer
	hintsRendererWidth int
	cases              *caseManager
	history            attempt.Repository
	remoteHistory      RemoteHistory
	showHistory        bool
	historyGeneration  int
	historyLoading     bool
	historyDetail      bool
	historyCursor      int
	historyEntries     []attempt.Entry
	historyErr         error
	historyVP          viewport.Model

	// pendingRunNote overrides the generic "local: N/M passed" status-bar
	// summary the next time a run finishes — used by an auto-triggered run
	// (e.g. after importing a failing case) whose own confirmation message
	// is more specific and should win instead of being clobbered.
	pendingRunNote string

	// returnToBrowse makes Back/Quit emit BackToBrowseMsg instead of tea.Quit
	// so an owning AppModel can restore browse mode.
	returnToBrowse bool

	// progressChanged is set once an accepted submission has changed this
	// problem's cached solve status, so the owning AppModel knows browse's
	// problem list/daily streak actually need reloading on close instead of
	// unconditionally refetching on every visit.
	progressChanged bool
}

// ProgressChanged reports whether an accepted submission during this
// workspace session changed the cached solve status.
func (m *WorkspaceModel) ProgressChanged() bool { return m.progressChanged }

type fileChangedMsg struct{}
type runDebounceMsg struct{ gen int }
type runFinishedMsg struct {
	casesRevision int
	historyErr    error
	res           runner.Result
	err           error
}
type editorFinishedMsg struct{ err error }
type remoteDoneMsg struct {
	historyErr error
	out        RemoteOutcome
	err        error
}

// withHistoryErr folds a history-save failure into a status message without
// clobbering a verdict summary already sitting there: with no verdict it's
// just the save failure, otherwise both are kept on one line.
func withHistoryErr(verdict string, historyErr error) string {
	if historyErr == nil {
		return verdict
	}
	if verdict == "" {
		return "could not save attempt history: " + historyErr.Error()
	}
	return verdict + " (history not saved: " + historyErr.Error() + ")"
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

// Close stops the file watcher. Safe to call more than once.
func (m *WorkspaceModel) Close() {
	if m == nil || m.watcher == nil {
		return
	}
	w := m.watcher
	m.watcher = nil
	w.Close()
}

// EnableImages turns on inline statement images. iw must be the same
// ImageWriter passed to tea.WithOutput. Call before running the program.
func (m *WorkspaceModel) EnableImages(proto termimg.Protocol, cacheDir string, iw *ImageWriter) {
	m.imgProto = proto
	m.imgDir = cacheDir
	m.imgWriter = iw
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
	if m.watcher == nil {
		return nil
	}
	// Snapshot the channel now, synchronously, rather than reading m.watcher
	// inside the returned closure: Close() runs on the Update goroutine and
	// may nil out m.watcher before the tea runtime gets around to invoking
	// this command, which would otherwise nil-deref.
	events := m.watcher.Events
	return func() tea.Msg {
		if _, ok := <-events; !ok {
			// Watcher was closed (workspace torn down); don't re-arm.
			return nil
		}
		return fileChangedMsg{}
	}
}

func (m *WorkspaceModel) runCmd() tea.Cmd {
	ws := m.ws
	meta := m.q.Meta
	repo, driver := m.history, m.runner
	revision := m.casesRevision
	return func() tea.Msg {
		started := time.Now()
		finish := func(res runner.Result, err error) tea.Msg {
			historyErr := recordAttempt(repo, localAttempt(ws.Slug, ws.Lang, started, res, err))
			return runFinishedMsg{casesRevision: revision, res: res, err: err, historyErr: historyErr}
		}
		cases, err := ws.ReadCases()
		if err != nil {
			return finish(runner.Result{}, fmt.Errorf("read test cases: %w", err))
		}
		res, err := driver.Run(context.Background(), runner.Spec{
			Lang:         ws.Lang,
			SolutionPath: ws.SolutionPath,
			Meta:         meta,
			Cases:        cases,
		})
		return finish(res, err)
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
	m.casesUnrun = false
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
		m.statusMsg = "run `lazyleet auth` in another terminal, then retry R/s"
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
	if kind == "run" {
		m.remoteRunCases, _ = m.ws.ReadCases()
	}
	m.refreshResults()
	return m, tea.Batch(m.remoteCmd(kind), m.spin.Tick)
}

func (m *WorkspaceModel) remoteCmd(kind string) tea.Cmd {
	ws := m.ws
	remote := m.remote
	repo := m.history
	input := m.remoteDataInput()
	return func() tea.Msg {
		started := time.Now()
		finish := func(out RemoteOutcome, err error) tea.Msg {
			historyErr := recordAttempt(repo, remoteAttempt(ws.Slug, ws.Lang, kind, started, out, err))
			return remoteDoneMsg{out: out, err: err, historyErr: historyErr}
		}
		code, err := ws.ReadSolution()
		if err != nil {
			return finish(RemoteOutcome{}, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		var out RemoteOutcome
		if kind == "submit" {
			out, err = remote.Submit(ctx, code)
		} else {
			out, err = remote.Run(ctx, code, input)
		}
		return finish(out, err)
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
	added, err := m.ws.AppendCases(cases)
	if err != nil {
		m.statusMsg = "import failed: " + err.Error()
		return m, nil
	}
	msg := fmt.Sprintf("imported %d case(s) → testcases.jsonl", added)
	if added == 0 {
		msg = "already have this case in testcases.jsonl"
	} else {
		// testcases.jsonl changed on disk: invalidate the same way add/edit/
		// delete do (reloadCaseList's invalidate=true path), so an in-flight
		// run started before this import gets discarded as stale by the
		// runFinishedMsg guard instead of overwriting the post-import result.
		// Must happen before startRun() below so the run it kicks off is
		// itself stamped with the post-import revision.
		m.invalidateCases()
	}
	// startRun() clears statusMsg as part of kicking off a normal run, and the
	// run's own completion normally overwrites it with a pass/fail summary —
	// pendingRunNote overrides that once so this confirmation is what's left
	// once the run settles, instead of being clobbered either way. Only defer
	// it if a run actually started (m.runner == nil leaves m.running false
	// and no runFinishedMsg will ever arrive to consume it).
	model, cmd := m.startRun()
	m.statusMsg = msg
	if m.running {
		m.pendingRunNote = msg
	}
	return model, cmd
}

// Update implements tea.Model.
func (m *WorkspaceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if debugScroll {
		if _, isTick := msg.(spinner.TickMsg); !isTick {
			scrollLogf("workspace Update <- %T  focused=%v", msg, m.focused)
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.relayout()
		m.sizeCaseForm()
		m.ready = true
		if m.showHints {
			m.sizeHints()
		}
		if m.historyDetail {
			m.resizeHistoryDetail()
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case fileChangedMsg:
		return m.handleFileChange()

	case runDebounceMsg:
		if msg.gen == m.runGen && !m.running {
			return m.startRun()
		}
		return m, nil

	case runFinishedMsg:
		m.running = false
		if msg.casesRevision != m.casesRevision {
			m.lastRun = nil
			m.lastErr = nil
			m.statusMsg = "test cases changed · press r to run again"
			m.refreshResults()
			return m, nil
		}
		m.lastErr = msg.err
		if msg.err == nil {
			r := msg.res
			m.lastRun = &r
		}
		switch {
		case m.pendingRunNote != "":
			m.statusMsg, m.pendingRunNote = m.pendingRunNote, ""
		case msg.err != nil:
			m.statusMsg = "local run failed: " + msg.err.Error()
		default:
			// Mirrors the "run: <verdict>" / "submit: <verdict>" echo remote
			// run/submit already leave in the status bar, so a local run is
			// just as visible at a glance if you're not looking at Results.
			m.statusMsg = localRunSummary(*m.lastRun)
		}
		m.statusMsg = withHistoryErr(m.statusMsg, msg.historyErr)
		m.refreshResults()
		if m.showHistory && !m.historyDetail {
			return m, m.loadHistory()
		}
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
			if m.remoteKind == "submit" && o.Accepted {
				m.progressChanged = true
			}
		}
		m.statusMsg = withHistoryErr(m.statusMsg, msg.historyErr)
		m.refreshResults()
		if m.showHistory && !m.historyDetail {
			return m, m.loadHistory()
		}
		return m, nil

	case historyLoadedMsg:
		if msg.slug != m.ws.Slug || msg.generation != m.historyGeneration {
			return m, nil
		}
		m.historyLoading = false
		m.historyErr = msg.err
		if msg.err == nil {
			m.historyEntries = msg.entries
			m.historyCursor = min(m.historyCursor, max(0, len(msg.entries)-1))
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if m.running || m.remoteRunning {
			m.refreshResults()
		}
		return m, cmd
	}

	if m.cases != nil && m.cases.editing {
		var cmd tea.Cmd
		if m.cases.focus == 0 {
			m.cases.inputs, cmd = m.cases.inputs.Update(msg)
		} else {
			m.cases.expected, cmd = m.cases.expected.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m *WorkspaceModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHints {
		return m.handleHintsKey(msg)
	}
	if m.showHistory {
		return m.handleHistoryKey(msg)
	}
	if m.cases != nil {
		return m.handleCaseKey(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Back):
		m.Close()
		if m.returnToBrowse {
			return m, func() tea.Msg { return BackToBrowseMsg{} }
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case m.showHelp && msg.String() == "esc":
		m.showHelp = false
		return m, nil
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
	case key.Matches(msg, m.keys.Hints):
		return m.openHints()
	case key.Matches(msg, m.keys.History):
		m.showHistory = true
		m.historyDetail = false
		return m, m.loadHistory()
	case key.Matches(msg, m.keys.Tests):
		return m.openCaseManager()
	case key.Matches(msg, m.keys.Copy):
		// F1 keyboard fallback: copy the whole focused pane's visible content
		// when there's no active drag selection to copy instead (e.g.
		// terminals without OSC 52 support).
		if m.sel.hasSelection && m.selPane == m.focused {
			m.copySelection()
		} else {
			m.copyText(plainViewportText(m.focusedViewport().View()))
		}
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.sel.clear()
		m.focusedViewport().ScrollUp(2)
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.sel.clear()
		m.focusedViewport().ScrollDown(2)
		return m, nil
	case key.Matches(msg, m.keys.PageUp):
		m.sel.clear()
		m.focusedViewport().PageUp()
		return m, nil
	case key.Matches(msg, m.keys.PageDown):
		m.sel.clear()
		m.focusedViewport().PageDown()
		return m, nil
	case key.Matches(msg, m.keys.Top):
		m.sel.clear()
		m.focusedViewport().GotoTop()
		return m, nil
	case key.Matches(msg, m.keys.Bottom):
		m.sel.clear()
		m.focusedViewport().GotoBottom()
		return m, nil
	}

	// F2 digit-jump: 1-3 focus Statement/Code/Results straight away, from any
	// pane, mirroring lazygit's numbered panels. While zoomed/tabbed this
	// switches *which* pane is zoomed (m.mode is left untouched) rather than
	// no-opping or exiting zoom, matching lazygit's own screen-mode behavior.
	// Gated on !showHelp like the other modal guards above; the rect-empty
	// guard mirrors browse's equivalent (see browse_model.go) — in practice
	// Compute() never actually leaves a pane's rect empty (tabbed mode gives
	// every pane the full work area) but the check stays meaningful either way.
	if !m.showHelp {
		if n, ok := digitKey(msg); ok {
			if p, ok := paneForNumber(n); ok && !m.layout.RectFor(p).Empty() {
				m.focused = p
				m.relayout()
				return m, nil
			}
		}
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

func (m *WorkspaceModel) focusedViewport() *viewport.Model { return m.paneViewport(m.focused) }

func (m *WorkspaceModel) paneViewport(p Pane) *viewport.Model {
	switch p {
	case PaneStatement:
		return &m.statement
	case PaneResults:
		return &m.results
	default:
		return &m.code
	}
}

// paneAt returns the pane under the given terminal cell.
func (m *WorkspaceModel) paneAt(x, y int) (Pane, bool) {
	if m.layout.Tabbed {
		return m.focused, true
	}
	for _, p := range []Pane{PaneStatement, PaneCode, PaneResults} {
		if m.layout.RectFor(p).Contains(x, y) {
			return p, true
		}
	}
	return 0, false
}

// wheelAtEdge reports whether a wheel event would do nothing: the pane under the
// cursor is already at the top (wheel-up) or bottom (wheel-down) of its content.
// See WheelEdgeFilter.
func (m *WorkspaceModel) wheelAtEdge(msg tea.MouseMsg) bool {
	if m.showHints {
		if msg.Button == tea.MouseButtonWheelUp {
			return m.hints.AtTop()
		}
		return m.hints.AtBottom()
	}
	if m.showHistory {
		if !m.historyDetail {
			return true
		}
		if msg.Button == tea.MouseButtonWheelUp {
			return m.historyVP.AtTop()
		}
		return m.historyVP.AtBottom()
	}
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return true // horizontal wheel never scrolls a vertical pane
	}
	p, ok := m.paneAt(msg.X, msg.Y)
	if !ok {
		return true
	}
	vp := m.paneViewport(p)
	if msg.Button == tea.MouseButtonWheelUp {
		return vp.AtTop()
	}
	return vp.AtBottom()
}

func (m *WorkspaceModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// A drag already in progress on one of the three numbered panes: route
	// its motion/release to the pane it started in regardless of which pane
	// the cursor is over now (dragging past a pane's edge still extends the
	// selection, matching normal terminal drag-select) rather than
	// re-resolving paneAt below. Any other action (in practice, a stray
	// press with no matching release — mouse protocols always pair the two
	// for a real drag) falls through to normal click dispatch instead of
	// being swallowed; begin()/relayout() below reset sel.active cleanly
	// either way.
	//
	// Deliberately checked BEFORE the showHints/showHistory/cases overlay
	// guards below: a drag can start on a numbered pane and then have an
	// overlay opened over it via the keyboard (H, a, t) before the button is
	// released. If the release were swallowed by one of those guards
	// instead, sel.active would be left stuck true with no way to finish or
	// clear it until the next click — this lets an in-flight drag always
	// reach its release, regardless of what opened in the meantime.
	if m.sel.active && (msg.Action == tea.MouseActionMotion || msg.Action == tea.MouseActionRelease) {
		return m.continueSelection(msg)
	}

	if m.showHints {
		return m.handleHintsMouse(msg)
	}
	if m.showHistory {
		if m.historyDetail {
			var cmd tea.Cmd
			m.historyVP, cmd = m.historyVP.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	if m.cases != nil {
		return m, nil
	}

	p, ok := m.paneAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	switch {
	case tea.MouseEvent(msg).IsWheel():
		// Only vertical wheel scrolls; ignore trackpad WheelLeft/Right noise.
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.sel.clear()
			m.paneViewport(p).ScrollUp(wheelScrollLines)
		case tea.MouseButtonWheelDown:
			m.sel.clear()
			m.paneViewport(p).ScrollDown(wheelScrollLines)
		}
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if m.focused != p {
			m.focused = p
			m.relayout()
		}
		if lx, ly, ok := localCell(msg.X, msg.Y, m.layout.RectFor(p)); ok {
			m.selPane = p
			m.sel.begin(lx, ly)
		}
	}
	return m, nil
}

// continueSelection routes an in-progress drag's motion and release events
// (see handleMouse) to the pane it started in (selPane) — coordinates are
// clamped to that pane's body rather than re-validated, so dragging past the
// border still extends the selection instead of freezing it.
func (m *WorkspaceModel) continueSelection(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	r := m.layout.RectFor(m.selPane)
	lx, ly := clampCell(msg.X, msg.Y, r)
	if msg.Action == tea.MouseActionRelease {
		m.sel.end(lx, ly)
		m.copySelection()
		return m, nil
	}
	m.sel.extend(lx, ly)
	return m, nil
}

// copySelection finalizes a finished selection over selPane: copies its
// plain text to the system clipboard via OSC 52 and leaves a one-line
// confirmation in the status bar. A no-op for a plain click (no drag) — see
// selectionState.end.
func (m *WorkspaceModel) copySelection() {
	if !m.sel.hasSelection {
		return
	}
	vp := m.paneViewport(m.selPane)
	m.copyText(selectedText(strings.Split(vp.View(), "\n"), m.sel))
}

// copyText writes text to the system clipboard (OSC 52, via the same
// *ImageWriter already wired in for out-of-band escape injection — see
// imgwriter.go) and leaves a one-line status-bar confirmation, reusing the
// same statusMsg mechanism run/submit/sync results already report through.
func (m *WorkspaceModel) copyText(text string) {
	if msg := copyToClipboard(m.imgWriter, text); msg != "" {
		m.statusMsg = msg
	}
}

func (m *WorkspaceModel) relayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.sel.clear() // pane geometry is about to change under it
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

// innerSize is the content area inside a pane's border — the title lives in
// the top border row now (see numberedFrame in frame.go), not a separate
// line, so there's nothing else to subtract.
func innerSize(r Rect) (w, h int) {
	w = r.W - 2 // border
	h = r.H - 2 // border
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
		if r := newStatementRenderer(w); r != nil {
			m.stmtRenderer = r
			m.stmtWidth = w
		}
	}
	header := renderProblemHeader(m.th, ProblemMeta{
		Difficulty: m.q.Difficulty,
		ACRate:     m.q.ACRate,
		PaidOnly:   m.q.PaidOnly,
		Tags:       m.q.Tags,
	}, w)
	body, prefix := renderStatementMD(m.stmtRenderer, m.q.Statement, w, m.stmtImages)
	m.statement.SetContent(strings.TrimRight(header+body, "\n"))
	if m.selPane == PaneStatement {
		m.sel.clear()
	}
	m.imgWritten = queueImagePrefix(m.imgWriter, prefix, m.imgWritten)
}

func (m *WorkspaceModel) refreshCode() {
	if m.code.Width < 1 {
		return
	}
	m.code.SetContent(gutter(highlightCode(m.codeSrc, m.ws.Lang), m.th.Muted))
	if m.selPane == PaneCode {
		m.sel.clear()
	}
}

func (m *WorkspaceModel) refreshResults() {
	if m.results.Width < 1 {
		return
	}
	var body string
	if m.showRemote {
		body = renderRemote(m.th, m.remoteKind, m.remoteOut, m.remoteErr, m.remoteRunning, m.spin.View(), m.results.Width, m.remoteRunCases)
	} else {
		body = renderResults(m.th, m.lastRun, m.lastErr, m.running, m.spin.View(), m.results.Width)
	}
	m.results.SetContent(body)
	if m.selPane == PaneResults {
		m.sel.clear()
	}
}

// codeChangedSinceRun reports whether the solution or managed test cases have
// changed since the last local run.
func (m *WorkspaceModel) codeChangedSinceRun() bool {
	return m.casesUnrun || (m.ranSrc != "" && m.codeSrc != m.ranSrc)
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
func (m *WorkspaceModel) View() (out string) {
	if debugScroll {
		defer func() { traceView("workspace", out) }()
	}
	if !m.ready {
		return "loading workspace…"
	}
	if m.showHints {
		return m.hintsView()
	}
	if m.showHistory {
		return m.historyView()
	}
	if m.cases != nil {
		return m.caseManagerView()
	}
	if m.showHelp {
		return m.renderHelp()
	}

	var body string
	if m.layout.Tabbed {
		body = m.renderPane(m.focused, m.layout.RectFor(m.focused), true)
	} else {
		right := lipgloss.JoinVertical(lipgloss.Left,
			m.renderPane(PaneCode, m.layout.Code, m.focused == PaneCode),
			m.renderPane(PaneResults, m.layout.Results, m.focused == PaneResults),
		)
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderPane(PaneStatement, m.layout.Statement, m.focused == PaneStatement),
			right,
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderStatusBar())
}

// renderPane draws one bordered pane. In the normal (two-column) layout that
// means numberedFrame's usual "[N]-Title" top border. In the tabbed layout
// (narrow terminal or zoom — only one pane is ever visible) the top border
// instead carries a strip naming every pane, the focused one bracketed, via
// tabStripContent — that's the only way to still see the other two panes'
// numbers to jump to.
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

	border, title := m.th.PaneBorder, m.th.Title
	if focused {
		border, title = m.th.PaneBorderFocused, m.th.TitleFocused
	}

	body := vp.View()
	if p == m.selPane {
		body = applySelectionHighlight(body, m.sel, m.th.Selection)
	}

	if m.layout.Tabbed {
		bd := border.GetBorderStyle()
		top := borderContentRow(bd.TopLeft, bd.Top, bd.TopRight, lineStyle(border, true), m.tabStripContent(), r.W)
		bottom := borderContentRow(bd.BottomLeft, bd.Bottom, bd.BottomRight, lineStyle(border, false), "", r.W)
		return joinFrame(border, top, bottom, r, body)
	}
	return numberedFrame(border, title, p.Number(), m.paneTitleText(p, title), r, body)
}

// tabStripContent lists every workspace pane's number and name for the top
// border in tabbed mode, the focused one bracketed — e.g.
// "1:Statement  [2:Code]  3:Results" — so the other two panes' digits stay
// visible as a reminder of what pressing them (or Tab) switches to.
func (m *WorkspaceModel) tabStripContent() string {
	var parts []string
	for _, q := range []Pane{PaneStatement, PaneCode, PaneResults} {
		label := fmt.Sprintf("%d:%s", q.Number(), q.String())
		if q == m.focused {
			parts = append(parts, m.th.TitleFocused.Render("["+label+"]"))
		} else {
			parts = append(parts, m.th.Title.Render(" "+label+" "))
		}
	}
	return strings.Join(parts, " ")
}

// paneTitleText builds pane p's title, styled with ts (the same Title/
// TitleFocused the pane's number bracket uses) — the text embedded in the
// top border to the right of "[N]" via numberedFrame. PaneCode's "unrun"
// marker keeps its own ErrorText color rather than inheriting ts, which is
// why this returns an already-styled string rather than plain text for
// numberedFrame to wrap itself.
func (m *WorkspaceModel) paneTitleText(p Pane, ts lipgloss.Style) string {
	switch p {
	case PaneStatement:
		return ts.Render(problemTitle(m.q.FrontendID, m.q.Title, m.q.PaidOnly))
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

	status := ""
	if m.running {
		status = m.th.Spinner.Render(m.spin.View()) + " running"
	} else if m.statusMsg != "" {
		status = m.statusMsg
	}

	var segs []string
	for _, h := range m.keys.shortcutHints(m.focused) {
		segs = append(segs, m.th.StatusKey.Render(h.key)+m.th.StatusBar.Render(" "+h.desc))
	}
	hints := strings.Join(segs, m.th.StatusDivider.Render(" │ "))

	line := renderSplitStatusLine(hints, status, w)
	return m.th.StatusBar.Width(w).Render(line)
}

// renderHelp is the full-screen reference shown while showHelp is set —
// workspace mode has no other place a user can see the complete keymap: the
// status bar only ever has room for the handful of most-used bindings.
func (m *WorkspaceModel) renderHelp() string {
	pairs := [][2]string{
		{"↑/k ↓/j", "scroll the focused pane"},
		{"ctrl+u / ctrl+d", "page up / down"},
		{"g / G", "jump to top / bottom"},
		{"tab / ⇧tab", "next / prev pane (h / l also work)"},
		{"1-3", "jump straight to Statement / Code / Results"},
		{"z", "zoom the focused pane"},
		{"e", "edit in $EDITOR"},
		{"r", "run local tests"},
		{"R", "run on LeetCode (needs `lazyleet auth`)"},
		{"s", "submit to LeetCode (needs `lazyleet auth`)"},
		{"i", "import the last failing case as a local test"},
		{"t", "manage test cases (add, edit, delete)"},
		{"H", "open hints (hidden until you reveal them)"},
		{"a", "recent local and remote attempt history"},
		{"click+drag", "select text in Statement/Code/Results (copies on release)"},
		{"y", "copy the focused pane's selection, or all of it if none"},
		{"q", "back to browse (b also works)"}, {"?", "toggle this help"},
	}
	var b strings.Builder
	b.WriteString(m.th.TitleFocused.Render("lazyleet — workspace") + "\n\n")
	for _, p := range pairs {
		b.WriteString(fmt.Sprintf("  %s  %s\n", m.th.StatusKey.Render(fmt.Sprintf("%-16s", p[0])), p[1]))
	}
	b.WriteString("\n" + m.th.Muted.Render("press ? or esc to return"))
	return b.String()
}

func extOf(path string) string {
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return ""
	}
	return path[i+1:]
}

var _ tea.Model = (*WorkspaceModel)(nil)
