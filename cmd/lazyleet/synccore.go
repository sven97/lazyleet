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
	if err := db.ReplaceProblems(ctx, rows); err != nil {
		return 0, err
	}
	return db.ProblemCount(ctx)
}

// syncCatalogIfChanged avoids the expensive paginated catalog fetch when the
// local cache is already current. It makes one cheap head request (total count
// plus highest frontend id), and only fetches when the catalog actually moved:
// a pure append of new problems is fetched incrementally; anything else
// (removals, renumbering, metadata edits) falls back to a full resync. The bool
// result reports whether any fetch ran. progress is forwarded to
// fetchAndCacheProblems when a full sync is needed.
func syncCatalogIfChanged(ctx context.Context, client *leetcode.Client, db *store.Store, progress func(fetched, total int)) (count int, synced bool, err error) {
	local, err := db.ProblemCount(ctx)
	if err != nil {
		return 0, false, err
	}
	localMax, err := db.MaxFrontendID(ctx)
	if err != nil {
		return 0, false, err
	}
	remoteTotal, remoteMax, err := client.CatalogHead(ctx)
	if err != nil {
		return 0, false, err
	}
	if local == remoteTotal && localMax == remoteMax {
		return local, false, nil
	}

	// More problems than we have and the newest id advanced: only new problems
	// were appended, so fetch just those (1-2 requests instead of ~35).
	if remoteTotal > local && remoteMax > localMax {
		n, err := fetchNewProblems(ctx, client, db, localMax)
		return n, err == nil, err
	}

	// Removals, renumbering, or in-place edits: do a full resync.
	count, err = fetchAndCacheProblems(ctx, client, db, progress)
	return count, true, err
}

// fetchNewProblems fetches only problems newer than maxID by paging the list in
// descending frontend-id order until it reaches an id it already has. New
// problems have no cached solve status to preserve, so rows are inserted with
// the status the server reports (blank when anonymous).
func fetchNewProblems(ctx context.Context, client *leetcode.Client, db *store.Store, maxID int) (int, error) {
	const page = 100
	var rows []store.Problem
	for skip := 0; ; skip += page {
		batch, _, err := client.ListProblems(ctx, leetcode.ProblemFilter{
			OrderBy:   "FRONTEND_ID",
			SortOrder: "DESCENDING",
		}, skip, page)
		if err != nil {
			return 0, err
		}
		done := false
		for _, s := range batch {
			// Non-numeric ids (e.g. "LCP 01") parse to 0 and aren't part of the
			// monotonic numeric-id tracking; skip them rather than mistaking them
			// for the end of the new-problem range.
			if s.FrontendID <= 0 {
				continue
			}
			if s.FrontendID <= maxID {
				done = true
				break
			}
			rows = append(rows, store.Problem{
				FrontendID: s.FrontendID,
				Slug:       s.Slug,
				Title:      s.Title,
				Difficulty: s.Difficulty,
				ACRate:     s.ACRate,
				PaidOnly:   s.PaidOnly,
				Status:     s.Status,
				TopicTags:  s.TopicTags,
			})
		}
		if done || len(batch) < page {
			break
		}
	}
	if len(rows) == 0 {
		return db.ProblemCount(ctx)
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
