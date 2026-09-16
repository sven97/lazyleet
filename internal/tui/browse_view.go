package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *BrowseModel) View() (out string) {
	if debugScroll {
		defer func() { traceView("browse", out) }()
	}
	if !m.ready {
		return "loading browse…"
	}
	if m.showHelp {
		return m.renderHelp()
	}

	var body string
	if m.layout.Single {
		r := m.layout.RectFor(m.focus)
		body = m.frame(m.regionTitle(m.focus), true, r, m.regionBody(m.focus, innerW(r)))
	} else {
		left := lipgloss.JoinVertical(lipgloss.Left,
			m.frame("Status", m.focus == RegionStatus, m.layout.Status,
				m.statusPaneBody(innerW(m.layout.Status))),
			m.frame("Sources", m.focus == RegionSources, m.layout.Sources,
				m.sourcesBody(innerW(m.layout.Sources))),
			m.frame(m.listTitle(), m.focus == RegionList, m.layout.List,
				m.listBody(innerW(m.layout.List))),
		)
		right := m.frame(m.regionTitle(RegionDetail), m.focus == RegionDetail, m.layout.Detail, m.detailBody())
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderStatusBar())
}

func (m *BrowseModel) regionTitle(r Region) string {
	switch r {
	case RegionStatus:
		return "Status"
	case RegionSources:
		return "Sources"
	case RegionList:
		return m.listTitle()
	case RegionDetail:
		return m.detailTitle()
	}
	return r.String()
}

func (m *BrowseModel) regionBody(r Region, w int) string {
	switch r {
	case RegionStatus:
		return m.statusPaneBody(w)
	case RegionSources:
		return m.sourcesBody(w)
	case RegionDetail:
		return m.detailBody()
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
	// MaxWidth/MaxHeight hard-clip: a body that renders taller than its pane
	// must never push the neighbouring panes off-screen.
	return bs.Width(r.W - 2).Height(r.H - 2).
		MaxWidth(r.W).MaxHeight(r.H).Render(inner)
}

func (m *BrowseModel) sourcesBody(w int) string {
	var b strings.Builder
	b.WriteString(m.th.Muted.Render("PROBLEMS") + "\n")
	for i, s := range m.sources {
		if i == 2 {
			b.WriteString("\n" + m.th.Muted.Render("STUDY PLANS") + "\n")
		}
		line := s.label
		if i == m.activeSrc {
			line = "● " + line
		} else {
			line = "  " + line
		}
		line = truncate(line, w)
		if i == m.srcCursor && m.focus == RegionSources {
			line = lipgloss.NewStyle().Reverse(true).Render(padRight(line, w))
		}
		b.WriteString(line)
		if i < len(m.sources)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// staleSyncAfter is how old the full-catalog sync can get before the compact
// Status pane flags it instead of showing it in the same muted style as a
// fresh one — LeetCode adds problems continuously, so a silent multi-day-old
// "N problems" count is misleading. Matches the default cache_ttl.
const staleSyncAfter = 24 * time.Hour

// statusPaneBody is the compact 3-line summary shown in the small left-top pane
// (its rect is brStatusH tall: border + title + 3 body lines).
func (m *BrowseModel) statusPaneBody(w int) string {
	// The region is almost always the "com" default, so it's only worth the
	// space when it isn't — same rule statusDetailBody uses.
	var regionSuffix string
	if r := m.auth.Region; r != "" && r != "com" {
		regionSuffix = "  ·  " + r
	}

	var line1 string
	if m.auth.Authed {
		who := "signed in"
		if m.auth.User != "" {
			who = m.auth.User
		}
		line1 = m.th.Pass.Render("✓ ") + truncate(who+regionSuffix, w-2)
	} else {
		line1 = m.th.Muted.Render(truncate("· anonymous"+regionSuffix, w))
	}

	var line2 string
	if len(m.allRows) == 0 {
		line2 = m.th.ErrorText.Render("cache empty — press s to sync")
	} else {
		// Signed-in users get their own solved count leading the line — it's
		// the one number this whole app is about, and otherwise never shows
		// up outside a specific study plan. Anonymous users have no solved
		// status to show, so fall back to the catalog size.
		var txt string
		if m.auth.Authed {
			txt = fmt.Sprintf("%d/%d solved", m.solvedCount(), len(m.allRows))
		} else {
			txt = fmt.Sprintf("%d problems", len(m.allRows))
		}
		style := m.th.Muted
		if !m.lastSync.IsZero() {
			age := time.Since(m.lastSync)
			txt += " · synced " + roughAge(age) + " ago"
			if age > staleSyncAfter {
				txt += " · press s"
				style = m.th.ErrorText
			}
		}
		line2 = style.Render(truncate(txt, w))
	}
	return line1 + "\n" + line2 + "\n" + m.dailyStatusLine(w)
}

// dailyStatusLine is the one-line "daily · done/not-done · streak N" summary.
func (m *BrowseModel) dailyStatusLine(w int) string {
	if m.dailyErr != nil {
		return m.th.Muted.Render(truncate("daily · unavailable", w))
	}
	if !m.dailyLoaded || m.daily.Slug == "" {
		return m.th.Muted.Render("daily · …")
	}
	mark := m.th.Muted.Render("○")
	state := "not done"
	if m.daily.Done {
		mark, state = m.th.Pass.Render("✓"), "done"
	}
	txt := "daily · " + state
	if m.daily.Streak > 0 {
		txt += " · streak " + plural(m.daily.Streak, "day")
	}
	return mark + " " + m.th.Muted.Render(truncate(txt, w-2))
}

// statusDetailBody is the expanded info shown in the Detail pane while the
// Status pane is focused: who you are, how fresh the two caches are (the full
// problem catalog and your own solve status), and today's daily challenge.
func (m *BrowseModel) statusDetailBody() string {
	th := m.th
	var b strings.Builder
	row := func(k, v string) { b.WriteString("  " + fmt.Sprintf("%-11s", k) + v + "\n") }
	section := func(name string) { b.WriteString("\n" + th.Title.Render(name) + "\n") }

	b.WriteString(th.Title.Render("Account") + "\n")
	switch {
	case !m.auth.Authed:
		row("user", th.Muted.Render("anonymous — run `lazyleet auth`"))
	case m.auth.User != "":
		row("user", m.auth.User)
	default:
		row("user", th.Muted.Render("signed in"))
	}
	if r := m.auth.Region; r != "" && r != "com" {
		row("region", r)
	}

	section("Catalog")
	row("problems", strconv.Itoa(len(m.allRows)))
	row("synced", m.syncAge(m.lastSync, "never — press s"))

	section("Progress")
	if !m.auth.Authed {
		row("solved", th.Muted.Render("sign in to track"))
	} else {
		row("solved", fmt.Sprintf("%d / %d", m.solvedCount(), len(m.allRows)))
		pending := "not yet — syncs on open"
		if m.progressing {
			pending = "syncing…"
		}
		row("synced", m.syncAge(m.progressSync, pending))
	}

	section("Daily")
	switch {
	case m.dailyErr != nil:
		b.WriteString("  " + th.Muted.Render("unavailable — "+m.dailyErr.Error()) + "\n")
	case !m.dailyLoaded || m.daily.Slug == "":
		b.WriteString("  " + th.Muted.Render("loading…") + "\n")
	default:
		name := m.daily.Title
		if m.daily.FrontendID > 0 {
			name = fmt.Sprintf("%d. %s", m.daily.FrontendID, m.daily.Title)
		}
		row("problem", name)
		if m.daily.Difficulty != "" {
			row("difficulty", DifficultyStyle(m.daily.Difficulty).Render(m.daily.Difficulty))
		}
		status := th.Muted.Render("○ not done")
		if m.daily.Done {
			status = th.Pass.Render("✓ done")
		}
		if m.auth.Authed && m.daily.Streak > 0 {
			status += th.Muted.Render("  ·  streak " + plural(m.daily.Streak, "day"))
		}
		row("status", status)
		if today := time.Now().Format("2006-01-02"); m.daily.Date != "" && m.daily.Date != today {
			b.WriteString("  " + th.Muted.Render("cached "+m.daily.Date) + "\n")
		}
	}

	if dir := dataDir(m.auth); dir != "" {
		b.WriteString("\n" + th.Muted.Render(dir) + "\n")
	}
	return b.String()
}

// syncAge renders "<rough age> ago", or the muted fallback when t is unset.
func (m *BrowseModel) syncAge(t time.Time, fallback string) string {
	if t.IsZero() {
		return m.th.Muted.Render(fallback)
	}
	return roughAge(time.Since(t)) + " ago"
}

func (m *BrowseModel) solvedCount() int {
	n := 0
	for _, r := range m.allRows {
		if r.Solved() {
			n++
		}
	}
	return n
}

// plural renders "1 day" / "3 days" (the plural is the noun + "s").
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// dataDir is the directory holding auth.json and the cache DB, with $HOME
// collapsed to "~". Empty when neither path is known.
func dataDir(a AuthState) string {
	p := a.AuthFile
	if p == "" {
		p = a.CacheDB
	}
	if p == "" {
		return ""
	}
	dir := filepath.Dir(p)
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(dir, home) {
		dir = "~" + dir[len(home):]
	}
	return dir
}

func (m *BrowseModel) listTitle() string {
	var base string
	switch m.activeSourceKind() {
	case srcPlan:
		solved, total := m.planProgress()
		base = fmt.Sprintf("%s  %d/%d solved", m.sources[m.activeSrc].label, solved, total)
	case srcDaily:
		base = "Daily Question"
		if m.daily.Date != "" {
			base += "  " + m.daily.Date
		}
		if m.daily.Done {
			base += "  " + m.th.Pass.Render("✓ done")
		}
	default:
		base = fmt.Sprintf("Problems  (%d)", len(m.filtered))
	}
	if s := m.filterSummary(); s != "" {
		base += "  " + m.th.Muted.Render(s)
	}
	return base
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
		if r := m.emptyListReason(); r != "" {
			return m.th.Muted.Render(r)
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

func (m *BrowseModel) detailTitle() string {
	if m.detailShowsStatus {
		return "Status detail"
	}
	if r, ok := m.currentRow(); ok {
		return problemTitle(r.FrontendID, r.Title, r.PaidOnly)
	}
	return "Detail"
}

func (m *BrowseModel) detailBody() string {
	if m.detailShowsStatus {
		return m.previewVP.View() // holds statusDetailBody, set by refreshDetail
	}
	// The viewport still holds the previously shown statement until the new
	// one is fetched and rendered; don't show stale content for the wrong row.
	stale := m.previewContentSlug != m.currentSlug()
	switch {
	case m.previewErr != nil && !stale:
		return m.th.ErrorText.Render("could not load: ") + m.previewErr.Error()
	case m.previewLoading || stale:
		return m.th.Spinner.Render(m.spin.View()) + " loading statement…"
	default:
		return m.previewVP.View()
	}
}

func (m *BrowseModel) renderStatusBar() string {
	if m.layout.Footer.Empty() {
		return ""
	}
	w := m.width
	left := ""
	if m.syncing {
		left = m.th.Spinner.Render(m.spin.View()) + " syncing "
	} else if m.progressing {
		left = m.th.Spinner.Render(m.spin.View()) + " syncing progress "
	} else if m.statusMsg != "" {
		left = m.statusMsg + "  "
	}

	var segs []string
	for _, h := range m.keys.shortcutHints(m.filtering) {
		segs = append(segs, m.th.StatusKey.Render(h.key)+m.th.StatusBar.Render(" "+h.desc))
	}
	// The Status pane already shows this, with a staleness nudge — only repeat
	// it here when that pane isn't actually on screen (single-pane layout,
	// i.e. a narrow terminal or another pane zoomed).
	right := ""
	if !m.lastSync.IsZero() && m.layout.Single && m.focus != RegionStatus {
		age := time.Since(m.lastSync)
		style := m.th.Muted
		if age > staleSyncAfter {
			style = m.th.ErrorText
		}
		right = style.Render(fmt.Sprintf("  synced %s ago", roughAge(age)))
	}

	line := left + strings.Join(segs, m.th.StatusDivider.Render(" │ ")) + right
	return m.th.StatusBar.Width(w).Render(truncate(line, w))
}

func (m *BrowseModel) renderHelp() string {
	pairs := [][2]string{
		{"↑/k ↓/j", "move"}, {"g / G", "top / bottom"}, {"ctrl+u / ctrl+d", "page"},
		{"tab / ⇧tab", "cycle panes"}, {"h / l", "prev / next pane"},
		{"enter", "open workspace (or apply a source)"},
		{"/", "fuzzy filter"}, {"esc", "clear fuzzy filter / close help"},
		{"d", "cycle difficulty filter (Easy → Medium → Hard → all)"},
		{"f", "cycle status filter (unsolved → solved → attempted → all)"},
		{"p", "toggle hide paid-only"},
		{"S", "cycle sort (# → AC%↑ → AC%↓ → difficulty)"},
		{"c", "clear all filters + sort"},
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
