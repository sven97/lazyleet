package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// This file holds the shared bordered-pane renderer both BrowseModel.frame
// and WorkspaceModel.renderPane draw with: a lazygit-style top border that
// carries the pane's bracketed number and title embedded in the line itself
// ("╭─[1]─Status──────────╮") instead of a separate text row above the body.
// Lip Gloss v1 has no title-in-border primitive, so the border rows are built
// by hand here, reusing each Theme border's own glyphs/colors rather than
// hardcoding new rune literals — a future ASCII/light-terminal theme can swap
// the whole look by changing Theme alone.

// numberedFrame draws a bordered pane whose top border embeds a bracketed
// pane number and title: "╭─[1]─Status──────────╮". The body starts
// immediately below that row — there is no separate title line inside the
// border any more, unlike the pre-F2 layout.
//
// styledTitle is rendered exactly as given (truncated if it doesn't fit) —
// callers style it themselves (e.g. with an embedded, differently-colored
// suffix like workspace's " ● unrun" marker) rather than numberedFrame
// re-wrapping it in a single uniform style. Only the "[N]" bracket is styled
// here, using titleStyle.
func numberedFrame(border, titleStyle lipgloss.Style, number int, styledTitle string, r Rect, body string) string {
	if r.Empty() {
		return ""
	}
	bd := border.GetBorderStyle()
	top := borderTitleRow(bd.TopLeft, bd.Top, bd.TopRight, lineStyle(border, true), titleStyle, number, styledTitle, r.W)
	bottom := borderContentRow(bd.BottomLeft, bd.Bottom, bd.BottomRight, lineStyle(border, false), "", r.W)
	return joinFrame(border, top, bottom, r, body)
}

// joinFrame stitches a pre-built top row and bottom row around body, drawing
// plain left/right side borders for the rows in between (via lipgloss's own
// border rendering on the *bordered* style — unlike the top/bottom rows,
// which are hand-built strings, this one legitimately wants Style.Border()).
// Shared by numberedFrame (single "[N]-Title" top row) and WorkspaceModel's
// tabbed pane, which builds its own top row — a strip naming every pane, not
// just the focused one's number/title — via borderContentRow directly.
func joinFrame(border lipgloss.Style, top, bottom string, r Rect, body string) string {
	sideW := r.W - 2
	if sideW < 0 {
		sideW = 0
	}
	sideH := r.H - 2

	var out string
	if sideH <= 0 {
		// No room for any content row at all — just the border row(s).
		// lipgloss's MaxHeight(0) is treated as "unset", not "clip to
		// zero" (see clipLines), so this can't be built the same way as
		// the sideH>0 case below; skip straight to the final line-count
		// safety net instead.
		out = lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	} else {
		// MaxWidth/MaxHeight hard-clip: a body that renders wider/taller
		// than its pane must never push the neighbouring panes off-screen.
		sides := border.BorderTop(false).BorderBottom(false).
			Width(sideW).Height(sideH).MaxWidth(r.W).MaxHeight(sideH).
			Render(body)
		out = lipgloss.JoinVertical(lipgloss.Left, top, sides, bottom)
	}
	return clipLines(out, r.H)
}

// clipLines hard-truncates s to at most n lines. It's the final safety net
// for a pane too short even for both its border rows (r.H < 2): lipgloss's
// own MaxHeight(n) is a no-op when n is 0 (treated as "unset" rather than
// "clip to zero"), so joinFrame can't rely on it alone at that edge.
func clipLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

// lineStyle extracts a plain, unbordered style carrying just border's line
// color (and background, if any) for the top or bottom edge, for rendering
// individual border-row glyphs by hand. Render()ing text through border
// itself would be wrong here — since it has Border() enabled, every such
// call would wrap that text in its own miniature box instead of just
// coloring it; Style.Border() is only correct to use where lipgloss draws
// the border itself (see joinFrame's side rendering).
func lineStyle(border lipgloss.Style, top bool) lipgloss.Style {
	if top {
		return lipgloss.NewStyle().Foreground(border.GetBorderTopForeground()).Background(border.GetBorderTopBackground())
	}
	return lipgloss.NewStyle().Foreground(border.GetBorderBottomForeground()).Background(border.GetBorderBottomBackground())
}

// fillWidth repeats bar enough times to cover exactly cells terminal
// columns, hard-clipping (ANSI/grapheme-safe, via truncate) rather than
// relying on len() or assuming bar is a single byte.
func fillWidth(bar string, cells int) string {
	if cells <= 0 {
		return ""
	}
	bw := lipgloss.Width(bar)
	if bw < 1 {
		bw = 1
	}
	return truncate(strings.Repeat(bar, cells/bw+1), cells)
}

// borderContentRow wraps pre-styled content into a border row: corner,
// content, bar-fill to the remaining width, corner. line is a plain
// (unbordered) color style for the corners/fill glyphs — see lineStyle.
// content must already fit within width-corners or it is hard-truncated
// (ANSI-safe); shorter content is padded with the border's own bar glyph.
// Used for the bottom edge (empty content), a numbered top row (via
// borderTitleRow), and workspace's tabbed strip (every pane's number+name).
func borderContentRow(left, bar, right string, line lipgloss.Style, content string, width int) string {
	corners := lipgloss.Width(left) + lipgloss.Width(right)
	if width <= corners {
		return truncate(line.Render(fillWidth(bar, width)), width)
	}
	middle := width - corners
	switch w := lipgloss.Width(content); {
	case w > middle:
		content = truncate(content, middle)
	case w < middle:
		content += line.Render(fillWidth(bar, middle-w))
	}
	return line.Render(left) + content + line.Render(right)
}

// borderTitleRow renders a top border row with a bracketed pane number and
// (space permitting) its title embedded: "╭─[1]─Status──────────╮". Falls
// back to a plain filled row when even "[N]" doesn't fit, and drops just the
// title (keeping the number) when the title alone doesn't fit — the
// too-narrow-to-show-title case. line is the plain border-glyph color (see
// lineStyle); titleStyle colors the "[N]" bracket.
func borderTitleRow(left, bar, right string, line, titleStyle lipgloss.Style, number int, styledTitle string, width int) string {
	corners := lipgloss.Width(left) + lipgloss.Width(right)
	middle := width - corners

	label := "[" + strconv.Itoa(number) + "]"
	barW := lipgloss.Width(bar)
	prefixW := barW + lipgloss.Width(label) + barW

	var content string
	if middle > 0 && prefixW <= middle {
		content = line.Render(bar) + titleStyle.Render(label) + line.Render(bar)
		if rem := middle - prefixW; rem > 0 && styledTitle != "" {
			content += truncate(styledTitle, rem)
		}
	}
	return borderContentRow(left, bar, right, line, content, width)
}
