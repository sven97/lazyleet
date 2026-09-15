package main

import (
	"context"
	"fmt"
	"time"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/plans"
	"github.com/sven97/lazyleet/internal/store"
)

// fetchAndCacheProblems pulls the full problem list from LeetCode and upserts it
// into the store. progress, if non-nil, is called after each page.
func fetchAndCacheProblems(ctx context.Context, client *leetcode.Client, db *store.Store, progress func(fetched, total int)) (int, error) {
	if client.Authenticated() {
		if err := requireSession(ctx, client); err != nil {
			return 0, err
		}
	}
	summaries, err := client.ListAllProblems(ctx, leetcode.ProblemFilter{}, progress)
	if err != nil {
		return 0, err
	}
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
		return 0, err
	}
	return db.ProblemCount(ctx)
}

// fetchAndCacheProgress refreshes only the caller's solve status by pulling the
// server-filtered "accepted" and "attempted" problem lists (a few pages each,
// vs. the whole catalog) and rewriting the status column. Returns the number of
// solved problems. Requires an authenticated client.
func fetchAndCacheProgress(ctx context.Context, client *leetcode.Client, db *store.Store) (int, error) {
	if err := requireSession(ctx, client); err != nil {
		return 0, err
	}
	ac, err := client.ListAllProblems(ctx, leetcode.ProblemFilter{Status: "AC"}, nil)
	if err != nil {
		return 0, err
	}
	tried, err := client.ListAllProblems(ctx, leetcode.ProblemFilter{Status: "TRIED"}, nil)
	if err != nil {
		return 0, err
	}
	acSlugs := make([]string, len(ac))
	for i, p := range ac {
		acSlugs[i] = p.Slug
	}
	triedSlugs := make([]string, len(tried))
	for i, p := range tried {
		triedSlugs[i] = p.Slug
	}
	n, err := db.ReplaceProblemStatuses(ctx, acSlugs, triedSlugs)
	if err != nil {
		return n, err
	}
	_ = db.SetMetaTime(ctx, store.MetaProgressSyncedAt, time.Now())
	return n, nil
}

// cacheBundledPlans writes every compiled-in study plan into the store.
func cacheBundledPlans(ctx context.Context, db *store.Store) (int, error) {
	all := plans.All()
	for _, b := range all {
		if err := db.PutStudyPlan(ctx, store.StudyPlan{
			Slug:     b.Slug,
			Name:     b.Name,
			Source:   "bundled",
			Problems: b.Problems,
		}); err != nil {
			return 0, err
		}
	}
	return len(all), nil
}

// Check before replacing cached statuses: an expired cookie can otherwise
// return an anonymous, empty progress list and erase the user's cached progress.
func requireSession(ctx context.Context, client *leetcode.Client) error {
	user, err := client.WhoAmI(ctx)
	if err != nil {
		return fmt.Errorf("could not verify session: %w", err)
	}
	if user == "" {
		return fmt.Errorf("session expired — run `lazyleet auth`, then retry")
	}
	return nil
}
