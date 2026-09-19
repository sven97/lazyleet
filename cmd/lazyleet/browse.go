package main

import (
	"context"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/termimg"
	"github.com/sven97/lazyleet/internal/tui"
)

// runBrowse is the default action: one Bubble Tea program that switches between
// browse and workspace so filters and cursor are preserved on return.
func runBrowse(app *appContext) error {
	data, err := newBrowseData(app)
	if err != nil {
		return err
	}
	defer data.Close()

	bm := tui.NewBrowseModel(data)
	bm.SetVersion(versionString())
	factory := func(slug string) (*tui.WorkspaceModel, error) {
		return app.buildWorkspaceModel(context.Background(), slug, "", false)
	}
	am := tui.NewAppModel(bm, factory)

	iw := tui.NewImageWriter(os.Stdout)
	am.EnableImages(termimg.Detect(app.cfg.Images), app.paths.ImageCacheDir, iw)

	_, err = tea.NewProgram(
		am,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithFPS(uiFPS),
		tea.WithFilter(tui.WheelEdgeFilter),
		tea.WithOutput(iw),
	).Run()
	return err
}
