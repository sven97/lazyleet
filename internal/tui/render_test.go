package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTruncateRespectsDisplayWidth(t *testing.T) {
	cases := []struct {
		s   string
		max int
	}{
		{"plain ascii title that is quite long indeed", 20},
		{"🔒 Number of Transactions per Visit", 30}, // emoji is 2 cells wide
		{"🔒 Students With Invalid Department", 18},
		{strings.Repeat("我", 40), 15}, // CJK, 2 cells each
		{"short", 40},                 // already fits — unchanged
	}
	for _, c := range cases {
		got := truncate(c.s, c.max)
		if w := lipgloss.Width(got); w > c.max {
			t.Errorf("truncate(%q, %d) width = %d, want <= %d (got %q)", c.s, c.max, w, c.max, got)
		}
	}
	if got := truncate("short", 40); got != "short" {
		t.Errorf("a string that fits should pass through unchanged, got %q", got)
	}
}

func TestFormatRowNeverExceedsWidth(t *testing.T) {
	m := &BrowseModel{th: DefaultTheme()}
	rows := []BrowseRow{
		{FrontendID: 1336, Title: "Number of Transactions per Visit", Difficulty: "Hard", ACRate: 47.9, PaidOnly: true},
		{FrontendID: 1350, Title: "Students With Invalid Department", Difficulty: "Easy", ACRate: 89.7, PaidOnly: true},
		{FrontendID: 1, Title: "Two Sum", Difficulty: "Easy", ACRate: 52, Status: "ac"},
		{FrontendID: 4047, Title: strings.Repeat("Long Title ", 12), Difficulty: "Medium", ACRate: 12.3},
	}
	for _, w := range []int{24, 40, 60, 80, 120} {
		for _, r := range rows {
			line := m.formatRow(r, w)
			if got := lipgloss.Width(line); got > w {
				t.Errorf("formatRow(%q, w=%d) width = %d, must not exceed w", r.Title, w, got)
			}
			if strings.Contains(line, "\n") {
				t.Errorf("formatRow(%q) contains a newline — it would wrap the list", r.Title)
			}
		}
	}
}
