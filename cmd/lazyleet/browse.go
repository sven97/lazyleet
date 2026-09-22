package main

import (
	"context"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/selfupdate"
	"github.com/sven97/lazyleet/internal/termimg"
	"github.com/sven97/lazyleet/internal/tui"
)

// runBrowse is the default action: one Bubble Tea program that switches between
// browse and workspace so filters and cursor are preserved on return. If the
// user installed an update from inside the TUI, the new binary is exec'd in
// place once the program (and its database handle) has shut down.
func runBrowse(app *appContext) error {
	updates := newUpdateService(app)
	restart, err := runBrowseTUI(app, updates)
	if err != nil || !restart {
		return err
	}
	return selfupdate.Restart(updates.install.Launch)
}

func runBrowseTUI(app *appContext, updates *updateService) (restart bool, err error) {
	data, err := newBrowseData(app)
	if err != nil {
		return false, err
	}
	defer data.Close()

	bm := tui.NewBrowseModel(data)
	bm.SetVersion(versionString())
	bm.SetUpdater(updates)
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
	return bm.RestartRequested(), err
}
