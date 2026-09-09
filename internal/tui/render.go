package tui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/lipgloss"

	"github.com/sven97/lazyleet/internal/runner"
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
			b.WriteString(th.Muted.Render("  out  ") + truncate(c.Actual, width-7) + "\n")
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
func renderRemote(th Theme, kind string, out *RemoteOutcome, err error, running bool, spin string, width int) string {
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

	if !out.Accepted && strings.TrimSpace(out.LastCase) != "" {
		b.WriteString("\n" + th.DiffDel.Render("failed on input") + "\n")
		for _, ln := range strings.Split(strings.TrimRight(out.LastCase, "\n"), "\n") {
			b.WriteString("  " + truncate(ln, width-3) + "\n")
		}
		b.WriteString(th.Muted.Render("press i to import this case as a local test") + "\n")
	}
	if !out.Accepted && len(out.Expected) > 0 && len(out.Actual) > 0 {
		b.WriteString("\n" + th.DiffAdd.Render("  exp  ") + truncate(strings.Join(out.Expected, " | "), width-7) + "\n")
		b.WriteString(th.DiffDel.Render("  got  ") + truncate(strings.Join(out.Actual, " | "), width-7) + "\n")
	}
	return b.String()
}

func truncate(s string, max int) string {
	if max < 1 {
		max = 1
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}
