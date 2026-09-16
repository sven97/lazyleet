package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *WorkspaceModel) openHints() (tea.Model, tea.Cmd) {
	m.showHints = true
	m.sizeHints()
	return m, nil
}

func (m *WorkspaceModel) sizeHints() {
	w, h := max(1, m.width-2), max(1, m.height-3)
	if m.hints.Width == 0 && m.hints.Height == 0 {
		m.hints = viewport.New(w, h)
		m.hints.MouseWheelEnabled = true
	} else {
		m.hints.Width, m.hints.Height = w, h
	}
	m.refreshHints()
}

func (m *WorkspaceModel) refreshHints() {
	var b strings.Builder
	switch {
	case !m.q.HintsFetched:
		// The cache predates hints support (or hints simply haven't been
		// fetched yet) — we don't actually know if this problem has hints.
		b.WriteString("Hints haven't been cached yet for this problem.\n\nTo fetch them, reopen with:\n\nlazyleet solve " + m.q.Slug + " --refresh\n")
	case len(m.q.Hints) == 0:
		// Hints were fetched and this problem genuinely has none; refreshing
		// would never change that.
		b.WriteString("This problem has no hints on LeetCode.\n")
	case m.hintsRevealed == 0:
		b.WriteString("Hints are hidden. Press Enter to reveal the first hint.\n")
	default:
		for i, hint := range m.q.Hints[:m.hintsRevealed] {
			fmt.Fprintf(&b, "## Hint %d\n\n%s\n\n", i+1, hint)
		}
		if m.hintsRevealed < len(m.q.Hints) {
			b.WriteString("Press Enter to reveal the next hint.\n")
		} else {
			b.WriteString("All hints revealed.\n")
		}
	}
	w := m.hints.Width
	if m.hintsRenderer == nil || m.hintsRendererWidth != w {
		if r := newStatementRenderer(w); r != nil {
			m.hintsRenderer = r
			m.hintsRendererWidth = w
		}
	}
	body, _ := renderStatementMD(m.hintsRenderer, b.String(), w, nil)
	m.hints.SetContent(strings.TrimRight(body, "\n"))
}

func (m *WorkspaceModel) handleHintsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "h", "q", "b":
		m.showHints = false
		return m, nil
	case "enter", "n":
		if m.hintsRevealed < len(m.q.Hints) {
			m.hintsRevealed++
			m.refreshHints()
		}
		return m, nil
	case "H":
		m.hintsRevealed = 0
		m.refreshHints()
		m.hints.GotoTop()
		return m, nil
	}
	var cmd tea.Cmd
	m.hints, cmd = m.hints.Update(msg)
	return m, cmd
}

func (m *WorkspaceModel) hintsView() string {
	title := fmt.Sprintf("Hints · %s · %d/%d revealed", m.q.Title, m.hintsRevealed, len(m.q.Hints))
	footer := "enter/n reveal next · H hide all · ↑/↓ scroll · esc back"
	body := m.th.TitleFocused.Render(truncate(title, m.width)) + "\n" + m.hints.View() + "\n" + truncate(footer, m.width)
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(body)
}
