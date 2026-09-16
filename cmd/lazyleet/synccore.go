package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/plans"
	"github.com/sven97/lazyleet/internal/store"
)

// fetchAndCacheProblems pulls the full problem list from LeetCode and upserts it
// into the store. progress, if non-nil, is called after each page.
//
// The catalog fetch pages through the whole problem set (tens of requests,
// tens of seconds), so a session can expire mid-fetch even though it looked
// valid at the start. To avoid that silently wiping cached solve status, the
// per-row Status is only trusted when the session is confirmed live both
// before and after the fetch; otherwise the catalog metadata (title,
// difficulty, tags, ...) still updates but each row keeps its previously
// cached status rather than being overwritten with a stale/anonymous one.
// A merely-stale session therefore degrades the status refresh instead of
// failing the whole catalog sync.
func fetchAndCacheProblems(ctx context.Context, client *leetcode.Client, db *store.Store, progress func(fetched, total int)) (int, error) {
	trustStatus := true
	if client.Authenticated() {
		trustStatus = requireSession(ctx, client) == nil
	}
	summaries, err := client.ListAllProblems(ctx, leetcode.ProblemFilter{}, progress)
	if err != nil {
		return 0, err
	}
	if trustStatus && client.Authenticated() {
		trustStatus = requireSession(ctx, client) == nil
	}

	var existingStatus map[string]string
	if !trustStatus {
		existing, err := db.ListProblems(ctx, store.ProblemFilter{})
		if err != nil {
			return 0, err
		}
		existingStatus = make(map[string]string, len(existing))
		for _, p := range existing {
			existingStatus[p.Slug] = p.Status
		}
	}

	rows := make([]store.Problem, len(summaries))
	for i, s := range summaries {
		status := s.Status
		if !trustStatus {
			status = existingStatus[s.Slug]
		}
		rows[i] = store.Problem{
			FrontendID: s.FrontendID,
			Slug:       s.Slug,
			Title:      s.Title,
			Difficulty: s.Difficulty,
			ACRate:     s.ACRate,
			PaidOnly:   s.PaidOnly,
			Status:     status,
			TopicTags:  s.TopicTags,
		}
	}
	if err := db.UpsertProblems(ctx, rows); err != nil {
		return 0, err
	}
	return db.ProblemCount(ctx)
}

// syncCatalogIfChanged avoids the expensive paginated catalog fetch when the
// server's problem count already matches the local cache. It makes one cheap
// count-only request, compares it with the cached row count, and only performs
// the full sync when the totals differ (new/removed problems). The bool result
// reports whether a full fetch actually ran. progress is forwarded to
// fetchAndCacheProblems when a full sync is needed.
func syncCatalogIfChanged(ctx context.Context, client *leetcode.Client, db *store.Store, progress func(fetched, total int)) (count int, synced bool, err error) {
	local, err := db.ProblemCount(ctx)
	if err != nil {
		return 0, false, err
	}
	remote, err := client.TotalProblems(ctx, leetcode.ProblemFilter{})
	if err != nil {
		return 0, false, err
	}
	if local == remote {
		return local, false, nil
	}
	count, err = fetchAndCacheProblems(ctx, client, db, progress)
	return count, true, err
}

// fetchAndCacheProgress refreshes only the caller's solve status by pulling the
// server-filtered "accepted" and "attempted" problem lists (a few pages each,
// vs. the whole catalog) and rewriting the status column. Returns the number of
// solved problems. Requires an authenticated client.
func fetchAndCacheProgress(ctx context.Context, client *leetcode.Client, db *store.Store) (int, error) {
	if !client.Authenticated() {
		return 0, fmt.Errorf("not signed in — run `lazyleet auth` first")
	}
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
	// Fetching two paginated lists can take long enough for the session to
	// expire mid-flight; re-check before trusting the result to replace
	// cached progress.
	if err := requireSession(ctx, client); err != nil {
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

// requireSession wraps leetcode.Client.VerifySession with this CLI's
// error message. Check before replacing cached statuses: an expired cookie
// can otherwise return an anonymous, empty progress list and erase the
// user's cached progress.
func requireSession(ctx context.Context, client *leetcode.Client) error {
	if err := client.VerifySession(ctx); err != nil {
		if errors.Is(err, leetcode.ErrSessionExpired) {
			return fmt.Errorf("session expired — run `lazyleet auth`, then retry")
		}
		return err
	}
	return nil
}
