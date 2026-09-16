package tui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sven97/lazyleet/internal/runner"
	"github.com/sven97/lazyleet/internal/testcase"
)

// chromaLexer maps a lazyleet language slug to a chroma lexer name.
func chromaLexer(lang string) string {
	switch lang {
	case "python3", "python":
		return "python"
	case "golang", "go":
		return "go"
	case "javascript":
		return "javascript"
	case "typescript":
		return "typescript"
	case "cpp", "c++":
		return "cpp"
	case "c":
		return "c"
	case "java":
		return "java"
	case "rust":
		return "rust"
	default:
		return "text"
	}
}

// highlightCode returns src with ANSI syntax colors, or src unchanged if
// highlighting fails.
func highlightCode(src, lang string) string {
	var b strings.Builder
	if err := quick.Highlight(&b, src, chromaLexer(lang), "terminal256", "github-dark"); err != nil {
		return src
	}
	return b.String()
}

// gutter prefixes each line with a right-aligned line number.
func gutter(text string, muted lipgloss.Style) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	width := len(fmt.Sprintf("%d", len(lines)))
	var b strings.Builder
	for i, ln := range lines {
		b.WriteString(muted.Render(fmt.Sprintf("%*d ", width, i+1)))
		b.WriteString(ln)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// localRunSummary is the one-line status-bar echo of a finished local run.
func localRunSummary(r runner.Result) string {
	switch {
	case r.BuildErr != "":
		return "local: build error"
	case r.Total == 0:
		return "local: no verified cases"
	default:
		return fmt.Sprintf("local: %d/%d passed", r.Passed, r.Total)
	}
}

// renderResults formats a run into the Results pane body.
func renderResults(th Theme, res *runner.Result, runErr error, running bool, spin string, width int) string {
	switch {
	case running:
		return th.Spinner.Render(spin) + " running local tests…"
	case runErr != nil:
		return th.ErrorText.Render("could not run: ") + runErr.Error()
	case res == nil:
		return th.Muted.Render("no run yet — press ") + "r" + th.Muted.Render(" or save the file")
	case res.BuildErr != "":
		return th.Fail.Render("build error") + "\n\n" + strings.TrimSpace(res.BuildErr)
	}

	var b strings.Builder
	verified := 0
	for _, c := range res.Cases {
		if c.Status != runner.StatusUnknown {
			verified++
		}
	}
	switch {
	case len(res.Cases) > 0 && verified == 0:
		// ran fine, but no example had an expected output to check against
		b.WriteString(th.Muted.Render(fmt.Sprintf("• %d ran · add an \"out\" to verify", len(res.Cases))))
	case res.Passed == verified:
		b.WriteString(th.Pass.Render(fmt.Sprintf("✓ %d/%d passed", res.Passed, verified)))
	default:
		b.WriteString(th.Fail.Render(fmt.Sprintf("✗ %d/%d passed", res.Passed, verified)))
	}
	b.WriteString(th.Muted.Render(fmt.Sprintf("  ·  %dms", res.Elapsed.Milliseconds())))
	b.WriteString("\n")

	for _, c := range res.Cases {
		b.WriteString("\n")
		switch c.Status {
		case runner.StatusPass:
			b.WriteString(th.Pass.Render(fmt.Sprintf("✓ case %d", c.Index+1)))
		case runner.StatusUnknown:
			b.WriteString(th.Muted.Render(fmt.Sprintf("• case %d (no expected output)", c.Index+1)))
		case runner.StatusTimeout:
			b.WriteString(th.Fail.Render(fmt.Sprintf("⏱ case %d timed out", c.Index+1)))
		case runner.StatusError:
			b.WriteString(th.Fail.Render(fmt.Sprintf("! case %d errored", c.Index+1)))
		default:
			b.WriteString(th.Fail.Render(fmt.Sprintf("✗ case %d", c.Index+1)))
		}
		b.WriteString(th.Muted.Render(fmt.Sprintf("  %.1fms", float64(c.Elapsed.Microseconds())/1000)))
		b.WriteString("\n")

		if len(c.Input) > 0 {
			b.WriteString(th.Muted.Render("  in   ") + truncate(strings.Join(c.Input, ", "), width-7) + "\n")
		}
		if c.Status == runner.StatusFail {
			b.WriteString(th.DiffAdd.Render("  exp  ") + truncate(c.Expected, width-7) + "\n")
			b.WriteString(th.DiffDel.Render("  got  ") + truncate(c.Actual, width-7) + "\n")
		}
		if c.Status == runner.StatusPass && c.Expected != "" {
			// Show both, even though they're equal on a pass: seeing the
			// actual expected/got values (not just the checkmark) is the
			// point of a "let me verify this myself" pass over the results.
			b.WriteString(th.Muted.Render("  exp  ") + truncate(c.Expected, width-7) + "\n")
			b.WriteString(th.Muted.Render("  got  ") + truncate(c.Actual, width-7) + "\n")
		}
		if c.Status == runner.StatusUnknown && c.Actual != "" {
			b.WriteString(th.Muted.Render("  got  ") + truncate(c.Actual, width-7) + "\n")
		}
		if c.Err != "" {
			b.WriteString(th.ErrorText.Render("  err  ") + truncate(c.Err, width-7) + "\n")
		}
		if s := strings.TrimSpace(c.Stdout); s != "" {
			b.WriteString(th.Muted.Render("  log  ") + truncate(strings.ReplaceAll(s, "\n", "⏎"), width-7) + "\n")
		}
	}
	return b.String()
}

// renderRemote formats a LeetCode run/submit outcome for the Results pane.
// sentCases is exactly what was sent as a Run Code request's data input
// (ignored for "submit") — used to label each response case's input.
func renderRemote(th Theme, kind string, out *RemoteOutcome, err error, running bool, spin string, width int, sentCases []testcase.Case) string {
	verb := "running on LeetCode"
	if kind == "submit" {
		verb = "submitting to LeetCode"
	}
	switch {
	case running:
		return th.Spinner.Render(spin) + " " + verb + "…"
	case err != nil:
		return th.ErrorText.Render(verb+" failed: ") + err.Error()
	case out == nil:
		return th.Muted.Render("no LeetCode run yet — press R to run, s to submit")
	}

	var b strings.Builder
	head := out.Verdict
	if out.Total > 0 {
		head += fmt.Sprintf("  %d/%d", out.Passed, out.Total)
	}
	if out.Accepted {
		b.WriteString(th.Pass.Render("✓ " + head))
	} else {
		b.WriteString(th.Fail.Render("✗ " + head))
	}
	b.WriteString(th.Muted.Render("   [" + kind + " @ LeetCode]"))
	b.WriteString("\n")

	if out.Runtime != "" {
		line := "  runtime  " + out.Runtime
		if out.RuntimePct > 0 {
			line += fmt.Sprintf("  (beats %.1f%%)", out.RuntimePct)
		}
		b.WriteString(th.Muted.Render(line) + "\n")
	}
	if out.Memory != "" {
		line := "  memory   " + out.Memory
		if out.MemoryPct > 0 {
			line += fmt.Sprintf("  (beats %.1f%%)", out.MemoryPct)
		}
		b.WriteString(th.Muted.Render(line) + "\n")
	}

	if s := strings.TrimSpace(out.CompileErr); s != "" {
		b.WriteString("\n" + th.Fail.Render("compile error") + "\n" + s + "\n")
	}
	if s := strings.TrimSpace(out.RuntimeErr); s != "" {
		b.WriteString("\n" + th.Fail.Render("runtime error") + "\n" + s + "\n")
	}

	switch {
	case kind == "run" && len(out.Expected) > 0 && len(out.Actual) > 0:
		// Run Code always evaluates a small, known set of cases (the visible
		// examples, or whatever you gave it) — show all of them, input,
		// expected, and actual, not just the one(s) that failed. That's what
		// lets a "passed" result be checked rather than taken on faith.
		b.WriteString(renderRunCases(th, out, sentCases, width))
		if !out.Accepted && strings.TrimSpace(out.LastCase) != "" {
			b.WriteString(th.Muted.Render("press i to import the failing case(s) as local tests") + "\n")
		}
	case !out.Accepted && strings.TrimSpace(out.LastCase) != "":
		// Submit only ever surfaces one case: the first hidden test that
		// failed. Its input is LastCase; mapOutcome already fills Expected/
		// Actual from that same case's actual/expected value when LeetCode's
		// response carries them (a submission check leaves the per-case
		// arrays Run Code uses empty, but has separate single-value fields
		// for this one case — see JudgeResult's field comments).
		b.WriteString("\n" + th.DiffDel.Render("failed on input") + "\n")
		for _, ln := range strings.Split(strings.TrimRight(out.LastCase, "\n"), "\n") {
			b.WriteString("  " + truncate(ln, width-3) + "\n")
		}
		b.WriteString(th.Muted.Render("press i to import this case as a local test") + "\n")
		if len(out.Expected) > 0 && len(out.Actual) > 0 {
			b.WriteString(th.DiffAdd.Render("  exp  ") + truncate(out.Expected[0], width-7) + "\n")
			b.WriteString(th.DiffDel.Render("  got  ") + truncate(out.Actual[0], width-7) + "\n")
		}
	}
	return b.String()
}

// renderRunCases lists every case a Run Code request evaluated: input,
// expected, and actual, each marked pass/fail. Unlike Submit (up to hundreds
// of hidden cases, only one of which — the first failure — is ever visible),
// Run Code only ever covers a handful of cases you can see in full.
//
// The input column comes from sentCases (what was actually sent), not from
// parsing the response apart — LeetCode's last_testcase isn't reliably
// populated for a Run Code request the way it is for a real submission's one
// failing case, so there's often nothing there to parse.
func renderRunCases(th Theme, out *RemoteOutcome, sentCases []testcase.Case, width int) string {
	n := len(out.Expected)
	if len(out.Actual) < n {
		n = len(out.Actual)
	}
	if out.Total > 0 && out.Total < n {
		n = out.Total
	}
	if cr := out.CompareResult; cr != "" && len(cr) < n {
		n = len(cr)
	}
	if n == 0 {
		return ""
	}

	wrong := func(i int) bool {
		if cr := out.CompareResult; i < len(cr) {
			return cr[i] != '1'
		}
		return out.Expected[i] != out.Actual[i]
	}

	var b strings.Builder
	for i := 0; i < n; i++ {
		mark, label := th.Pass.Render("✓"), th.Muted.Render(fmt.Sprintf("case %d", i+1))
		expStyle, gotStyle := th.Muted, th.Muted // matching values on a pass — no diff to highlight
		if wrong(i) {
			mark, label = th.Fail.Render("✗"), th.Fail.Render(fmt.Sprintf("case %d", i+1))
			expStyle, gotStyle = th.DiffAdd, th.DiffDel
		}
		b.WriteString("\n" + mark + " " + label + "\n")
		if i < len(sentCases) {
			b.WriteString(th.Muted.Render("  in   ") + truncate(strings.Join(sentCases[i].In, ", "), width-7) + "\n")
		}
		b.WriteString(expStyle.Render("  exp  ") + truncate(out.Expected[i], width-7) + "\n")
		b.WriteString(gotStyle.Render("  got  ") + truncate(out.Actual[i], width-7) + "\n")
	}
	return b.String()
}

// truncate shortens s to at most max terminal cells, adding an ellipsis when it
// cuts. It measures by display width and is ANSI- and grapheme-aware, so a wide
// glyph (emoji like 🔒, CJK) or an embedded style sequence can't push the line
// past max and wrap onto the next row.
func truncate(s string, max int) string {
	if max < 1 {
		max = 1
	}
	return ansi.Truncate(s, max, "…")
}

// renderSplitStatusLine lays out a status/spinner message flush right and the
// shortcut hints flush left, so status text changing length (a sync
// progressing, a run finishing) never shifts the hints out from under the
// user's fingers — hints always start at column 0, whatever their length.
//
// Status gets priority for space: it's live feedback for whatever the user
// just triggered (a run/submit verdict, a sync result, an actionable "run
// `lazyleet auth`" nudge), while the hints are static reference material
// available in full via `?` help. So hints truncate (and finally disappear)
// to make room for it, not the other way around; status is only truncated if
// it alone doesn't fit the line.
func renderSplitStatusLine(hints, status string, w int) string {
	if status == "" {
		return truncate(hints, w)
	}
	statusW := lipgloss.Width(status)
	if statusW >= w {
		return truncate(status, w)
	}
	avail := w - statusW - 1 // reserve one separating space for hints
	if avail <= 0 {
		return truncate(status, w)
	}
	hints = truncate(hints, avail)
	gap := w - lipgloss.Width(hints) - statusW
	return hints + strings.Repeat(" ", gap) + status
}
