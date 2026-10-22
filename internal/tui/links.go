package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// This file makes the OSC 8 hyperlinks the panes emit (see hyperlink in
// render.go) clickable from inside the app. The program runs with mouse
// capture on, so a terminal sees none of the clicks and can't open its own
// OSC 8 links — and terminals that don't implement OSC 8 never could. A plain
// click (press and release on one cell, see selectionState.end) therefore
// resolves the link under the cursor itself and hands it to the OS opener.

const osc8Prefix = "\x1b]8;"

// linkAtCell returns the OSC 8 link whose display text covers terminal column
// col of line, a rendered row such as one line of a viewport's View(). Escape
// sequences are zero-width, so a link's span is measured in visible cells. A
// link still open at the end of the line covers the rest of it.
func linkAtCell(line string, col int) (target string, ok bool) {
	var (
		open  string // target of the link currently open on this line, if any
		start int    // visible column where that link's display text begins
		pos   int    // byte offset scanned so far
	)
	for {
		i := strings.Index(line[pos:], osc8Prefix)
		if i < 0 {
			break
		}
		i += pos
		body := line[i+len(osc8Prefix):]
		end, stLen := indexST(body)
		if end < 0 {
			break // unterminated sequence: nothing sensible to read past it
		}
		pos = i + len(osc8Prefix) + end + stLen

		// body is "params;URI"; an empty URI closes the current link.
		_, uri, found := strings.Cut(body[:end], ";")
		col0 := ansi.StringWidth(line[:i])
		switch {
		case found && uri != "":
			open, start = uri, col0
		case open != "":
			if col >= start && col < col0 {
				return open, true
			}
			open = ""
		}
	}
	if open != "" && col >= start && col < ansi.StringWidth(line) {
		return open, true
	}
	return "", false
}

// indexST returns the offset of the first string terminator in s (ESC \ or
// BEL) and its length, or -1 if there is none.
func indexST(s string) (idx, n int) {
	esc, bel := strings.Index(s, "\x1b\\"), strings.IndexByte(s, '\a')
	switch {
	case esc >= 0 && (bel < 0 || esc < bel):
		return esc, 2
	case bel >= 0:
		return bel, 1
	}
	return -1, 0
}

// openURL launches the OS's default handler for target. A variable so tests
// can observe a click without spawning a browser.
var openURL = func(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap the opener; its exit status is nobody's business
	return nil
}

// openLinkAt opens the hyperlink under the pane-local cell (lx, ly) of view (a
// viewport's View() output), for a plain click on that pane. It returns the
// line to show in the status bar, or "" when no link is there — a click on
// plain text just focuses the pane as before.
func openLinkAt(view string, lx, ly int) string {
	lines := strings.Split(view, "\n")
	if ly < 0 || ly >= len(lines) {
		return ""
	}
	target, ok := linkAtCell(lines[ly], lx)
	if !ok {
		return ""
	}
	if err := openLink(target); err != nil {
		return "couldn't open link: " + err.Error()
	}
	return "opened " + target
}

// openLink opens target in the default browser. Only http(s) is allowed: the
// Detail pane also shows links lifted from LeetCode's problem statements, and
// a click must never hand a file:, javascript: or custom-scheme URL to the OS.
func openLink(target string) error {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("not an http(s) link: %q", target)
	}
	return openURL(target)
}
