package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/termimg"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 15), uint8(y * 20), 200, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestImageURLs(t *testing.T) {
	md := "text ![a diagram](https://assets.leetcode.com/uploads/x.png) more " +
		"![](https://assets.leetcode.com/uploads/y.jpg) and a dup " +
		"![z](https://assets.leetcode.com/uploads/x.png)"
	got := imageURLs(md)
	if len(got) != 2 {
		t.Fatalf("got %d urls, want 2: %v", len(got), got)
	}
	if got[0] != "https://assets.leetcode.com/uploads/x.png" {
		t.Errorf("first url = %q", got[0])
	}
}

func TestRenderStatementFitsWidth(t *testing.T) {
	md := "# Letter Combinations of a Phone Number\n\n" +
		"Given a string containing digits from `2-9` inclusive, return all possible " +
		"letter combinations that the number could represent. Return the answer in any order.\n\n" +
		"A mapping of digits to letters (just like on the telephone buttons) is given below. " +
		"Note that 1 does not map to any letters.\n\n" +
		"- `1 <= digits.length <= 4`\n- `digits[i]` is a digit in the range `['2', '9']`.\n"
	const width = 44
	out, _ := renderStatementMD(newStatementRenderer(width), md, width, nil)
	for i, ln := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(ln); w > width {
			t.Fatalf("line %d is %d cells wide (> %d): %q", i, w, width, ln)
		}
	}
}

func TestRenderStatementFallbackWhenNoImages(t *testing.T) {
	md := "# Title\n\nSome text ![tree](https://assets.leetcode.com/uploads/t.png) end."
	out, _ := renderStatementMD(nil, md, 60, nil)
	if !strings.Contains(out, "⟨tree⟩") {
		t.Errorf("expected ⟨tree⟩ placeholder, got:\n%s", out)
	}
	if strings.Contains(out, "assets.leetcode.com") {
		t.Errorf("raw image markdown leaked:\n%s", out)
	}
}

func TestRenderStatementInlinesKnownImage(t *testing.T) {
	md := "before ![g](https://assets.leetcode.com/uploads/g.png) after"
	im, err := termimg.Decode(tinyPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	imgs := &statementImages{
		proto: termimg.ProtoBlocks,
		byURL: map[string]*termimg.Image{"https://assets.leetcode.com/uploads/g.png": im},
	}
	out, _ := renderStatementMD(nil, md, 60, imgs)
	if strings.Contains(out, "⟨g⟩") {
		t.Errorf("known image should not fall back to text:\n%s", out)
	}
	if !strings.Contains(out, "▀") { // block glyph from the rendered image
		t.Errorf("expected an inline block image, got:\n%s", out)
	}
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Errorf("surrounding text dropped:\n%s", out)
	}
}

// TestLinkifyURLsWrapsBareURL asserts a bare URL in plain (unrendered) text
// gets wrapped in exactly one OSC 8 hyperlink, with the visible text
// unchanged once the OSC 8 escapes are stripped back out.
func TestLinkifyURLsWrapsBareURL(t *testing.T) {
	in := "see https://example.com/docs for details"
	got := linkifyURLs(in)
	want := "see " + hyperlink("https://example.com/docs", "https://example.com/docs") + " for details"
	if got != want {
		t.Fatalf("linkifyURLs() =\n%q\nwant\n%q", got, want)
	}
	if strings.Count(got, "\x1b]8;;") != 2 { // one open + one close marker
		t.Errorf("expected exactly one hyperlink wrap (2 OSC8 markers), got %d in %q", strings.Count(got, "\x1b]8;;"), got)
	}
}

// TestLinkifyURLsTrimsTrailingSentencePunctuation keeps a URL's trailing
// sentence punctuation (or a markdown link's closing paren) outside the
// clickable range, matching how most linkifiers treat it.
func TestLinkifyURLsTrimsTrailingSentencePunctuation(t *testing.T) {
	cases := map[string]string{
		"visit https://example.com.":     "https://example.com",
		"see (https://example.com/x).":   "https://example.com/x",
		"link: https://example.com/x!!!": "https://example.com/x",
	}
	for in, wantURL := range cases {
		got := linkifyURLs(in)
		if !strings.Contains(got, "\x1b]8;;"+wantURL+"\x1b\\") {
			t.Errorf("linkifyURLs(%q) = %q, want it to wrap exactly %q", in, got, wantURL)
		}
	}
}

// TestLinkifyURLsStopsBeforeANSIEscape reproduces glamour's own link
// rendering shape: BaseElement prints the URL's own SGR-styled run with a
// style-reset code directly appended, no separating space (see
// LinkElement.renderHrefPart / BaseElement.Render in
// charmbracelet/glamour/ansi). The greedy \S+ a naive regex would use swallows
// that trailing escape into the "URL"; bareURLRe must not.
func TestLinkifyURLsStopsBeforeANSIEscape(t *testing.T) {
	rendered := "\x1b[4;30mhttps://example.com/path\x1b[0m and more text"
	got := linkifyURLs(rendered)
	if !strings.Contains(got, "\x1b]8;;https://example.com/path\x1b\\") {
		t.Fatalf("expected the hyperlink target to be exactly the URL (no trailing escape bytes), got %q", got)
	}
	// The original SGR start/reset codes must both survive intact, now
	// bracketing the OSC 8-wrapped URL rather than the bare URL text — the
	// visible styling is unaffected either way.
	want := "\x1b[4;30m" + hyperlink("https://example.com/path", "https://example.com/path") + "\x1b[0m and more text"
	if got != want {
		t.Fatalf("linkifyURLs must not disturb the surrounding ANSI styling:\ngot  %q\nwant %q", got, want)
	}
}

// TestLinkifyURLsNoBareURLIsNoop asserts text without a bare URL passes
// through unchanged.
func TestLinkifyURLsNoBareURLIsNoop(t *testing.T) {
	in := "no links here, just prose."
	if got := linkifyURLs(in); got != in {
		t.Fatalf("linkifyURLs(%q) = %q, want unchanged", in, got)
	}
}

// TestRenderStatementLinkifiesBareURLAfterGlamour is an end-to-end check that
// renderStatementMD's final output (real glamour rendering, not a hand-built
// string) gets its bare URL wrapped exactly once, and that glamour's own
// [text](url) link rendering — which prints the href as its own styled,
// visible run (see glamour/ansi/link.go) — isn't double-wrapped or corrupted:
// the plain (ANSI-stripped) text must be identical before and after
// linkifying, and each URL gets exactly one hyperlink wrap.
func TestRenderStatementLinkifiesBareURLAfterGlamour(t *testing.T) {
	md := "Bare link: https://example.com/bare\n\n" +
		"Markdown link: [the docs](https://example.com/md)\n"
	const width = 60
	out, _ := renderStatementMD(newStatementRenderer(width), md, width, nil)

	if !strings.Contains(out, "\x1b]8;;https://example.com/bare\x1b\\") {
		t.Errorf("bare URL should be hyperlink-wrapped, got:\n%s", out)
	}
	if !strings.Contains(out, "\x1b]8;;https://example.com/md\x1b\\") {
		t.Errorf("glamour's own printed href should also be hyperlink-wrapped, got:\n%s", out)
	}
	// Each URL should be wrapped exactly once (one open + one close marker),
	// not double-wrapped by a second pass over glamour's own link styling.
	if n := strings.Count(out, "\x1b]8;;https://example.com/bare\x1b\\"); n != 1 {
		t.Errorf("bare URL wrapped %d times, want 1:\n%s", n, out)
	}
	if n := strings.Count(out, "\x1b]8;;https://example.com/md\x1b\\"); n != 1 {
		t.Errorf("markdown link's href wrapped %d times, want 1:\n%s", n, out)
	}
	// The visible (ANSI+OSC8-stripped) text must still contain both URLs and
	// the surrounding prose, unmangled.
	plain := ansi.Strip(out)
	plain = strings.NewReplacer("\x1b]8;;https://example.com/bare\x1b\\", "",
		"\x1b]8;;https://example.com/md\x1b\\", "", "\x1b]8;;\x1b\\", "").Replace(plain)
	if !strings.Contains(plain, "Bare link:") || !strings.Contains(plain, "the docs") {
		t.Errorf("linkifying must not disturb surrounding prose, got:\n%s", plain)
	}
}

func TestProblemTitle(t *testing.T) {
	if got, want := problemTitle(1, "Two Sum", false), "1. Two Sum"; got != want {
		t.Errorf("problemTitle() = %q, want %q", got, want)
	}
	if got := problemTitle(1, "Two Sum", true); !strings.Contains(got, "🔒") || !strings.Contains(got, "1. Two Sum") {
		t.Errorf("problemTitle() paid-only = %q, want lock glyph + id/title", got)
	}
}

func TestHumanizeTag(t *testing.T) {
	cases := map[string]string{
		"array":               "Array",
		"dynamic-programming": "Dynamic Programming",
		"Array":               "Array",
	}
	for in, want := range cases {
		if got := humanizeTag(in); got != want {
			t.Errorf("humanizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRenderProblemHeaderShared locks in that browse (via BrowseRow.Meta) and
// the workspace (via leetcode.Question's fields) feed renderProblemHeader the
// same shape of metadata and get identical output for identical values — the
// two panes must never drift apart in structure or content.
func TestRenderProblemHeaderShared(t *testing.T) {
	th := DefaultTheme()
	row := BrowseRow{Difficulty: "Medium", ACRate: 53.3, PaidOnly: true, Tags: []string{"array", "math"}}
	q := leetcode.Question{Difficulty: "Medium", ACRate: 53.3, PaidOnly: true, Tags: []string{"array", "math"}}

	fromBrowse := renderProblemHeader(th, row.Meta(), 60)
	fromWorkspace := renderProblemHeader(th, ProblemMeta{
		Difficulty: q.Difficulty, ACRate: q.ACRate, PaidOnly: q.PaidOnly, Tags: q.Tags,
	}, 60)

	if fromBrowse != fromWorkspace {
		t.Errorf("browse and workspace headers diverged:\nbrowse:    %q\nworkspace: %q", fromBrowse, fromWorkspace)
	}
	if !strings.Contains(ansi.Strip(fromBrowse), "Medium") || !strings.Contains(ansi.Strip(fromBrowse), "AC 53.3%") {
		t.Errorf("header missing difficulty/AC%%: %q", ansi.Strip(fromBrowse))
	}
	if !strings.Contains(ansi.Strip(fromBrowse), "Array · Math") {
		t.Errorf("header missing humanized tags: %q", ansi.Strip(fromBrowse))
	}
	if !strings.Contains(ansi.Strip(fromBrowse), "paid-only") {
		t.Errorf("header missing paid-only marker: %q", ansi.Strip(fromBrowse))
	}
}
