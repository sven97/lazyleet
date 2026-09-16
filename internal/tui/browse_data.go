package tui

import (
	"context"
	"time"
)

// BrowseRow is one problem as shown in the browse list.
type BrowseRow struct {
	FrontendID int
	Slug       string
	Title      string
	Difficulty string // Easy | Medium | Hard
	ACRate     float64
	PaidOnly   bool
	Status     string // "" | ac | notac
	Tags       []string
}

// Solved reports whether the row is marked accepted.
func (r BrowseRow) Solved() bool { return r.Status == "ac" }

// Meta returns the header metadata renderProblemHeader needs for this row.
func (r BrowseRow) Meta() ProblemMeta {
	return ProblemMeta{Difficulty: r.Difficulty, ACRate: r.ACRate, PaidOnly: r.PaidOnly, Tags: r.Tags}
}

// PlanRef identifies a study plan in the sidebar.
type PlanRef struct {
	Slug     string
	Name     string
	Official bool // true = fetched from LeetCode, false = bundled
}

// DailyInfo is today's LeetCode daily challenge plus the signed-in user's
// streak. Streak is 0 when anonymous or unknown; Done covers both "solved
// today" and the streak's current-day-completed flag.
type DailyInfo struct {
	Date       string // YYYY-MM-DD
	FrontendID int    // the number shown to users (e.g. 1 for Two Sum)
	Slug       string
	Title      string
	Difficulty string
	Done       bool
	Streak     int
}

// AuthState is the auth/config summary shown in the Status pane.
type AuthState struct {
	Authed   bool
	User     string // "" unless known
	Region   string
	AuthFile string
	CacheDB  string
}

// BrowseData is everything browse mode needs from the outside world. The cmd
// layer implements it over store + leetcode + plans; tests use a fake.
type BrowseData interface {
	// ListProblems returns the full cached problem list (may be empty).
	ListProblems(ctx context.Context) ([]BrowseRow, error)
	// Auth returns the current auth/config summary (no network).
	Auth(ctx context.Context) AuthState
	// CurrentUser returns the signed-in LeetCode username (one lightweight
	// request). It returns "" when anonymous or the session has expired.
	CurrentUser(ctx context.Context) (string, error)
	// LastSync reports when the problem catalog was last refreshed.
	LastSync(ctx context.Context) (t time.Time, ok bool)
	// ProgressLastSync reports when the signed-in user's solve status was last
	// refreshed (distinct from the full-catalog LastSync).
	ProgressLastSync(ctx context.Context) (t time.Time, ok bool)
	// Sync refreshes the whole problem catalog from LeetCode and returns the
	// problem count.
	Sync(ctx context.Context) (int, error)
	// SyncProgress refreshes only the signed-in user's solve status (a few
	// requests, not the whole catalog) and returns the solved count. It is a
	// no-op returning (0, nil) when not authenticated.
	SyncProgress(ctx context.Context) (int, error)
	// LoadStatement returns a problem's statement as Markdown (cache then API).
	LoadStatement(ctx context.Context, slug string) (string, error)
	// Plans lists the study plans to show in the sidebar.
	Plans(ctx context.Context) ([]PlanRef, error)
	// PlanSlugs returns the ordered problem slugs for a plan.
	PlanSlugs(ctx context.Context, ref PlanRef) ([]string, error)
	// Daily returns today's daily challenge and the user's streak.
	Daily(ctx context.Context) (DailyInfo, error)
	// LoadPosition returns the source and problem selected when browse mode was
	// last closed, so the next launch can resume there. sourceKey is "" when
	// nothing has been remembered yet.
	LoadPosition(ctx context.Context) (sourceKey, slug string)
	// SavePosition remembers the active source and selected problem.
	// Best-effort: called on every settle, so a failure isn't worth surfacing.
	SavePosition(ctx context.Context, sourceKey, slug string) error
}
