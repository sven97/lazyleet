package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sven97/lazyleet/internal/attempt"
	"github.com/sven97/lazyleet/internal/runner"
)

type historyLoadedMsg struct {
	slug       string
	generation int
	entries    []attempt.Entry
	err        error
}

func (m *WorkspaceModel) SetHistory(repo attempt.Repository) { m.history = repo }

func recordAttempt(repo attempt.Repository, e attempt.Entry) error {
	if repo == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return repo.RecordAttempt(ctx, e)
}

func localAttempt(slug, lang string, started time.Time, res runner.Result, err error) attempt.Entry {
	e := attempt.Entry{Slug: slug, Lang: lang, Kind: "local", CreatedAt: started, Passed: res.Passed, Total: res.Total, Runtime: res.Elapsed.Round(time.Millisecond).String()}
	var b strings.Builder
	switch {
	case err != nil:
		e.Verdict = "Run error"
		b.WriteString(err.Error() + "\n")
	case res.BuildErr != "":
		e.Verdict = "Build error"
		b.WriteString(res.BuildErr + "\n")
	case res.Total == 0:
		e.Verdict = "No test cases"
	case res.OK():
		e.Verdict = "Passed"
	default:
		e.Verdict = "Unverified"
		for _, c := range res.Cases {
			if c.Status == runner.StatusFail {
				e.Verdict = "Wrong answer"
			}
			if c.Status == runner.StatusError {
				e.Verdict = "Runtime error"
				break
			}
			if c.Status == runner.StatusTimeout {
				e.Verdict = "Time limit exceeded"
				break
			}
		}
	}
	for _, c := range res.Cases {
		fmt.Fprintf(&b, "Case %d · %s\nInput: %s\nExpected: %s\nActual: %s\n", c.Index+1, c.Status, strings.Join(c.Input, " | "), c.Expected, c.Actual)
		if c.Stdout != "" {
			fmt.Fprintf(&b, "Output: %s\n", c.Stdout)
		}
		if c.Err != "" {
			fmt.Fprintf(&b, "Error: %s\n", c.Err)
		}
		b.WriteByte('\n')
	}
	e.Detail = b.String()
	return e
}

func remoteAttempt(slug, lang, kind string, started time.Time, out RemoteOutcome, err error) attempt.Entry {
	e := attempt.Entry{Slug: slug, Lang: lang, Kind: kind, CreatedAt: started, Verdict: out.Verdict, Passed: out.Passed, Total: out.Total, Runtime: out.Runtime, Memory: out.Memory, RemoteID: out.RemoteID}
	var b strings.Builder
	if err != nil {
		e.Verdict = "Request error"
		b.WriteString(err.Error() + "\n")
	}
	if e.Verdict == "" {
		e.Verdict = "Unknown"
	}
	if out.LastCase != "" {
		fmt.Fprintf(&b, "Failing input:\n%s\n", out.LastCase)
	}
	if len(out.Expected) > 0 {
		fmt.Fprintf(&b, "Expected:\n%s\n", strings.Join(out.Expected, "\n"))
	}
	if len(out.Actual) > 0 {
		fmt.Fprintf(&b, "Actual:\n%s\n", strings.Join(out.Actual, "\n"))
	}
	if len(out.Stdout) > 0 {
		fmt.Fprintf(&b, "Output:\n%s\n", strings.Join(out.Stdout, "\n"))
	}
	if out.CompileErr != "" {
		fmt.Fprintf(&b, "Compile error:\n%s\n", out.CompileErr)
	}
	if out.RuntimeErr != "" {
		fmt.Fprintf(&b, "Runtime error:\n%s\n", out.RuntimeErr)
	}
	e.Detail = b.String()
	return e
}

func (m *WorkspaceModel) loadHistory() tea.Cmd {
	repo, slug := m.history, m.ws.Slug
	m.historyGeneration++
	generation := m.historyGeneration
	m.historyLoading = true
	return func() tea.Msg {
		if repo == nil {
			return historyLoadedMsg{slug: slug, generation: generation, err: fmt.Errorf("attempt history is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		entries, err := repo.RecentAttempts(ctx, slug, 50)
		return historyLoadedMsg{slug: slug, generation: generation, entries: entries, err: err}
	}
}

func (m *WorkspaceModel) handleHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.historyDetail {
		switch msg.String() {
		case "esc", "b", "q":
			m.historyDetail = false
			return m, nil
		}
		var cmd tea.Cmd
		m.historyVP, cmd = m.historyVP.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "esc", "a", "b", "q":
		m.showHistory = false
		return m, nil
	case "up", "k":
		m.historyCursor = max(0, m.historyCursor-1)
	case "down", "j":
		m.historyCursor = min(max(0, len(m.historyEntries)-1), m.historyCursor+1)
	case "r":
		if !m.historyLoading {
			return m, m.loadHistory()
		}
	case "enter":
		if len(m.historyEntries) > 0 {
			m.historyDetail = true
			m.sizeHistoryDetail()
		}
	}
	return m, nil
}

// sizeHistoryDetail (re)builds the detail viewport for the currently
// selected history entry, sized to the current terminal dimensions. It
// always starts scroll at the top, so it's for opening detail view (or
// switching to a different entry) — not for a plain resize while an entry
// is already open; use resizeHistoryDetail for that so YOffset survives.
func (m *WorkspaceModel) sizeHistoryDetail() {
	if !m.historyDetail || len(m.historyEntries) == 0 {
		return
	}
	m.historyVP = viewport.New(max(1, m.width), max(1, m.height-3))
	m.historyVP.MouseWheelEnabled = true
	m.setHistoryDetailContent()
}

// resizeHistoryDetail adjusts the existing history-detail viewport to the
// current terminal dimensions, following the same convention as relayout():
// construct the viewport once, and on subsequent resizes just update its
// Width/Height so scroll position (YOffset) is preserved.
func (m *WorkspaceModel) resizeHistoryDetail() {
	if !m.historyDetail || len(m.historyEntries) == 0 {
		return
	}
	if m.historyVP.Width == 0 && m.historyVP.Height == 0 {
		m.sizeHistoryDetail()
		return
	}
	m.historyVP.Width, m.historyVP.Height = max(1, m.width), max(1, m.height-3)
	m.setHistoryDetailContent()
}

// setHistoryDetailContent (re)wraps and sets the viewport's content for the
// currently selected entry at the viewport's current width, without
// touching YOffset (SetContent only clamps it if it now exceeds the content).
func (m *WorkspaceModel) setHistoryDetailContent() {
	e := m.historyEntries[m.historyCursor]
	body := fmt.Sprintf("%s · %s · %s\n%s\n%d/%d passed\n", e.CreatedAt.Local().Format("2006-01-02 15:04:05"), e.Kind, e.Lang, e.Verdict, e.Passed, e.Total)
	if e.Runtime != "" {
		body += "Runtime: " + e.Runtime + "\n"
	}
	if e.Memory != "" {
		body += "Memory: " + e.Memory + "\n"
	}
	if e.RemoteID != "" {
		body += "Judge ID: " + e.RemoteID + "\n"
	}
	m.historyVP.SetContent(ansi.Hardwrap(ansi.Strip(body+"\n"+e.Detail), m.historyVP.Width, true))
}

func (m *WorkspaceModel) historyView() string {
	var b strings.Builder
	b.WriteString(m.th.TitleFocused.Render(truncate("Attempt history · "+m.q.Title, m.width)) + "\n")
	if m.historyDetail {
		b.WriteString(m.historyVP.View() + "\nesc back · ↑/↓ scroll")
	} else {
		b.WriteByte('\n')
		switch {
		case m.historyLoading:
			b.WriteString("Loading recent attempts…\n")
		case m.historyErr != nil:
			b.WriteString("History unavailable: " + m.historyErr.Error() + "\n")
		case len(m.historyEntries) == 0:
			b.WriteString("No attempts yet. Run local tests or use the remote judge.\n")
		default:
			rows := max(1, m.height-6)
			start := max(0, m.historyCursor-rows+1)
			for i := start; i < min(len(m.historyEntries), start+rows); i++ {
				e := m.historyEntries[i]
				prefix := "  "
				if i == m.historyCursor {
					prefix = "› "
				}
				line := fmt.Sprintf("%s%s  %-6s %-10s %s · %d/%d", prefix, e.CreatedAt.Local().Format("01-02 15:04"), e.Kind, e.Lang, e.Verdict, e.Passed, e.Total)
				line = truncate(line, m.width)
				if i == m.historyCursor {
					line = m.th.TitleFocused.Render(line)
				}
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\nLatest 50 · ↑/↓ select · enter details · r reload · esc back")
	}
	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = truncate(line, m.width)
	}
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(strings.Join(lines, "\n"))
}
