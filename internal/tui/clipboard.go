package tui

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// This file backs the clipboard half of F1a: turning a finished selection (or
// the `y` whole-pane fallback) into bytes on the wire. osc52Copy's output is
// handed to the same *ImageWriter both models already hold (wired in via
// EnableImages, ultimately from cmd/lazyleet/browse.go / solve.go's
// tea.WithOutput(iw)) — the existing precedent for injecting raw escape
// sequences into the Bubble Tea output stream without racing the renderer's
// own writes to the terminal (see imgwriter.go's Queue/Write).

// osc52Copy returns the OSC 52 escape sequence asking the terminal to set the
// system clipboard ("c", the default selection) to text — supported locally
// and over SSH by iTerm2, WezTerm, kitty, Windows Terminal, and most others.
// A terminal without support simply ignores an escape sequence it doesn't
// recognize, so this always degrades silently rather than erroring.
func osc52Copy(text string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(text))
	return "\x1b]52;c;" + enc + "\x07"
}

// copiedStatusMsg is the one-line status-bar confirmation shown after a
// successful copy, reusing the same statusMsg field run/submit/sync results
// already report through (see BrowseModel.statusMsg / WorkspaceModel.statusMsg)
// rather than a modal.
func copiedStatusMsg(text string) string {
	n := utf8.RuneCountInString(text)
	if n == 1 {
		return "copied 1 character to clipboard"
	}
	return fmt.Sprintf("copied %d characters to clipboard", n)
}

// copyToClipboard queues text as an OSC 52 copy on iw (a no-op when iw is nil
// or text is empty) and returns the status-bar confirmation to show, or ""
// when nothing was copied. Shared by BrowseModel.copyText and
// WorkspaceModel.copyText, which were previously identical, byte-for-byte
// duplicates of this same three-line body.
func copyToClipboard(iw *ImageWriter, text string) string {
	if text == "" {
		return ""
	}
	if iw != nil {
		iw.Queue(osc52Copy(text))
	}
	return copiedStatusMsg(text)
}

// plainViewportText strips ANSI styling from a rendered viewport.View() and
// trims each line's trailing pad space plus any trailing blank lines — used
// by the `y` fallback, which copies a whole pane's currently visible content
// rather than a drawn selection.
func plainViewportText(view string) string {
	lines := strings.Split(view, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ansi.Strip(ln), " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
