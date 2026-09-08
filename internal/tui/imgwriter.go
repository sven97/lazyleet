package tui

import (
	"io"
	"sync"
)

// ImageWriter wraps the terminal output so out-of-band graphics escapes (the
// Kitty transmit + virtual-placement blob) can be injected right before a
// frame, without racing bubbletea's render goroutine. Both the renderer's
// writes and Queue go through the same mutex, so the injected bytes always land
// intact and ahead of the frame that references them.
//
// The transmit escape can't ride inside View() because bubbletea truncates
// frame lines to the terminal width.
type ImageWriter struct {
	w       io.Writer
	mu      sync.Mutex
	pending []byte
}

// NewImageWriter wraps w (normally os.Stdout). Pass it to tea.WithOutput.
func NewImageWriter(w io.Writer) *ImageWriter { return &ImageWriter{w: w} }

// Queue schedules bytes to be written just before the next frame.
func (iw *ImageWriter) Queue(b string) {
	if b == "" {
		return
	}
	iw.mu.Lock()
	iw.pending = append(iw.pending, b...)
	iw.mu.Unlock()
}

func (iw *ImageWriter) Write(p []byte) (int, error) {
	iw.mu.Lock()
	defer iw.mu.Unlock()
	if len(iw.pending) > 0 {
		if _, err := iw.w.Write(iw.pending); err != nil {
			return 0, err
		}
		iw.pending = iw.pending[:0]
	}
	return iw.w.Write(p)
}
