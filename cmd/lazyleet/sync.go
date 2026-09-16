package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"github.com/sven97/lazyleet/internal/store"
)

func newSyncCmd(app *appContext) *cobra.Command {
	var force, progressOnly, prefetchPlans bool

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

			if progressOnly {
				creds, _ := app.loadCredentials()
				if creds.Anonymous() {
					return fmt.Errorf("not signed in — run `lazyleet auth` first")
				}
				client, err := app.newClient()
				if err != nil {
					return err
				}
				start := time.Now()
				solved, err := fetchAndCacheProgress(ctx, client, db)
				if err != nil {
					return fmt.Errorf("progress: %w", err)
				}
				fmt.Fprintf(out, "updated solve status: %d solved in %s\n",
					solved, time.Since(start).Round(time.Millisecond))
				return nil
			}

			if !force {
				if fresh, _ := db.ProblemsFresh(ctx, app.cfg.CacheTTL.D()); fresh {
					last, _ := db.ProblemsLastSynced(ctx)
					fmt.Fprintf(out, "problem cache is fresh (synced %s ago); use --force to refresh anyway\n",
						time.Since(last).Round(time.Minute))
					return syncPlans(ctx, app, db, out, prefetchPlans, force, app.fetchPlan)
				}
			}

			client, err := app.newClient()
			if err != nil {
				return err
			}

			start := time.Now()
			progress := func(fetched, total int) {
				fmt.Fprintf(out, "\rfetching problem list… %d/%d", fetched, total)
			}

			var (
				count  int
				synced bool
			)
			if force {
				count, err = fetchAndCacheProblems(ctx, client, db, progress)
				synced = true
			} else {
				count, synced, err = syncCatalogIfChanged(ctx, client, db, progress)
			}
			fmt.Fprintln(out)
			if err != nil {
				return fmt.Errorf("problem list: %w", err)
			}
			if synced {
				fmt.Fprintf(out, "cached %d problems in %s\n", count, time.Since(start).Round(time.Millisecond))
			} else {
				fmt.Fprintf(out, "problem catalog already up to date (%d problems); use --force to refresh anyway\n", count)
			}

			return syncPlans(ctx, app, db, out, prefetchPlans, force, app.fetchPlan)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "refresh even if the cache is still fresh")
	cmd.Flags().BoolVar(&progressOnly, "progress", false, "only refresh your solve status (a few requests, not the whole catalog)")
	cmd.Flags().BoolVar(&prefetchPlans, "plans", false, "also prefetch official study plans for offline use")
	cmd.MarkFlagsMutuallyExclusive("progress", "plans")
	return cmd
}

func syncPlans(ctx context.Context, app *appContext, db *store.Store, out io.Writer, official, force bool, fetch planFetcher) error {
	n, err := cacheBundledPlans(ctx, db)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "cached %d bundled study plan(s)\n", n)
	if !official {
		return nil
	}
	var failures []error
	for _, ref := range officialPlans {
		result, err := resolvePlan(ctx, db, ref.slug, app.cfg.CacheTTL.D(), force, fetch)
		if err == nil {
			err = result.refreshErr
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", ref.slug, err))
			continue
		}
		fmt.Fprintf(out, "cached %s (%d problems)\n", result.plan.Name, len(result.plan.Questions))
	}
	return errors.Join(failures...)
}
