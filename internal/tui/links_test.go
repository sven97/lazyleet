package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestLinkAtCell(t *testing.T) {
	const (
		st  = "\x1b\\"
		bel = "\a"
	)
	open := func(u string) string { return "\x1b]8;;" + u + st }
	closeSeq := "\x1b]8;;" + st

	for _, tc := range []struct {
		name string
		line string
		col  int
		want string // "" means no link
	}{
		{"first cell", "ab " + open("https://a.example") + "link" + closeSeq + " z", 3, "https://a.example"},
		{"last cell", "ab " + open("https://a.example") + "link" + closeSeq + " z", 6, "https://a.example"},
		{"cell before", "ab " + open("https://a.example") + "link" + closeSeq + " z", 2, ""},
		{"cell after", "ab " + open("https://a.example") + "link" + closeSeq + " z", 7, ""},
		{"no link on line", "plain text", 3, ""},
		{"second of two links", open("https://a.example") + "aa" + closeSeq + " " + open("https://b.example") + "bb" + closeSeq, 4, "https://b.example"},
		{"gap between two links", open("https://a.example") + "aa" + closeSeq + " " + open("https://b.example") + "bb" + closeSeq, 2, ""},
		{"styled display text", open("https://a.example") + "\x1b[4mlink\x1b[0m" + closeSeq, 2, "https://a.example"},
		{"BEL terminator", "\x1b]8;;https://a.example" + bel + "link" + "\x1b]8;;" + bel, 1, "https://a.example"},
		{"params before the URI", "\x1b]8;id=1;https://a.example" + st + "link" + closeSeq, 1, "https://a.example"},
		{"link left open runs to end of line", open("https://a.example") + "link", 3, "https://a.example"},
		{"open link, click past end of text", open("https://a.example") + "link", 4, ""},
		{"wide glyphs count two cells", "世界" + open("https://a.example") + "x" + closeSeq, 4, "https://a.example"},
		{"unterminated sequence", "ab \x1b]8;;https://a.example", 3, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := linkAtCell(tc.line, tc.col)
			if tc.want == "" {
				if ok {
					t.Fatalf("linkAtCell(col %d) = %q, want no link", tc.col, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("linkAtCell(col %d) = %q, %v, want %q", tc.col, got, ok, tc.want)
			}
		})
	}
}

func TestOpenLinkOnlyAllowsHTTP(t *testing.T) {
	var opened []string
	orig := openURL
	openURL = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openURL = orig })

	for _, ok := range []string{"https://github.com/sven97/lazyleet", "http://example.com/a?b=c"} {
		if err := openLink(ok); err != nil {
			t.Errorf("openLink(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"file:///etc/passwd", "javascript:alert(1)", "ftp://example.com", "https://", "not a url", ""} {
		if err := openLink(bad); err == nil {
			t.Errorf("openLink(%q) succeeded, want an error", bad)
		}
	}
	if len(opened) != 2 {
		t.Errorf("opener called for %v, want only the two http(s) URLs", opened)
	}
}

// statusAboutModel is a booted browse model whose Detail pane is showing the
// Status/About panel, as it does after pressing 1.
func statusAboutModel(t *testing.T) *BrowseModel {
	t.Helper()
	m, _ := bootBrowse(t)
	m.SetVersion("v1.2.3")
	m.setFocus(RegionStatus)
	m.refreshDetail()
	return m
}

// screenCellOfLink returns the screen coordinates of a cell inside the
// rendered link to target in the Detail pane, failing the test if it isn't
// there. This is what a user would click on.
func screenCellOfLink(t *testing.T, m *BrowseModel, target string) (x, y int) {
	t.Helper()
	marker := "\x1b]8;;" + target + "\x1b\\"
	for row, line := range strings.Split(m.previewVP.View(), "\n") {
		if i := strings.Index(line, marker); i >= 0 {
			r := m.layout.Detail
			return r.X + 1 + ansi.StringWidth(line[:i]) + 2, r.Y + 1 + row
		}
	}
	t.Fatalf("no link to %s in the Detail pane:\n%s", target, m.previewVP.View())
	return 0, 0
}

// stubOpener replaces the OS opener for one test and returns what it was
// asked to open.
func stubOpener(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	orig := openURL
	openURL = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openURL = orig })
	return &opened
}

func TestStatusPanelLinksOpenOnClick(t *testing.T) {
	for _, target := range []string{lazyleetRepoURL, lazyleetIssuesURL, lazyleetReleasesURL} {
		t.Run(target, func(t *testing.T) {
			opened := stubOpener(t)
			m := statusAboutModel(t)
			x, y := screenCellOfLink(t, m, target)

			step(&m, press(x, y))
			if len(*opened) != 0 {
				t.Fatalf("link opened on press, before the click finished: %v", *opened)
			}
			step(&m, release(x, y))

			if len(*opened) != 1 || (*opened)[0] != target {
				t.Fatalf("opened %v, want exactly [%s]", *opened, target)
			}
			if !strings.Contains(m.statusMsg, target) {
				t.Errorf("expected a status-bar confirmation naming the link, got %q", m.statusMsg)
			}
		})
	}
}

func TestStatusPanelClickOffLinkOpensNothing(t *testing.T) {
	opened := stubOpener(t)
	m := statusAboutModel(t)
	x, y := screenCellOfLink(t, m, lazyleetRepoURL)
	r := m.layout.Detail

	// Same row, but the blank space far to the right of the link's text.
	farX := r.X + r.W - 3
	step(&m, press(farX, y))
	step(&m, release(farX, y))
	// And the row above the link block, which holds no link.
	step(&m, press(x, y-2))
	step(&m, release(x, y-2))

	if len(*opened) != 0 {
		t.Fatalf("opened %v for clicks that weren't on a link", *opened)
	}
}

func TestDraggingAcrossALinkSelectsInsteadOfOpening(t *testing.T) {
	opened := stubOpener(t)
	m := statusAboutModel(t)
	tc := newTestClipboard(t)
	m.imgWriter = tc.iw
	x, y := screenCellOfLink(t, m, lazyleetRepoURL)

	step(&m, press(x, y))
	step(&m, motion(x+5, y))
	step(&m, release(x+5, y))

	if len(*opened) != 0 {
		t.Fatalf("a drag opened %v; it should only select", *opened)
	}
	if got := decodeOSC52(t, tc.flush(t)); got == "" {
		t.Error("the drag should still have copied the selected text")
	}
}

func TestClickOnNonHTTPLinkIsRefused(t *testing.T) {
	opened := stubOpener(t)
	m, _ := bootBrowse(t)
	m.setFocus(RegionDetail)
	m.detailShowsStatus = false
	m.previewVP.SetContent("\x1b]8;;file:///etc/passwd\x1b\\evil\x1b]8;;\x1b\\")
	m.previewContentSlug = m.currentSlug()
	r := m.layout.Detail

	step(&m, press(r.X+1+1, r.Y+1))
	step(&m, release(r.X+1+1, r.Y+1))

	if len(*opened) != 0 {
		t.Fatalf("opened %v; non-http links must never reach the OS opener", *opened)
	}
	if !strings.Contains(m.statusMsg, "couldn't open link") {
		t.Errorf("expected the refusal to be reported, got %q", m.statusMsg)
	}
}
