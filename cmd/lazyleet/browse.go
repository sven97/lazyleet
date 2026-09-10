package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/termimg"
	"github.com/sven97/lazyleet/internal/tui"
)

// runBrowse is the default action: loop the browse-mode TUI, launching the
// workspace whenever the user picks a problem and returning to browse when they
// leave it.
func runBrowse(app *appContext) error {
	data, err := newBrowseData(app)
	if err != nil {
		return err
	}
	defer data.Close()

	for {
		iw := tui.NewImageWriter(os.Stdout)
		bm := tui.NewBrowseModel(data)
		bm.EnableImages(termimg.Detect(app.cfg.Images), app.paths.ImageCacheDir, iw)
		final, err := tea.NewProgram(
			bm,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
			tea.WithFPS(uiFPS),
			tea.WithFilter(tui.WheelEdgeFilter),
			tea.WithOutput(iw),
		).Run()
		if err != nil {
			return err
		}

		bm, ok := final.(*tui.BrowseModel)
		if !ok || bm.Chosen == "" {
			return nil // user quit
		}

		if err := app.openWorkspace(context.Background(), bm.Chosen, "", false); err != nil {
			fmt.Fprintln(os.Stderr, "workspace:", err)
			time.Sleep(1500 * time.Millisecond)
		}
	}
}
