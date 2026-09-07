package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sven97/lazyleet/internal/config"
)

// appContext carries process-wide dependencies resolved once in
// PersistentPreRunE and used by subcommands.
type appContext struct {
	paths config.Paths
	cfg   config.Config
}

func newRootCmd() *cobra.Command {
	var (
		configPath string
		verbose    bool
		app        appContext
	)

	root := &cobra.Command{
		Use:   "lazyleet",
		Short: "A lazygit-style terminal UI for LeetCode practice",
		Long: "lazyleet — browse LeetCode problems and study plans, then solve them\n" +
			"in a side-by-side workspace with a local test runner and one-key submit.",
		Version:       versionString(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			app.paths = config.ResolvePaths()
			if configPath != "" {
				app.paths.ConfigFile = configPath
			}
			cfg, err := config.Load(app.paths.ConfigFile)
			if err != nil {
				return err
			}
			app.cfg = cfg
			if verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "config: %s\ndata:   %s\n",
					app.paths.ConfigFile, app.paths.DataDir)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Default action: the browse-mode TUI.
			return runBrowse(&app)
		},
	}

	root.PersistentFlags().StringVar(&configPath, "config", "", "path to config.yml (default: XDG config dir)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "log resolved paths to stderr")

	root.AddCommand(
		newSolveCmd(&app),
		newAuthCmd(&app),
		newSyncCmd(&app),
		newDebugCmd(&app),
	)
	return root
}
