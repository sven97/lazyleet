package tui

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// debugScroll turns on verbose scroll/render tracing to the tea.LogToFile sink
// when LAZYLEET_DEBUG names a log file. Diagnostic only.
var debugScroll = os.Getenv("LAZYLEET_DEBUG") != ""

func scrollLogf(format string, args ...any) {
	if debugScroll {
		log.Printf("[scroll] "+format, args...)
	}
}

// wheelGuard is implemented by models that can say a mouse-wheel event would do
// nothing because the pane under the cursor is already at the edge it is trying
// to scroll past.
type wheelGuard interface {
	wheelAtEdge(tea.MouseMsg) bool
}

// WheelEdgeFilter is a tea.WithFilter that swallows mouse-wheel events which
// would be no-ops at a scroll boundary. Letting them through triggers an
// Update/View/flush cycle that renders an identical frame; over Kitty image
// placeholders (and against Ghostty's own overscroll) that repaint flickers,
// and a fast trackpad flick into the top or bottom produces a burst of them.
// Dropping the event before Update skips the cycle entirely.
func WheelEdgeFilter(model tea.Model, msg tea.Msg) tea.Msg {
	mm, ok := msg.(tea.MouseMsg)
	if !ok || !tea.MouseEvent(mm).IsWheel() {
		return msg
	}
	if g, ok := model.(wheelGuard); ok && g.wheelAtEdge(mm) {
		scrollLogf("filter DROP wheel btn=%v x=%d y=%d", mm.Button, mm.X, mm.Y)
		return nil
	}
	scrollLogf("filter PASS wheel btn=%v x=%d y=%d", mm.Button, mm.X, mm.Y)
	return msg
}

// wheelScrollLines is how many lines one mouse-wheel event nudges a list or
// viewport.
//
// A macOS trackpad flick (and Ghostty's translation of smooth scrolling into
// discrete wheel events) arrives as a rapid burst of these events. Multiplying
// each one by a large step makes the content lurch well past where the gesture
// meant to stop, which is what reads as "not smooth" next to the terminal's own
// scrollback. A small fixed step lets the burst accumulate naturally and keeps
// a notched mouse wheel usable.
const wheelScrollLines = 2
