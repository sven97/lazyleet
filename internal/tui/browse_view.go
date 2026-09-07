package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *BrowseModel) View() string {
	if !m.ready {
		return "loading browse…"
	}
	if m.showHelp {
		return m.renderHelp()
	}

	var body string
	if m.zoom {
		body = m.frame(m.focus.String(), true, fullRect(m.width, m.layout.Status), m.regionBody(m.focus, m.width-2, m.zoomInnerH()))
	} else {
		var cols []string
		if m.layout.ShowSidebar {
			cols = append(cols, m.frame("Sources", m.focus == RegionSidebar, m.layout.Sidebar,
				m.sidebarBody(innerW(m.layout.Sidebar))))
		}
		cols = append(cols, m.frame(m.listTitle(), m.focus == RegionList, m.layout.List,
			m.listBody(innerW(m.layout.List))))
		if m.layout.ShowPreview {
			cols = append(cols, m.frame(m.previewTitle(), m.focus == RegionPreview, m.layout.Preview,
				m.previewBody()))
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderStatusBar())
}

func (m *BrowseModel) zoomInnerH() int {
	h := m.height - statusBarHeight - 2 - 1
	if h < 1 {
		h = 1
	}
	return h
}

func (m *BrowseModel) regionBody(r Region, w, _ int) string {
	switch r {
	case RegionSidebar:
		return m.sidebarBody(w)
	case RegionPreview:
		return m.previewBody()
	default:
		return m.listBody(w)
	}
}

// frame draws a bordered pane with a title row.
func (m *BrowseModel) frame(title string, focused bool, r Rect, body string) string {
	if r.Empty() {
		return ""
	}
	ts := m.th.Title
	bs := m.th.PaneBorder
	if focused {
		ts, bs = m.th.TitleFocused, m.th.PaneBorderFocused
	}
	inner := lipgloss.JoinVertical(lipgloss.Left, ts.Render(title), body)
	return bs.Width(r.W - 2).Height(r.H - 2).Render(inner)
}

func (m *BrowseModel) sidebarBody(w int) string {
	var b strings.Builder
	b.WriteString(m.th.Muted.Render("PROBLEMS") + "\n")
	for i, s := range m.sources {
		if i == 1 {
			b.WriteString("\n" + m.th.Muted.Render("STUDY PLANS") + "\n")
		}
		line := s.label
		if i == m.activeSrc {
			line = "● " + line
		} else {
			line = "  " + line
		}
		line = truncate(line, w)
		if i == m.srcCursor && m.focus == RegionSidebar {
			line = lipgloss.NewStyle().Reverse(true).Render(padRight(line, w))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m *BrowseModel) listTitle() string {
	if m.activeSrc > 0 && m.activeSrc < len(m.sources) && m.sources[m.activeSrc].kind == srcPlan {
		solved, total := m.planProgress()
		return fmt.Sprintf("%s  %d/%d solved", m.sources[m.activeSrc].label, solved, total)
	}
	return fmt.Sprintf("Problems  (%d)", len(m.filtered))
}

func (m *BrowseModel) planProgress() (solved, total int) {
	total = len(m.view)
	for _, r := range m.view {
		if r.Solved() {
			solved++
		}
	}
	return
}

func (m *BrowseModel) listBody(w int) string {
	if m.filtering {
		return m.filter.View() + "\n" + m.listRowsBody(w, m.listRows()-1)
	}
	return m.listRowsBody(w, m.listRows())
}

func (m *BrowseModel) listRowsBody(w, rows int) string {
	if len(m.view) == 0 {
		if m.loadErr != nil {
			return m.th.ErrorText.Render("failed to load problems: ") + m.loadErr.Error()
		}
		return m.th.Muted.Render("no problems cached — press s to sync from LeetCode")
	}
	if len(m.filtered) == 0 {
		return m.th.Muted.Render("no matches for ") + m.filter.Value()
	}

	var b strings.Builder
	b.WriteString(m.th.Muted.Render(fmt.Sprintf("%-2s %5s %-3s %6s  %s", "", "#", "Dif", "AC%", "Title")) + "\n")

	end := m.top + rows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	for i := m.top; i < end; i++ {
		row := m.view[m.filtered[i]]
		line := m.formatRow(row, w)
		if i == m.cursor {
			style := lipgloss.NewStyle().Bold(true)
			if m.focus == RegionList {
				style = style.Reverse(true)
			}
			line = style.Render(padRight(clip(line, w), w))
		}
		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *BrowseModel) formatRow(r BrowseRow, w int) string {
	glyph := m.th.Muted.Render("·")
	switch r.Status {
	case "ac":
		glyph = m.th.Pass.Render("✓")
	case "notac":
		glyph = m.th.ErrorText.Render("~")
	}
	dif := "?"
	if len(r.Difficulty) > 0 {
		dif = string([]rune(r.Difficulty)[:1])
	}
	difCell := DifficultyStyle(r.Difficulty).Render(fmt.Sprintf("%-3s", dif))
	id := fmt.Sprintf("%5d", r.FrontendID)
	ac := fmt.Sprintf("%5.1f%%", r.ACRate)
	title := r.Title
	if r.PaidOnly {
		title = "🔒 " + title
	}
	prefix := fmt.Sprintf("%s %s %s %s  ", glyph, m.th.Muted.Render(id), difCell, m.th.Muted.Render(ac))
	avail := w - lipgloss.Width(prefix)
	if avail < 4 {
		avail = 4
	}
	return prefix + truncate(title, avail)
}

func (m *BrowseModel) previewTitle() string {
	if m.previewTab == 1 {
		return "Topics"
	}
	if r, ok := m.currentRow(); ok {
		return fmt.Sprintf("%d. %s", r.FrontendID, r.Title)
	}
	return "Preview"
}

func (m *BrowseModel) previewBody() string {
	switch {
	case m.previewLoading:
		return m.th.Spinner.Render(m.spin.View()) + " loading statement…"
	case m.previewErr != nil:
		return m.th.ErrorText.Render("could not load: ") + m.previewErr.Error()
	default:
		return m.previewVP.View()
	}
}

func (m *BrowseModel) renderStatusBar() string {
	if m.layout.Status.Empty() {
		return ""
	}
	w := m.width
	left := ""
	if m.syncing {
		left = m.th.Spinner.Render(m.spin.View()) + " syncing "
	} else if m.statusMsg != "" {
		left = m.statusMsg + "  "
	}

	var segs []string
	for _, h := range m.keys.shortcutHints(m.filtering) {
		segs = append(segs, m.th.StatusKey.Render(h.key)+m.th.StatusBar.Render(" "+h.desc))
	}
	right := ""
	if !m.lastSync.IsZero() {
		right = m.th.Muted.Render(fmt.Sprintf("  synced %s ago", roughAge(time.Since(m.lastSync))))
	}

	line := left + strings.Join(segs, m.th.StatusDivider.Render(" │ ")) + right
	return m.th.StatusBar.Width(w).Render(truncate(line, w))
}

func (m *BrowseModel) renderHelp() string {
	pairs := [][2]string{
		{"↑/k ↓/j", "move"}, {"g / G", "top / bottom"}, {"ctrl+u / ctrl+d", "page"},
		{"tab / ⇧tab", "cycle panes"}, {"h / l", "prev / next pane"},
		{"enter", "open workspace (or apply a source)"},
		{"/", "fuzzy filter"}, {"esc", "clear filter / close help"},
		{"]", "toggle preview tab (statement / topics)"},
		{"s", "sync problem cache from LeetCode"},
		{"z", "zoom the focused pane"}, {"?", "toggle this help"}, {"q", "quit"},
	}
	var b strings.Builder
	b.WriteString(m.th.TitleFocused.Render("lazyleet — browse mode") + "\n\n")
	for _, p := range pairs {
		b.WriteString(fmt.Sprintf("  %s  %s\n", m.th.StatusKey.Render(fmt.Sprintf("%-16s", p[0])), p[1]))
	}
	b.WriteString("\n" + m.th.Muted.Render("press ? or esc to return"))
	return b.String()
}

// --- small helpers ---

func innerW(r Rect) int {
	w := r.W - 2
	if w < 1 {
		return 1
	}
	return w
}

func fullRect(w int, status Rect) Rect {
	h := status.Y
	if status.Empty() {
		h = 0
	}
	if h <= 0 {
		h = 1
	}
	return Rect{X: 0, Y: 0, W: w, H: h}
}

func padRight(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

func clip(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return truncate(s, w)
}

func roughAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
