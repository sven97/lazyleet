package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"

	"github.com/sven97/lazyleet/internal/termimg"
)

// ProblemMeta is the header metadata shown above a problem's statement —
// difficulty, acceptance rate, and topic tags. The browse Detail pane and the
// workspace Statement pane both derive it from their own data source
// (BrowseRow.Meta, or leetcode.Question directly) and render it through
// renderProblemHeader, so the two panes can never drift apart in structure or
// content. The title itself is left to the caller's pane/frame title.
type ProblemMeta struct {
	Difficulty string
	ACRate     float64
	PaidOnly   bool
	Tags       []string
}

// problemTitle formats the "ID. Title" heading shared by the browse Detail
// pane's frame title and the workspace Statement pane's pane title.
func problemTitle(frontendID int, title string, paidOnly bool) string {
	s := fmt.Sprintf("%d. %s", frontendID, title)
	if paidOnly {
		s = "🔒 " + s
	}
	return s
}

// humanizeTag turns a topic tag slug ("dynamic-programming") or an
// already-readable name ("Array") into display form ("Dynamic Programming").
func humanizeTag(s string) string {
	words := strings.Fields(strings.ReplaceAll(s, "-", " "))
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// renderProblemHeader renders the difficulty/acceptance/tags line shown above
// a problem's statement in both the browse Detail pane and the workspace
// Statement pane.
func renderProblemHeader(th Theme, meta ProblemMeta, width int) string {
	line := fmt.Sprintf("%s   AC %.1f%%", DifficultyStyle(meta.Difficulty).Render(meta.Difficulty), meta.ACRate)
	if meta.PaidOnly {
		line += "  " + th.ErrorText.Render("🔒 paid-only")
	}
	out := truncate(line, width)
	if len(meta.Tags) > 0 {
		tags := make([]string, len(meta.Tags))
		for i, t := range meta.Tags {
			tags[i] = humanizeTag(t)
		}
		out += "\n" + truncate(th.Muted.Render(strings.Join(tags, " · ")), width)
	}
	return out + "\n"
}

// queueImagePrefix hands the Kitty transmit+placement escapes to the
// ImageWriter so they're emitted just before the next frame (they can't ride in
// View — bubbletea truncates frame lines to the terminal width). `last` is the
// blob queued previously; the returned value should be stored back to dedupe.
func queueImagePrefix(iw *ImageWriter, prefix, last string) string {
	if iw == nil || prefix == "" || prefix == last {
		return last
	}
	iw.Queue(prefix)
	return prefix
}

// glamourMargin is how much wider than its word-wrap glamour's dark style can
// render a line (document margin + block indents). We wrap that much narrower
// and still clamp as a safety net.
const glamourMargin = 4

// newStatementRenderer builds a glamour renderer whose output fits within
// contentWidth cells.
//
// It uses a fixed "dark" style, NOT glamour.WithAutoStyle(): auto-style probes
// the terminal background with an OSC query and reads stdin for the reply, which
// steals keypresses when run (as we do) off the UI goroutine while bubbletea
// owns the terminal.
func newStatementRenderer(contentWidth int) *glamour.TermRenderer {
	w := contentWidth - glamourMargin
	if w < 20 {
		w = 20
	}
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(w))
	if err != nil {
		return nil
	}
	return r
}

// clampLines truncates each line to width cells (ANSI-aware), leaving lines that
// carry a terminal-graphics escape (image blocks) untouched.
func clampLines(s string, width int) string {
	if width < 1 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		// never touch image lines: Kitty transmit (\x1b_G) or a placeholder grid
		if strings.Contains(ln, "\x1b_G") || strings.Contains(ln, "\U0010EEEE") {
			continue
		}
		if ansi.StringWidth(ln) > width {
			lines[i] = ansi.Truncate(ln, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

// mdImageRe matches a Markdown image: group 1 = alt text, group 2 = URL.
var mdImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)[^)]*\)`)

// bareURLRe matches a bare http(s) URL in already glamour-rendered ANSI text.
// It deliberately excludes whitespace *and* ESC (\x1b) from the run: glamour
// often prints a style-reset code directly after a link's URL with no
// separating space (see LinkElement.Render in glamour/ansi/link.go), and \S+
// would otherwise swallow that escape sequence into the "URL" match.
var bareURLRe = regexp.MustCompile(`https?://[^\s\x1b]+`)

// urlTrailingPunct is punctuation a greedy URL match often absorbs from the
// surrounding sentence or markdown rather than the URL itself — a
// sentence-ending period, a quote closing a blockquote, etc. Left outside the
// hyperlink wrap. Brackets/parens are handled separately by
// urlBracketPairs below, since — unlike this flat set — whether one belongs
// to the URL depends on whether it's balanced.
const urlTrailingPunct = ".,;:!?\"'"

// urlBracketPairs are the trailing closing brackets splitTrailingURLPunct
// treats specially: a trailing closer is peeled off only when it ISN'T
// balanced by an opener earlier in the same match, so a URL that
// legitimately ends in a balanced pair (e.g. a Wikipedia-style
// ".../Rust_(programming_language)") keeps its own closing paren, while one
// genuinely absorbed from the surrounding markdown/sentence (e.g. "(see
// https://example.com)") still gets stripped.
var urlBracketPairs = map[byte]byte{')': '(', ']': '[', '}': '{'}

// splitTrailingURLPunct peels urlTrailingPunct runes, and unbalanced trailing
// brackets, off the end of u.
func splitTrailingURLPunct(u string) (core, trail string) {
	end := len(u)
	for end > 0 {
		c := u[end-1]
		if open, isCloser := urlBracketPairs[c]; isCloser {
			// u[:end] still includes c itself here. If opens are already
			// >= closes without it, c has no unmatched opener to pair with
			// — it's not part of the URL, strip it. Otherwise it closes a
			// real opener inside the URL — keep it.
			if strings.Count(u[:end], string(open)) >= strings.Count(u[:end], string(c)) {
				break
			}
			end--
			continue
		}
		if !strings.ContainsRune(urlTrailingPunct, rune(c)) {
			break
		}
		end--
	}
	return u[:end], u[end:]
}

// linkifyURLs finds bare URLs in fully-rendered (post-glamour) ANSI text and
// wraps each in an OSC 8 hyperlink (see hyperlink in render.go) so
// Cmd/Ctrl-click opens them in supporting terminals. It's meant to run as the
// very last step on a finished render — the regex naturally steps over ANSI
// escape sequences (they don't look like a URL) rather than fighting
// glamour's own link styling, whether the URL came from a bare link in the
// source markdown or is the visible href glamour prints beside a
// [text](url) link (see LinkElement.Render).
//
// Known limitation: running after glamour's word-wrap means a bare URL too
// long to fit one line arrives here already split across lines by a "\n" —
// bareURLRe stops at that newline, so only the first wrapped fragment gets
// linkified, and both the displayed text and the OSC 8 click target end up
// truncated to that fragment (a broken/dead link) rather than the real URL.
// Not fixed here: the real fix needs linkifying each raw markdown segment
// before glamour wraps it, so the hyperlink target is captured from the
// unwrapped source — a bigger, riskier change than warranted as a late
// addition, and one that needs its own tests around glamour's link
// rendering. Tracked in notes/PLAN.md's Phase 7 UI revamp entry.
func linkifyURLs(s string) string {
	return bareURLRe.ReplaceAllStringFunc(s, func(u string) string {
		core, trail := splitTrailingURLPunct(u)
		if core == "" {
			return u
		}
		return hyperlink(core, core) + trail
	})
}

// statementImages holds decoded statement images keyed by URL and the terminal
// protocol to render them with.
type statementImages struct {
	proto termimg.Protocol
	byURL map[string]*termimg.Image
}

// imageURLs returns the distinct image URLs referenced in a markdown statement.
func imageURLs(md string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range mdImageRe.FindAllStringSubmatch(md, -1) {
		if u := m[2]; !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// renderStatementMD renders a Markdown statement with glamour. When images are
// available and the protocol supports them, each image reference is replaced by
// an inline image block; otherwise it degrades to "⟨alt⟩" text. It returns the
// rendered body and, for the Kitty protocol, the transmit+placement escape
// prefix the caller must emit ahead of the frame.
//
// This is CPU-heavy (glamour + chroma + PNG-to-placeholder) — call it off the
// UI goroutine.
func renderStatementMD(r *glamour.TermRenderer, md string, contentWidth int, imgs *statementImages) (body, prefix string) {
	render := func(s string) string {
		if r == nil {
			return s
		}
		out, err := r.Render(s)
		if err != nil {
			return s
		}
		return clampLines(strings.TrimRight(out, "\n"), contentWidth)
	}

	locs := mdImageRe.FindAllStringSubmatchIndex(md, -1)
	if len(locs) == 0 {
		return linkifyURLs(clampLines(render(md), contentWidth)), ""
	}

	inline := imgs != nil && imgs.proto != termimg.ProtoNone
	imgW := contentWidth - 2
	if imgW > 72 {
		imgW = 72
	}
	if imgW < 8 {
		imgW = 8
	}

	var b, pfx strings.Builder
	last := 0
	for _, m := range locs {
		whole0, whole1 := m[0], m[1]
		alt := md[m[2]:m[3]]
		url := md[m[4]:m[5]]

		if seg := md[last:whole0]; strings.TrimSpace(seg) != "" {
			b.WriteString(render(seg))
			b.WriteByte('\n')
		}

		var im *termimg.Image
		if inline {
			im = imgs.byURL[url]
		}
		if im != nil {
			var block string
			if imgs.proto == termimg.ProtoKitty {
				block, _ = im.Placeholders(imgW)
				pfx.WriteString(im.Transmit(imgW)) // emitted ahead of the frame, not clipped
			} else {
				block, _ = im.Render(imgs.proto, imgW)
			}
			b.WriteString("\n")
			b.WriteString(block)
			b.WriteString("\n\n")
		} else {
			label := strings.TrimSpace(alt)
			if label == "" {
				label = "image"
			}
			b.WriteString("\n  ⟨" + label + "⟩\n\n")
		}
		last = whole1
	}
	if seg := md[last:]; strings.TrimSpace(seg) != "" {
		b.WriteString(render(seg))
	}
	return linkifyURLs(b.String()), pfx.String()
}
