package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/plans"
	"github.com/sven97/lazyleet/internal/store"
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
					return syncBundledPlans(cmd, db)
				}
			}

			client, err := app.newClient()
			if err != nil {
				return err
			}

			fmt.Fprint(out, "fetching problem list… ")
			start := time.Now()
			summaries, err := client.ListAllProblems(ctx, leetcode.ProblemFilter{}, func(fetched, total int) {
				fmt.Fprintf(out, "\rfetching problem list… %d/%d", fetched, total)
			})
			if err != nil {
				fmt.Fprintln(out)
				return fmt.Errorf("problem list: %w", err)
			}
			fmt.Fprintf(out, "\rfetching problem list… %d problems in %s\n", len(summaries), time.Since(start).Round(time.Millisecond))

			rows := make([]store.Problem, len(summaries))
			for i, s := range summaries {
				rows[i] = store.Problem{
					FrontendID: s.FrontendID,
					Slug:       s.Slug,
					Title:      s.Title,
					Difficulty: s.Difficulty,
					ACRate:     s.ACRate,
					PaidOnly:   s.PaidOnly,
					Status:     s.Status,
					TopicTags:  s.TopicTags,
				}
			}
			if err := db.UpsertProblems(ctx, rows); err != nil {
				return err
			}
			n, _ := db.ProblemCount(ctx)
			fmt.Fprintf(out, "cached %d problems\n", n)

			return syncBundledPlans(cmd, db)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "refresh even if the cache is still fresh")
	return cmd
}

func syncBundledPlans(cmd *cobra.Command, db *store.Store) error {
	ctx := cmd.Context()
	for _, b := range plans.All() {
		if err := db.PutStudyPlan(ctx, store.StudyPlan{
			Slug:     b.Slug,
			Name:     b.Name,
			Source:   "bundled",
			Problems: b.Problems,
		}); err != nil {
			return err
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "cached %d bundled study plan(s)\n", len(plans.All()))
	return nil
}
