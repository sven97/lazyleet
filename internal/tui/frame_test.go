package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// withColorProfile forces lipgloss's default renderer to emit real ANSI
// codes for the duration of the test, restoring the previous profile after.
// Under `go test` (no tty) lipgloss otherwise renders every Style.Render as
// plain text — see the existing note on TestRenderRunCasesShowsGotInMutedNotFailColorOnAPass
// in render_test.go — so this is needed for the one test here that actually
// asserts on color, rather than just on rendered structure/width.
func withColorProfile(t *testing.T, p termenv.Profile) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(p)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func TestNumberedFrameEmbedsNumberAndTitleInTopBorder(t *testing.T) {
	th := DefaultTheme()
	r := Rect{W: 24, H: 6}
	got := numberedFrame(th.PaneBorder, th.Title, 1, th.Title.Render("Status"), r, "line1\nline2")
	lines := strings.Split(got, "\n")
	if len(lines) != r.H {
		t.Fatalf("rendered %d lines, want exactly %d (r.H): %q", len(lines), r.H, got)
	}
	top := lines[0]
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Errorf("top row should start/end with the theme's rounded corners, got %q", top)
	}
	if !strings.Contains(top, "[1]") {
		t.Errorf("top row should embed the bracketed pane number, got %q", top)
	}
	if !strings.Contains(top, "Status") {
		t.Errorf("top row should embed the title, got %q", top)
	}
	// The body starts immediately below the top border — no separate title
	// line eating a row (the pre-F2 layout).
	if !strings.Contains(lines[1], "line1") {
		t.Errorf("body should start on row 1 (right below the top border), got %q", lines[1])
	}
	bottom := lines[len(lines)-1]
	if !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
		t.Errorf("bottom row should be a plain border fill with rounded corners, got %q", bottom)
	}
}

// TestFillWidthNeverInsertsEllipsis guards against a regression where
// fillWidth routed its over-generated repeat string through the ellipsis-tail
// truncate() helper: since fillWidth always over-generates on purpose (so it
// can clip down to an exact width), that path fired on every call and left a
// stray "…" in place of the border glyph on every pane's top/bottom border.
func TestFillWidthNeverInsertsEllipsis(t *testing.T) {
	for _, cells := range []int{0, 1, 2, 3, 7, 20, 21, 63} {
		got := fillWidth("─", cells)
		if strings.Contains(got, "…") {
			t.Errorf("fillWidth(%q, %d) = %q contains a stray ellipsis", "─", cells, got)
		}
		if w := lipgloss.Width(got); w != cells {
			t.Errorf("fillWidth(%q, %d) width = %d, want %d", "─", cells, w, cells)
		}
	}
}

func TestNumberedFrameBordersHaveNoStrayEllipsis(t *testing.T) {
	th := DefaultTheme()
	r := Rect{W: 24, H: 6}
	got := numberedFrame(th.PaneBorder, th.Title, 1, th.Title.Render("Status"), r, "line1\nline2")
	lines := strings.Split(got, "\n")
	top, bottom := lines[0], lines[len(lines)-1]
	if strings.Contains(top, "…") {
		t.Errorf("top border contains a stray ellipsis: %q", top)
	}
	if strings.Contains(bottom, "…") {
		t.Errorf("bottom border contains a stray ellipsis: %q", bottom)
	}
}

func TestNumberedFrameNeverExceedsItsRect(t *testing.T) {
	th := DefaultTheme()
	longTitle := strings.Repeat("Very Long Problem Title ", 5)
	longBody := strings.Repeat("some body content that is quite long\n", 20)
	for _, w := range []int{0, 1, 2, 3, 4, 5, 6, 8, 10, 13, 20, 30, 80} {
		for _, h := range []int{0, 1, 2, 3, 5, 10} {
			r := Rect{W: w, H: h}
			got := numberedFrame(th.PaneBorderFocused, th.TitleFocused, 4, th.TitleFocused.Render(longTitle), r, longBody)
			if r.Empty() {
				if got != "" {
					t.Errorf("w=%d h=%d: an empty rect should render empty, got %q", w, h, got)
				}
				continue
			}
			lines := strings.Split(got, "\n")
			if len(lines) != h {
				t.Errorf("w=%d h=%d: rendered %d lines, want exactly %d", w, h, len(lines), h)
			}
			for i, ln := range lines {
				if lw := lipgloss.Width(ln); lw > w {
					t.Errorf("w=%d h=%d: line %d width %d exceeds w: %q", w, h, i, lw, ln)
				}
			}
		}
	}
}

func TestNumberedFrameTooNarrowDropsTitleButKeepsNumber(t *testing.T) {
	th := DefaultTheme()
	// corners(2) + "─[1]─" (5) leaves only 2 cells of middle for the title —
	// enough for the number, not enough for "Status" (6 cells).
	r := Rect{W: 9, H: 3}
	got := numberedFrame(th.PaneBorder, th.Title, 1, th.Title.Render("Status"), r, "body")
	top := strings.Split(got, "\n")[0]
	if !strings.Contains(top, "[1]") {
		t.Errorf("the number should still fit and render, got %q", top)
	}
	if strings.Contains(top, "Status") {
		t.Errorf("the full title shouldn't fit at this width, got %q", top)
	}
	if lw := lipgloss.Width(top); lw != r.W {
		t.Errorf("top row width = %d, want exactly %d: %q", lw, r.W, top)
	}
}

func TestNumberedFrameTooNarrowDropsNumberToo(t *testing.T) {
	th := DefaultTheme()
	// corners(2) + "─[1]─" (5) = 7 doesn't fit in a width-6 pane (middle=4).
	r := Rect{W: 6, H: 3}
	got := numberedFrame(th.PaneBorder, th.Title, 1, th.Title.Render("Status"), r, "body")
	top := strings.Split(got, "\n")[0]
	if strings.Contains(top, "[1]") {
		t.Errorf("not even the number should fit at this width, got %q", top)
	}
	if lw := lipgloss.Width(top); lw != r.W {
		t.Errorf("top row width = %d, want exactly %d: %q", lw, r.W, top)
	}
}

func TestNumberedFrameFocusedVsUnfocusedColoring(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	th := DefaultTheme()
	r := Rect{W: 24, H: 4}

	focused := numberedFrame(th.PaneBorderFocused, th.TitleFocused, 1, th.TitleFocused.Render("Status"), r, "body")
	unfocused := numberedFrame(th.PaneBorder, th.Title, 1, th.Title.Render("Status"), r, "body")

	if focused == unfocused {
		t.Fatal("focused and unfocused frames should render different ANSI styling")
	}
	// colFocus = 39 (bright blue), colMuted = 245 (grey) in theme.go.
	if !strings.Contains(focused, "39m") {
		t.Errorf("focused frame should carry the focus color (256-color code 39), got %q", focused)
	}
	if !strings.Contains(unfocused, "245m") {
		t.Errorf("unfocused frame should carry the muted color (256-color code 245), got %q", unfocused)
	}
}

func TestRegionNumberRoundTrips(t *testing.T) {
	for _, r := range []Region{RegionStatus, RegionSources, RegionList, RegionDetail} {
		got, ok := regionForNumber(r.Number())
		if !ok || got != r {
			t.Errorf("regionForNumber(%d.Number()=%d) = (%v, %v), want (%v, true)", r, r.Number(), got, ok, r)
		}
	}
	if RegionStatus.Number() != 1 || RegionSources.Number() != 2 || RegionList.Number() != 3 || RegionDetail.Number() != 4 {
		t.Error("browse region numbering should follow iota order 1-4")
	}
	for _, n := range []int{0, -1, 5, 100} {
		if _, ok := regionForNumber(n); ok {
			t.Errorf("regionForNumber(%d) should be invalid", n)
		}
	}
}

func TestPaneNumberRoundTrips(t *testing.T) {
	for _, p := range []Pane{PaneStatement, PaneCode, PaneResults} {
		got, ok := paneForNumber(p.Number())
		if !ok || got != p {
			t.Errorf("paneForNumber(%d.Number()=%d) = (%v, %v), want (%v, true)", p, p.Number(), got, ok, p)
		}
	}
	if PaneStatement.Number() != 1 || PaneCode.Number() != 2 || PaneResults.Number() != 3 {
		t.Error("workspace pane numbering should follow iota order 1-3")
	}
	for _, n := range []int{0, -1, 4, 100} {
		if _, ok := paneForNumber(n); ok {
			t.Errorf("paneForNumber(%d) should be invalid", n)
		}
	}
}

func TestDigitKeyParsesBareDigitsOnly(t *testing.T) {
	if n, ok := digitKey(keyRune('1')); !ok || n != 1 {
		t.Errorf("digitKey('1') = (%d, %v), want (1, true)", n, ok)
	}
	if n, ok := digitKey(keyRune('9')); !ok || n != 9 {
		t.Errorf("digitKey('9') = (%d, %v), want (9, true)", n, ok)
	}
	if _, ok := digitKey(keyRune('0')); ok {
		t.Error("digitKey('0') should be invalid — panes are 1-based")
	}
	if _, ok := digitKey(keyRune('a')); ok {
		t.Error("digitKey('a') should be invalid — letters aren't digits")
	}
}
