package tui

import (
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/workspace"
)

// testClipboard wraps an *ImageWriter around an os.Pipe so a test can capture
// the exact bytes a copy handed to it — the same mechanism cmd/lazyleet
// wires up via tea.WithOutput(iw) in real use (see imgwriter.go). Nothing in
// this package writes to the pipe until flush() forces it (mirroring
// bubbletea calling iw.Write on its next render), so tests stay
// deterministic without needing a real terminal or goroutine synchronization.
type testClipboard struct {
	iw *ImageWriter
	r  *os.File
	w  *os.File
}

func newTestClipboard(t *testing.T) *testClipboard {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	tc := &testClipboard{iw: NewImageWriter(w), r: r, w: w}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	return tc
}

// flush forces any bytes Queue()d so far out to the pipe (simulating
// bubbletea's next render calling iw.Write) and returns everything written.
func (tc *testClipboard) flush(t *testing.T) string {
	t.Helper()
	if _, err := tc.iw.Write(nil); err != nil {
		t.Fatalf("flush: %v", err)
	}
	tc.w.Close()
	data, err := io.ReadAll(tc.r)
	if err != nil {
		t.Fatalf("flush: read: %v", err)
	}
	return string(data)
}

// decodeOSC52 extracts and base64-decodes the payload of a single OSC 52
// copy sequence, failing the test if raw doesn't look like exactly one.
func decodeOSC52(t *testing.T, raw string) string {
	t.Helper()
	const prefix, suffix = "\x1b]52;c;", "\x07"
	if !strings.HasPrefix(raw, prefix) || !strings.HasSuffix(raw, suffix) {
		t.Fatalf("not a single OSC 52 copy sequence: %q", raw)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(raw, prefix), suffix)
	dec, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("invalid base64 payload: %v", err)
	}
	return string(dec)
}

func motion(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft}
}
func release(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
}

// dragSelectionLines is the fixed content used across the drag tests below —
// plain, unstyled text so the exact copied bytes are predictable regardless
// of glamour/markdown rendering:
//
//	row0: "hello world"
//	row1: "second line here"
//	row2: "third partial line"
const (
	dragLine0 = "hello world"
	dragLine1 = "second line here"
	dragLine2 = "third partial line"
)

var dragContent = strings.Join([]string{dragLine0, dragLine1, dragLine2}, "\n")

// TestBrowseDetailDragSelectionCopiesExactPlainText drives a full
// press->motion->release tea.MouseMsg sequence over the browse Detail pane
// and asserts the exact plain text handed to the OSC 52 writer, covering a
// forward multi-line drag (partial first line, full middle line, partial
// last line).
func TestBrowseDetailDragSelectionCopiesExactPlainText(t *testing.T) {
	m, _ := bootBrowse(t)
	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.setFocus(RegionDetail)
	m.detailShowsStatus = false
	m.previewVP.SetContent(dragContent)
	m.previewContentSlug = m.currentSlug()

	r := m.layout.Detail
	anchorX, anchorY := r.X+1+6, r.Y+1+0 // "world" starts at column 6 of row 0
	releaseX, releaseY := r.X+1+4, r.Y+1+2

	step(&m, press(anchorX, anchorY))
	step(&m, motion(releaseX, releaseY))
	step(&m, release(releaseX, releaseY))

	want := "world\n" + dragLine1 + "\nthird"
	if got := decodeOSC52(t, tc.flush(t)); got != want {
		t.Fatalf("copied text =\n%q\nwant\n%q", got, want)
	}
	if !strings.Contains(m.statusMsg, "copied") {
		t.Errorf("expected a status-bar copy confirmation, got %q", m.statusMsg)
	}
}

// TestBrowseDetailDragSurvivesOverlayOpenedMidDrag guards against a
// regression where handleMouse's filtering/showHelp/topicPicker guard ran
// before the active-drag check: opening any of those overlays via the
// keyboard between a drag's press and release (e.g. pressing `?` mid-drag)
// swallowed the release, leaving detailSel.active stuck true with no way to
// finish or clear the selection until the next click.
func TestBrowseDetailDragSurvivesOverlayOpenedMidDrag(t *testing.T) {
	m, _ := bootBrowse(t)
	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.setFocus(RegionDetail)
	m.detailShowsStatus = false
	m.previewVP.SetContent(dragContent)
	m.previewContentSlug = m.currentSlug()

	r := m.layout.Detail
	anchorX, anchorY := r.X+1+6, r.Y+1+0
	releaseX, releaseY := r.X+1+4, r.Y+1+2

	step(&m, press(anchorX, anchorY))
	step(&m, motion(releaseX, releaseY))
	m.showHelp = true // simulate `?` opening the help overlay mid-drag
	step(&m, release(releaseX, releaseY))
	m.showHelp = false

	want := "world\n" + dragLine1 + "\nthird"
	if got := decodeOSC52(t, tc.flush(t)); got != want {
		t.Fatalf("copied text =\n%q\nwant\n%q", got, want)
	}
	if m.detailSel.active {
		t.Error("detailSel.active should be false after release, even though an overlay was open when it arrived")
	}
}

// TestBrowseDetailReversedDragCopiesSameText mirrors the drag above but
// starts the press where the previous test released, and releases where it
// pressed — a reversed drag must normalize to the exact same copied text.
func TestBrowseDetailReversedDragCopiesSameText(t *testing.T) {
	m, _ := bootBrowse(t)
	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.setFocus(RegionDetail)
	m.detailShowsStatus = false
	m.previewVP.SetContent(dragContent)
	m.previewContentSlug = m.currentSlug()

	r := m.layout.Detail
	pressX, pressY := r.X+1+4, r.Y+1+2     // row 2, col 4 (the forward test's release point)
	releaseX, releaseY := r.X+1+6, r.Y+1+0 // row 0, col 6 (the forward test's press point)

	step(&m, press(pressX, pressY))
	step(&m, motion(releaseX, releaseY))
	step(&m, release(releaseX, releaseY))

	want := "world\n" + dragLine1 + "\nthird"
	if got := decodeOSC52(t, tc.flush(t)); got != want {
		t.Fatalf("reversed-drag copied text =\n%q\nwant\n%q", got, want)
	}
}

// TestBrowseDetailPlainClickDoesNotCopy asserts a press+release with no
// movement in between (an ordinary click, e.g. to focus the pane) doesn't
// trigger a clipboard write.
func TestBrowseDetailPlainClickDoesNotCopy(t *testing.T) {
	m, _ := bootBrowse(t)
	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.setFocus(RegionDetail)
	m.detailShowsStatus = false
	m.previewVP.SetContent(dragContent)
	m.previewContentSlug = m.currentSlug()

	r := m.layout.Detail
	x, y := r.X+1+2, r.Y+1+1
	step(&m, press(x, y))
	step(&m, release(x, y))

	if got := tc.flush(t); got != "" {
		t.Fatalf("a plain click should not copy anything, got %q", got)
	}
}

// TestBrowseDetailCopyKeyFallsBackToWholePane asserts the `y` keyboard
// fallback copies the whole visible pane when there's no active selection.
func TestBrowseDetailCopyKeyFallsBackToWholePane(t *testing.T) {
	m, _ := bootBrowse(t)
	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.setFocus(RegionDetail)
	m.detailShowsStatus = false
	m.previewVP.SetContent(dragContent)
	m.previewContentSlug = m.currentSlug()

	step(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	got := decodeOSC52(t, tc.flush(t))
	for _, want := range []string{dragLine0, dragLine1, dragLine2} {
		if !strings.Contains(got, want) {
			t.Errorf("whole-pane copy missing line %q, got %q", want, got)
		}
	}
}

// TestWorkspaceCodeDragSelectionCopiesExactPlainText is the workspace-side
// equivalent of the browse Detail drag test above, over PaneCode.
func TestWorkspaceCodeDragSelectionCopiesExactPlainText(t *testing.T) {
	q, _ := leetcode.Fixture("two-sum")
	ws, err := workspace.Scaffold(t.TempDir(), q, "python3")
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewWorkspaceModel(ws, q, "true", false, 400, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)

	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.focused = PaneCode
	m.relayout()
	m.code.SetContent(dragContent)

	r := m.layout.RectFor(PaneCode)
	anchorX, anchorY := r.X+1+6, r.Y+1+0
	releaseX, releaseY := r.X+1+4, r.Y+1+2

	upModel, _ := m.Update(press(anchorX, anchorY))
	m = upModel.(*WorkspaceModel)
	upModel, _ = m.Update(motion(releaseX, releaseY))
	m = upModel.(*WorkspaceModel)
	upModel, _ = m.Update(release(releaseX, releaseY))
	m = upModel.(*WorkspaceModel)

	want := "world\n" + dragLine1 + "\nthird"
	if got := decodeOSC52(t, tc.flush(t)); got != want {
		t.Fatalf("copied text =\n%q\nwant\n%q", got, want)
	}
	if !strings.Contains(m.statusMsg, "copied") {
		t.Errorf("expected a status-bar copy confirmation, got %q", m.statusMsg)
	}
}

// TestWorkspaceResultsRefreshDuringActiveDragDoesNotClearSelection guards
// against a regression where refreshResults — called on every spinner.TickMsg
// while a run/submit is in flight — unconditionally cleared an in-progress
// click-drag selection in the Results pane, silently dropping it before the
// user could release the mouse: no copy, no error, no status message.
func TestWorkspaceResultsRefreshDuringActiveDragDoesNotClearSelection(t *testing.T) {
	q, _ := leetcode.Fixture("two-sum")
	ws, err := workspace.Scaffold(t.TempDir(), q, "python3")
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewWorkspaceModel(ws, q, "true", false, 400, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)

	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.focused = PaneResults
	m.relayout()
	m.results.SetContent(dragContent)

	r := m.layout.RectFor(PaneResults)
	anchorX, anchorY := r.X+1+6, r.Y+1+0
	releaseX, releaseY := r.X+1+4, r.Y+1+2

	upModel, _ := m.Update(press(anchorX, anchorY))
	m = upModel.(*WorkspaceModel)
	upModel, _ = m.Update(motion(releaseX, releaseY))
	m = upModel.(*WorkspaceModel)

	// Simulate a spinner tick firing mid-drag — the real-world trigger:
	// refreshResults runs on every spinner.TickMsg while a run/submit is in
	// progress (m.running / m.showRemote+m.remoteRunning).
	m.running = true
	m.refreshResults()
	if !m.sel.active {
		t.Fatal("refreshResults during an active drag must not clear it")
	}
	m.running = false
	m.results.SetContent(dragContent) // restore predictable content for the release below

	// The returned model is never read again after this — the assertions
	// below only need tc.flush(t) — so it's discarded rather than reassigned
	// into m (staticcheck SA4006: dead store).
	m.Update(release(releaseX, releaseY))

	want := "world\n" + dragLine1 + "\nthird"
	if got := decodeOSC52(t, tc.flush(t)); got != want {
		t.Fatalf("copied text =\n%q\nwant\n%q", got, want)
	}
}

// TestWorkspaceDragSurvivesHintsOpenedMidDrag is the workspace-side
// equivalent of TestBrowseDetailDragSurvivesOverlayOpenedMidDrag: opening the
// Hints overlay (the `H` key) between a drag's press and release must not
// swallow the release, since m.showHints was previously checked before the
// active-drag routing in handleMouse.
func TestWorkspaceDragSurvivesHintsOpenedMidDrag(t *testing.T) {
	q, _ := leetcode.Fixture("two-sum")
	ws, err := workspace.Scaffold(t.TempDir(), q, "python3")
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewWorkspaceModel(ws, q, "true", false, 400, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)

	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.focused = PaneCode
	m.relayout()
	m.code.SetContent(dragContent)

	r := m.layout.RectFor(PaneCode)
	anchorX, anchorY := r.X+1+6, r.Y+1+0
	releaseX, releaseY := r.X+1+4, r.Y+1+2

	upModel, _ := m.Update(press(anchorX, anchorY))
	m = upModel.(*WorkspaceModel)
	upModel, _ = m.Update(motion(releaseX, releaseY))
	m = upModel.(*WorkspaceModel)
	m.showHints = true // simulate `H` opening the hints overlay mid-drag
	upModel, _ = m.Update(release(releaseX, releaseY))
	m = upModel.(*WorkspaceModel)
	m.showHints = false

	want := "world\n" + dragLine1 + "\nthird"
	if got := decodeOSC52(t, tc.flush(t)); got != want {
		t.Fatalf("copied text =\n%q\nwant\n%q", got, want)
	}
	if m.sel.active {
		t.Error("sel.active should be false after release, even though Hints was open when it arrived")
	}
}

// TestWorkspaceDragPastPaneEdgeClampsToEdge asserts dragging the mouse past a
// pane's own border still extends the selection to that edge instead of
// losing the drag or crashing.
func TestWorkspaceDragPastPaneEdgeClampsToEdge(t *testing.T) {
	q, _ := leetcode.Fixture("two-sum")
	ws, err := workspace.Scaffold(t.TempDir(), q, "python3")
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewWorkspaceModel(ws, q, "true", false, 400, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	up, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = up.(*WorkspaceModel)

	tc := newTestClipboard(t)
	m.imgWriter = tc.iw

	m.focused = PaneCode
	m.relayout()
	m.code.SetContent(dragLine0)

	r := m.layout.RectFor(PaneCode)
	upModel, _ := m.Update(press(r.X+1, r.Y+1))
	m = upModel.(*WorkspaceModel)
	// Drag far outside the pane entirely (negative and huge coordinates).
	upModel, _ = m.Update(motion(-100, -100))
	m = upModel.(*WorkspaceModel)
	// The final release's returned model is never read again after this —
	// the assertions below only need tc.flush(t), not m — so it's discarded
	// rather than reassigned into m (staticcheck SA4006: dead store).
	m.Update(release(100000, 100000))

	got := decodeOSC52(t, tc.flush(t))
	if !strings.Contains(got, dragLine0) {
		t.Fatalf("expected the drag clamped to the pane's edges to still cover the line, got %q", got)
	}
}
