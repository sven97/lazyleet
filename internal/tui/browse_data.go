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

// PlanRef identifies a study plan in the sidebar.
type PlanRef struct {
	Slug     string
	Name     string
	Official bool // true = fetched from LeetCode, false = bundled
}

// BrowseData is everything browse mode needs from the outside world. The cmd
// layer implements it over store + leetcode + plans; tests use a fake.
type BrowseData interface {
	// ListProblems returns the full cached problem list (may be empty).
	ListProblems(ctx context.Context) ([]BrowseRow, error)
	// LastSync reports when the problem cache was last refreshed.
	LastSync(ctx context.Context) (t time.Time, ok bool)
	// Sync refreshes the cache from LeetCode and returns the problem count.
	Sync(ctx context.Context) (int, error)
	// LoadStatement returns a problem's statement as Markdown (cache then API).
	LoadStatement(ctx context.Context, slug string) (string, error)
	// Plans lists the study plans to show in the sidebar.
	Plans(ctx context.Context) ([]PlanRef, error)
	// PlanSlugs returns the ordered problem slugs for a plan.
	PlanSlugs(ctx context.Context, ref PlanRef) ([]string, error)
}
