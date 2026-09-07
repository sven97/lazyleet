package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newSyncCmd(app *appContext) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Refresh the local problem and study-plan cache from LeetCode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := app.openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			if !force {
				if fresh, _ := db.ProblemsFresh(ctx, app.cfg.CacheTTL.D()); fresh {
					last, _ := db.ProblemsLastSynced(ctx)
					fmt.Fprintf(out, "problem cache is fresh (synced %s ago); use --force to refresh anyway\n",
						time.Since(last).Round(time.Minute))
					n, err := cacheBundledPlans(ctx, db)
					if err == nil {
						fmt.Fprintf(out, "cached %d bundled study plan(s)\n", n)
					}
					return err
				}
			}

			client, err := app.newClient()
			if err != nil {
				return err
			}

			start := time.Now()
			count, err := fetchAndCacheProblems(ctx, client, db, func(fetched, total int) {
				fmt.Fprintf(out, "\rfetching problem list… %d/%d", fetched, total)
			})
			fmt.Fprintln(out)
			if err != nil {
				return fmt.Errorf("problem list: %w", err)
			}
			fmt.Fprintf(out, "cached %d problems in %s\n", count, time.Since(start).Round(time.Millisecond))

			n, err := cacheBundledPlans(ctx, db)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "cached %d bundled study plan(s)\n", n)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "refresh even if the cache is still fresh")
	return cmd
}
