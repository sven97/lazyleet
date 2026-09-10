// Command lazyleet is a lazygit-style terminal UI for practising on LeetCode:
// browse problems and study plans, then solve them in a side-by-side workspace
// with a local test runner and one-key submit.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// LAZYLEET_DEBUG=/path/to/log turns on verbose tracing (scroll/render
	// diagnostics live behind the same switch in internal/tui).
	if p := os.Getenv("LAZYLEET_DEBUG"); p != "" {
		if f, err := tea.LogToFile(p, "lazyleet"); err == nil {
			defer f.Close()
		}
	}

	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
