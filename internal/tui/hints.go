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
	m.hintsSel.clear() // pane geometry is about to change under it
	m.refreshHints()
}

// hintsBodyRect is the Hints overlay's viewport body in screen coordinates.
// Unlike the three numbered panes (see innerSize/localCell in frame.go /
// selection.go), the overlay draws no border: hintsView stacks a one-line
// title, the viewport, then a one-line footer directly, so the body starts
// at row 1 with no side-column offset either.
func (m *WorkspaceModel) hintsBodyRect() Rect {
	return Rect{X: 0, Y: 1, W: m.hints.Width, H: m.hints.Height}
}

// handleHintsMouse handles mouse input while the Hints overlay is shown: F1
// click-drag selection over its body (see selection.go), falling back to the
// viewport's own wheel-scroll handling otherwise.
func (m *WorkspaceModel) handleHintsMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	r := m.hintsBodyRect()
	// A drag already in progress: route its motion/release here regardless of
	// where the cursor is now. A stray press with no matching release (in
	// practice, real mouse protocols always pair the two) falls through to
	// the click-begin logic below instead of being swallowed; begin() resets
	// hintsSel.active cleanly either way.
	if m.hintsSel.active && (msg.Action == tea.MouseActionMotion || msg.Action == tea.MouseActionRelease) {
		lx, ly := rawClampCell(msg.X, msg.Y, r)
		if msg.Action == tea.MouseActionRelease {
			m.hintsSel.end(lx, ly)
			m.copyHintsSelection()
		} else {
			m.hintsSel.extend(lx, ly)
		}
		return m, nil
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if lx, ly, ok := rawLocalCell(msg.X, msg.Y, r); ok {
			m.hintsSel.begin(lx, ly)
			return m, nil
		}
	}
	if tea.MouseEvent(msg).IsWheel() {
		m.hintsSel.clear()
	}
	var cmd tea.Cmd
	m.hints, cmd = m.hints.Update(msg)
	return m, cmd
}

// copyHintsSelection finalizes a finished Hints-pane selection: copies its
// plain text to the system clipboard via OSC 52 and leaves a one-line status
// confirmation (visible once the user leaves the Hints overlay — it has no
// status bar of its own). A no-op for a plain click (no drag).
func (m *WorkspaceModel) copyHintsSelection() {
	if !m.hintsSel.hasSelection {
		return
	}
	m.copyText(selectedText(strings.Split(m.hints.View(), "\n"), m.hintsSel))
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
	m.hintsSel.clear()
}

func (m *WorkspaceModel) handleHintsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "b":
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
	case "y":
		// F1 keyboard fallback: copy the whole pane's visible content when
		// there's no active drag selection to copy instead.
		if m.hintsSel.hasSelection {
			m.copyHintsSelection()
		} else {
			m.copyText(plainViewportText(m.hints.View()))
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.hints, cmd = m.hints.Update(msg)
	return m, cmd
}

func (m *WorkspaceModel) hintsView() string {
	title := fmt.Sprintf("Hints · %s · %d/%d revealed", m.q.Title, m.hintsRevealed, len(m.q.Hints))
	footer := "enter/n reveal next · H hide all · ↑/↓ scroll · y copy · esc back"
	view := applySelectionHighlight(m.hints.View(), m.hintsSel, m.th.Selection)
	body := m.th.TitleFocused.Render(truncate(title, m.width)) + "\n" + view + "\n" + truncate(footer, m.width)
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(body)
}
