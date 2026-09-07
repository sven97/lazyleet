package main

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/tui"
	"github.com/sven97/lazyleet/internal/workspace"
)

func newSolveCmd(app *appContext) *cobra.Command {
	var lang string

	cmd := &cobra.Command{
		Use:   "solve <slug>",
		Short: "Open the coding workspace for a problem (Tier C)",
		Long: "Scaffold a workspace directory for the problem and open the side-by-side\n" +
			"coding view: statement, a live mirror of your solution file, and local\n" +
			"test results. Edit with `e` (your $EDITOR); saves auto-run.\n\n" +
			"Until the LeetCode API lands (Phase 1) only bundled fixtures work.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			q, ok := leetcode.Fixture(slug)
			if !ok {
				avail := leetcode.FixtureSlugs()
				sort.Strings(avail)
				return fmt.Errorf("no bundled fixture for %q (LeetCode API lands in Phase 1)\navailable: %s",
					slug, strings.Join(avail, ", "))
			}

			if err := app.paths.EnsureDirs(); err != nil {
				return err
			}
			if lang == "" {
				lang = app.cfg.DefaultLanguage
			}

			ws, err := workspace.Scaffold(app.paths.WorkspaceRoot, q, lang)
			if err != nil {
				return err
			}

			m, err := tui.NewWorkspaceModel(
				ws, q,
				app.cfg.ResolveEditor(),
				app.cfg.Workspace.RunOnSave,
				app.cfg.Workspace.RunDebounceMs,
			)
			if err != nil {
				return err
			}

			p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
			_, err = p.Run()
			return err
		},
	}

	cmd.Flags().StringVar(&lang, "lang", "", "language slug (default: config default_language)")
	return cmd
}
