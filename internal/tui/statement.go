package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"

	"github.com/sven97/lazyleet/internal/termimg"
)

// queueImagePrefix hands the Kitty transmit+placement escapes to the
// ImageWriter so they're emitted just before the next frame (they can't ride in
// View — bubbletea truncates frame lines to the terminal width). `last` is the
// blob queued previously; the returned value should be stored back to dedupe.
func queueImagePrefix(iw *ImageWriter, si *statementImages, last string) string {
	if iw == nil || si == nil || si.prefix == "" || si.prefix == last {
		return last
	}
	iw.Queue(si.prefix)
	return si.prefix
}

// glamourMargin is how much wider than its word-wrap glamour's dark style can
// render a line (document margin + block indents). We wrap that much narrower
// and still clamp as a safety net.
const glamourMargin = 4

// newStatementRenderer builds a glamour renderer whose output fits within
// contentWidth cells.
func newStatementRenderer(contentWidth int) *glamour.TermRenderer {
	w := contentWidth - glamourMargin
	if w < 20 {
		w = 20
	}
	r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(w))
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

// statementImages holds decoded statement images keyed by URL, the terminal
// protocol to render them with, and the Kitty transmit+placement prefix that
// renderStatementMD produced for the current width (emitted once as a frame
// prefix so it isn't clipped by a viewport).
type statementImages struct {
	proto  termimg.Protocol
	byURL  map[string]*termimg.Image
	prefix string
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
// an inline image block; otherwise it degrades to "⟨alt⟩" text. For the Kitty
// protocol it also fills imgs.prefix with the transmit+placement escapes the
// caller must emit as a frame prefix.
func renderStatementMD(r *glamour.TermRenderer, md string, contentWidth int, imgs *statementImages) string {
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
	if imgs != nil {
		imgs.prefix = ""
	}
	if len(locs) == 0 {
		return clampLines(render(md), contentWidth)
	}

	inline := imgs != nil && imgs.proto != termimg.ProtoNone
	imgW := contentWidth - 2
	if imgW > 72 {
		imgW = 72
	}
	if imgW < 8 {
		imgW = 8
	}

	var b, prefix strings.Builder
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
				prefix.WriteString(im.Transmit(imgW)) // emitted as a frame prefix, not clipped
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
	if imgs != nil {
		imgs.prefix = prefix.String()
	}
	return b.String()
}
