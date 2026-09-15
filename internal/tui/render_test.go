package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/sven97/lazyleet/internal/runner"
	"github.com/sven97/lazyleet/internal/testcase"
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

func TestRenderRunCasesClampsATrailingPaddingEntry(t *testing.T) {
	th := DefaultTheme()
	// 1 of 2 samples wrong, plus the trailing padding entry LeetCode's
	// interpret responses have been seen to carry beyond TotalTestcases.
	out := &RemoteOutcome{
		Total:    2,
		Expected: []string{"2", "0", ""},
		Actual:   []string{"1", "0", ""},
	}
	got := renderRunCases(th, out, nil, 40)

	if strings.Contains(got, "|") {
		t.Errorf("padding entry leaked into the render:\n%s", got)
	}
	if !strings.Contains(got, "case 1") || !strings.Contains(got, "case 2") {
		t.Errorf("should show both real cases:\n%s", got)
	}
	if strings.Contains(got, "case 3") {
		t.Errorf("the padding entry shouldn't render as a third case:\n%s", got)
	}
}

func TestRenderRemoteSubmitShowsExpGotWhenMapOutcomeFoundThem(t *testing.T) {
	th := DefaultTheme()
	// Simulates mapOutcome's fallback to CodeOutput/ExpectedOutput for the
	// one failing case a submission check surfaces (see remotejudge.go).
	out := &RemoteOutcome{
		Kind: "submit", Verdict: "Wrong Answer", Accepted: false,
		Passed: 5, Total: 12, LastCase: "1002",
		Expected: []string{"4"}, Actual: []string{"9"},
	}
	got := renderRemote(th, "submit", out, nil, false, "", 60, nil)
	for _, want := range []string{"1002", "4", "9"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "case 1") {
		t.Errorf("a single known-failing case doesn't need a case label:\n%s", got)
	}
}

func TestRenderResultsShowsExpectedAndActualOnAPass(t *testing.T) {
	th := DefaultTheme()
	res := &runner.Result{
		Passed: 1, Total: 1,
		Cases: []runner.CaseResult{
			{Index: 0, Status: runner.StatusPass, Input: []string{"[2,7,11,15]", "9"}, Expected: "[0,1]", Actual: "[0,1]"},
		},
	}
	got := renderResults(th, res, nil, false, "", 60)
	if !strings.Contains(got, "[2,7,11,15]") {
		t.Errorf("missing input:\n%s", got)
	}
	if strings.Count(got, "[0,1]") != 2 {
		t.Errorf("a pass should show both expected and actual (equal, but both), got:\n%s", got)
	}
}

func TestRenderRunCasesShowsInputExpectedAndActualForEveryCase(t *testing.T) {
	th := DefaultTheme()
	out := &RemoteOutcome{
		Total:    2,
		Expected: []string{"2", "0"},
		Actual:   []string{"2", "0"},
	}
	sent := []testcase.Case{
		{In: []string{"[2,7,11,15]", "9"}},
		{In: []string{"[3,2,4]", "6"}},
	}
	got := renderRunCases(th, out, sent, 60)
	for _, want := range []string{"case 1", "[2,7,11,15], 9", "case 2", "[3,2,4], 6"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "exp") != 2 || strings.Count(got, "got") != 2 {
		t.Errorf("expected exp/got for both (passing) cases, got:\n%s", got)
	}
}

func TestRenderRunCasesShowsGotInMutedNotFailColorOnAPass(t *testing.T) {
	// Regression: "got" was always DiffDel (fail/red)-styled, even for a
	// passing case — misleading next to its own ✓ mark. Can't assert on
	// color directly (styles render as plain text under go test / no tty),
	// so this at minimum locks in that a passing case's row doesn't carry
	// the ✗ mark or any fail-only text.
	th := DefaultTheme()
	out := &RemoteOutcome{Total: 1, Expected: []string{"2"}, Actual: []string{"2"}}
	got := renderRunCases(th, out, nil, 60)
	if strings.Contains(got, "✗") {
		t.Errorf("a passing case shouldn't render a fail mark:\n%s", got)
	}
	if !strings.Contains(got, "✓") {
		t.Errorf("a passing case should render a pass mark:\n%s", got)
	}
}

func TestRenderRemoteRunShowsCasesEvenWhenAccepted(t *testing.T) {
	th := DefaultTheme()
	out := &RemoteOutcome{
		Kind: "run", Verdict: "Sample tests passed", Accepted: true, Total: 1,
		Expected: []string{"[0,1]"}, Actual: []string{"[0,1]"},
	}
	sent := []testcase.Case{{In: []string{"[2,7,11,15]", "9"}}}
	got := renderRemote(th, "run", out, nil, false, "", 60, sent)
	if !strings.Contains(got, "[0,1]") || !strings.Contains(got, "[2,7,11,15], 9") {
		t.Errorf("an accepted Run Code result should still show its case detail, got:\n%s", got)
	}
}

func TestRenderRemoteSubmitFallsBackToRawInputOnly(t *testing.T) {
	th := DefaultTheme()
	// Real submission-check responses typically carry no per-case answer data.
	out := &RemoteOutcome{
		Kind: "submit", Verdict: "Wrong Answer", Accepted: false,
		Passed: 5, Total: 12, LastCase: "1002",
	}
	got := renderRemote(th, "submit", out, nil, false, "", 60, nil)
	if !strings.Contains(got, "failed on input") || !strings.Contains(got, "1002") {
		t.Errorf("submit should still show the one failing input, got:\n%s", got)
	}
	if strings.Contains(got, "case 1") {
		t.Errorf("no per-case table without per-case answer data, got:\n%s", got)
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
