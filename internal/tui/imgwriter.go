package tui

import (
	"os"
	"sync"
)

// ImageWriter wraps the terminal file so out-of-band graphics escapes (the
// Kitty transmit + virtual-placement blob) can be injected right before a
// frame, without racing bubbletea's render goroutine: both the renderer's
// writes and Queue go through the same mutex, so the injected bytes always land
// intact and ahead of the frame that references them.
//
// The transmit escape can't ride inside View() because bubbletea truncates
// frame lines to the terminal width. It implements enough of the terminal-file
// interface (Read/Write/Close/Fd) that bubbletea still detects a TTY and emits
// WindowSizeMsg.
type ImageWriter struct {
	f       *os.File
	mu      sync.Mutex
	pending []byte
}

// NewImageWriter wraps f (normally os.Stdout). Pass it to tea.WithOutput.
func NewImageWriter(f *os.File) *ImageWriter { return &ImageWriter{f: f} }

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
		if _, err := iw.f.Write(iw.pending); err != nil {
			return 0, err
		}
		iw.pending = iw.pending[:0]
	}
	return iw.f.Write(p)
}

// Read / Close / Fd delegate to the underlying file so bubbletea recognises a
// terminal. Close is a no-op — closing os.Stdout process-wide would be wrong.
func (iw *ImageWriter) Read(p []byte) (int, error) { return iw.f.Read(p) }
func (iw *ImageWriter) Close() error               { return nil }
func (iw *ImageWriter) Fd() uintptr                { return iw.f.Fd() }
