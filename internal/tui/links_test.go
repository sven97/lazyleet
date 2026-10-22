package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	return screenCellOfLinkIn(t, m.previewVP.View(), m.layout.Detail, target)
}

// screenCellOfLinkIn is screenCellOfLink for any bordered pane: view is the
// pane viewport's View() and r the pane's outer rect.
func screenCellOfLinkIn(t *testing.T, view string, r Rect, target string) (x, y int) {
	t.Helper()
	marker := "\x1b]8;;" + target + "\x1b\\"
	for row, line := range strings.Split(view, "\n") {
		if i := strings.Index(line, marker); i >= 0 {
			return r.X + 1 + ansi.StringWidth(line[:i]) + 2, r.Y + 1 + row
		}
	}
	t.Fatalf("no link to %s in the pane:\n%s", target, view)
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

// --- workspace panes -----------------------------------------------------

const stmtLinkURL = "https://example.com/docs/page"

// workspaceWithStatementLink is a workspace whose Statement pane renders a
// bare URL through the real markdown + linkify pipeline, focused on that pane.
func workspaceWithStatementLink(t *testing.T) *WorkspaceModel {
	t.Helper()
	m := newTestWorkspaceModel(t)
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40}) // wide: two-column layout
	m = up.(*WorkspaceModel)
	m.q.Statement = "See " + stmtLinkURL + " for details."
	m.focused = PaneStatement
	m.relayout()
	m.refreshStatement()
	return m
}

func stepWS(m **WorkspaceModel, msg tea.Msg) {
	up, _ := (*m).Update(msg)
	*m = up.(*WorkspaceModel)
}

func TestWorkspaceStatementLinkOpensOnClick(t *testing.T) {
	opened := stubOpener(t)
	m := workspaceWithStatementLink(t)
	x, y := screenCellOfLinkIn(t, m.statement.View(), m.layout.Statement, stmtLinkURL)

	stepWS(&m, press(x, y))
	if len(*opened) != 0 {
		t.Fatalf("link opened on press, before the click finished: %v", *opened)
	}
	stepWS(&m, release(x, y))

	if len(*opened) != 1 || (*opened)[0] != stmtLinkURL {
		t.Fatalf("opened %v, want exactly [%s]", *opened, stmtLinkURL)
	}
	if !strings.Contains(m.statusMsg, stmtLinkURL) {
		t.Errorf("expected a status-bar confirmation naming the link, got %q", m.statusMsg)
	}
}

func TestWorkspaceClickOffLinkOpensNothing(t *testing.T) {
	opened := stubOpener(t)
	m := workspaceWithStatementLink(t)
	_, y := screenCellOfLinkIn(t, m.statement.View(), m.layout.Statement, stmtLinkURL)
	r := m.layout.Statement

	// The first cell of the link's row is the "See " before the URL; the far
	// right of the same row is blank space past it.
	for _, x := range []int{r.X + 1, r.X + r.W - 3} {
		stepWS(&m, press(x, y))
		stepWS(&m, release(x, y))
	}
	if len(*opened) != 0 {
		t.Fatalf("opened %v for clicks that weren't on a link", *opened)
	}
}

func TestWorkspaceDraggingAcrossALinkSelectsInsteadOfOpening(t *testing.T) {
	opened := stubOpener(t)
	m := workspaceWithStatementLink(t)
	tc := newTestClipboard(t)
	m.imgWriter = tc.iw
	x, y := screenCellOfLinkIn(t, m.statement.View(), m.layout.Statement, stmtLinkURL)

	stepWS(&m, press(x, y))
	stepWS(&m, motion(x+5, y))
	stepWS(&m, release(x+5, y))

	if len(*opened) != 0 {
		t.Fatalf("a drag opened %v; it should only select", *opened)
	}
	if got := decodeOSC52(t, tc.flush(t)); got == "" {
		t.Error("the drag should still have copied the selected text")
	}
}

// A URL in the Code pane is just text in the user's source, not a hyperlink,
// so clicking it must do nothing.
func TestWorkspaceCodePaneURLIsNotClickable(t *testing.T) {
	opened := stubOpener(t)
	m := newTestWorkspaceModel(t)
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)
	m.focused = PaneCode
	m.relayout()
	m.code.SetContent("# see " + stmtLinkURL)
	r := m.layout.Code
	x, y := r.X+1+8, r.Y+1

	stepWS(&m, press(x, y))
	stepWS(&m, release(x, y))

	if len(*opened) != 0 {
		t.Fatalf("opened %v for a plain-text URL in the Code pane", *opened)
	}
}
