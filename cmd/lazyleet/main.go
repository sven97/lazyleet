// Command lazyleet is a lazygit-style terminal UI for practising on LeetCode:
// browse problems and study plans, then solve them in a side-by-side workspace
// with a local test runner and one-key submit.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
