package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"

	"github.com/sven97/lazyleet/internal/termimg"
)

// mdImageRe matches a Markdown image: group 1 = alt text, group 2 = URL.
var mdImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)[^)]*\)`)

// statementImages holds decoded statement images keyed by URL, plus the
// terminal protocol to render them with.
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
// an inline image block; otherwise it degrades to "⟨alt⟩" text.
func renderStatementMD(r *glamour.TermRenderer, md string, contentWidth int, imgs *statementImages) string {
	render := func(s string) string {
		if r == nil {
			return s
		}
		out, err := r.Render(s)
		if err != nil {
			return s
		}
		return strings.TrimRight(out, "\n")
	}

	locs := mdImageRe.FindAllStringSubmatchIndex(md, -1)
	if len(locs) == 0 {
		return render(md)
	}

	inline := imgs != nil && imgs.proto != termimg.ProtoNone
	imgW := contentWidth - 2
	if imgW > 72 {
		imgW = 72
	}
	if imgW < 8 {
		imgW = 8
	}

	var b strings.Builder
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
			body, _ := im.Render(imgs.proto, imgW)
			b.WriteString("\n")
			b.WriteString(body)
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
	return b.String()
}
