package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
	"github.com/sven97/lazyleet/internal/termimg"
)

func newDebugCmd(app *appContext) *cobra.Command {
	debug := &cobra.Command{
		Use:    "debug",
		Short:  "Introspection helpers for development",
		Hidden: true,
	}

	debug.AddCommand(&cobra.Command{
		Use:   "paths",
		Short: "Print every resolved filesystem location",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p := app.paths
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "config dir     %s\n", p.ConfigDir)
			fmt.Fprintf(out, "config file    %s\n", p.ConfigFile)
			fmt.Fprintf(out, "data dir       %s\n", p.DataDir)
			fmt.Fprintf(out, "database       %s\n", p.DatabaseFile)
			fmt.Fprintf(out, "auth file      %s\n", p.AuthFile)
			fmt.Fprintf(out, "workspace root %s\n", p.WorkspaceRoot)
			return nil
		},
	})

	debug.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "Print the effective configuration as YAML",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := yaml.Marshal(app.cfg)
			if err != nil {
				return err
			}
			cmd.OutOrStdout().Write(out)
			return nil
		},
	})

	var listLimit int
	var listRemote bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Print cached problems (or --remote to hit LeetCode directly)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			defer tw.Flush()

			if listRemote {
				client, err := app.newClient()
				if err != nil {
					return err
				}
				ps, total, err := client.ListProblems(cmd.Context(), leetcode.ProblemFilter{}, 0, listLimit)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "%d of %d problems (live)\n", len(ps), total)
				for _, p := range ps {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%.1f%%\t%s\n", p.FrontendID, dashIf(p.Status), p.Difficulty, p.ACRate, p.Title)
				}
				return nil
			}

			db, err := app.openStore()
			if err != nil {
				return err
			}
			defer db.Close()
			rows, err := db.ListProblems(cmd.Context(), store.ProblemFilter{Limit: listLimit})
			if err != nil {
				return err
			}
			n, _ := db.ProblemCount(cmd.Context())
			if n == 0 {
				return fmt.Errorf("problem cache is empty — run `lazyleet sync` first")
			}
			fmt.Fprintf(out, "%d of %d cached problems\n", len(rows), n)
			for _, p := range rows {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%.1f%%\t%s\n", p.FrontendID, dashIf(p.Status), p.Difficulty, p.ACRate, p.Title)
			}
			return nil
		},
	}
	listCmd.Flags().IntVar(&listLimit, "limit", 30, "max rows")
	listCmd.Flags().BoolVar(&listRemote, "remote", false, "bypass the cache and query LeetCode")
	debug.AddCommand(listCmd)

	debug.AddCommand(&cobra.Command{
		Use:   "problem <slug>",
		Short: "Fetch and print one problem's detail from LeetCode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.newClient()
			if err != nil {
				return err
			}
			q, err := client.QuestionDetail(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "#%d  %s  [%s]  qid=%d\n", q.FrontendID, q.Title, q.Difficulty, q.QuestionID)
			fmt.Fprintf(out, "entry: %s(", q.Meta.Name)
			for i, p := range q.Meta.Params {
				if i > 0 {
					fmt.Fprint(out, ", ")
				}
				fmt.Fprintf(out, "%s %s", p.Name, p.Type)
			}
			fmt.Fprintf(out, ") -> %s\n", q.Meta.Return.Type)
			fmt.Fprintf(out, "languages: %v\n", keysOf(q.CodeSnippets))
			fmt.Fprintf(out, "example cases: %d\n\n", len(q.ExampleCases))
			fmt.Fprintln(out, truncateStr(q.Statement, 1200))
			return nil
		},
	})

	var imgCols int
	imgCmd := &cobra.Command{
		Use:   "img <url>",
		Short: "Fetch an image and print it to the terminal (test image rendering)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := termimg.Fetch(cmd.Context(), app.paths.ImageCacheDir, args[0])
			if err != nil {
				return err
			}
			im, err := termimg.Decode(data)
			if err != nil {
				return err
			}
			proto := termimg.Detect(app.cfg.Images)
			body, rows := im.Render(proto, imgCols)
			fmt.Fprintf(cmd.ErrOrStderr(), "protocol=%s  %d rows\n", proto, rows)
			fmt.Fprintln(cmd.OutOrStdout(), body)
			return nil
		},
	}
	imgCmd.Flags().IntVar(&imgCols, "cols", 60, "target width in cells")
	debug.AddCommand(imgCmd)

	debug.AddCommand(&cobra.Command{
		Use:   "plan <slug>",
		Short: "Fetch and print a study plan (bundled or official)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			client, err := app.newClient()
			if err != nil {
				return err
			}
			plan, err := client.StudyPlanDetail(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s (%s) — %d problems\n", plan.Name, plan.Slug, len(plan.Questions))
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			defer tw.Flush()
			for _, q := range plan.Questions {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", q.FrontendID, q.Difficulty, q.Slug, q.Group)
			}
			return nil
		},
	})

	return debug
}

func dashIf(s string) string {
	switch s {
	case "ac":
		return "✓"
	case "notac":
		return "~"
	default:
		return "-"
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…(truncated)"
}
