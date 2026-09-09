package main

import (
	"context"
	"fmt"
	"time"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/plans"
	"github.com/sven97/lazyleet/internal/store"
	"github.com/sven97/lazyleet/internal/tui"
)

// officialPlans are LeetCode study plans surfaced in the browse sidebar.
var officialPlans = []struct{ slug, name string }{
	{"leetcode-75", "LeetCode 75"},
	{"top-interview-150", "Top Interview 150"},
	{"top-100-liked", "Top 100 Liked"},
}

// browseData adapts store + leetcode + plans to tui.BrowseData. It keeps one
// store handle open for the whole browse session.
type browseData struct {
	app *appContext
	db  *store.Store
}

func newBrowseData(app *appContext) (*browseData, error) {
	db, err := app.openStore()
	if err != nil {
		return nil, err
	}
	return &browseData{app: app, db: db}, nil
}

func (b *browseData) Close() error { return b.db.Close() }

func (b *browseData) Auth(ctx context.Context) tui.AuthState {
	creds, _ := b.app.loadCredentials()
	return tui.AuthState{
		Authed:   !creds.Anonymous(),
		Region:   string(b.app.cfg.Region),
		AuthFile: b.app.paths.AuthFile,
		CacheDB:  b.app.paths.DatabaseFile,
	}
}

func (b *browseData) ListProblems(ctx context.Context) ([]tui.BrowseRow, error) {
	rows, err := b.db.ListProblems(ctx, store.ProblemFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]tui.BrowseRow, len(rows))
	for i, r := range rows {
		out[i] = tui.BrowseRow{
			FrontendID: r.FrontendID,
			Slug:       r.Slug,
			Title:      r.Title,
			Difficulty: r.Difficulty,
			ACRate:     r.ACRate,
			PaidOnly:   r.PaidOnly,
			Status:     r.Status,
			Tags:       r.TopicTags,
		}
	}
	return out, nil
}

func (b *browseData) LastSync(ctx context.Context) (time.Time, bool) {
	t, err := b.db.ProblemsLastSynced(ctx)
	if err != nil || t.IsZero() {
		return time.Time{}, false
	}
	return t, true
}

func (b *browseData) Sync(ctx context.Context) (int, error) {
	client, err := b.app.newClient()
	if err != nil {
		return 0, err
	}
	n, err := fetchAndCacheProblems(ctx, client, b.db, nil)
	if err != nil {
		return 0, err
	}
	_, _ = cacheBundledPlans(ctx, b.db)
	return n, nil
}

func (b *browseData) SyncProgress(ctx context.Context) (int, error) {
	creds, _ := b.app.loadCredentials()
	if creds.Anonymous() {
		return 0, nil
	}
	client, err := b.app.newClient()
	if err != nil {
		return 0, err
	}
	return fetchAndCacheProgress(ctx, client, b.db)
}

func (b *browseData) LoadStatement(ctx context.Context, slug string) (string, error) {
	if q, ok := leetcode.Fixture(slug); ok {
		return q.Statement, nil
	}
	if d, err := b.db.GetProblemDetail(ctx, slug, b.app.cfg.CacheTTL.D()); err == nil {
		return d.StatementMD, nil
	}
	client, err := b.app.newClient()
	if err != nil {
		return "", err
	}
	q, err := client.QuestionDetail(ctx, slug)
	if err != nil {
		return "", err
	}
	_ = b.db.PutProblemDetail(ctx, cacheFromQuestion(q))
	return q.Statement, nil
}

func (b *browseData) Plans(ctx context.Context) ([]tui.PlanRef, error) {
	var out []tui.PlanRef
	for _, p := range plans.All() {
		out = append(out, tui.PlanRef{Slug: p.Slug, Name: p.Name, Official: false})
	}
	for _, o := range officialPlans {
		out = append(out, tui.PlanRef{Slug: o.slug, Name: o.name, Official: true})
	}
	return out, nil
}

func (b *browseData) PlanSlugs(ctx context.Context, ref tui.PlanRef) ([]string, error) {
	if !ref.Official {
		if bp, ok := plans.Get(ref.Slug); ok {
			return bp.Problems, nil
		}
		return nil, fmt.Errorf("unknown bundled plan %q", ref.Slug)
	}
	if sp, err := b.db.GetStudyPlan(ctx, ref.Slug); err == nil && len(sp.Problems) > 0 {
		return sp.Problems, nil
	}
	client, err := b.app.newClient()
	if err != nil {
		return nil, err
	}
	plan, err := client.StudyPlanDetail(ctx, ref.Slug)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, len(plan.Questions))
	for i, q := range plan.Questions {
		slugs[i] = q.Slug
	}
	_ = b.db.PutStudyPlan(ctx, store.StudyPlan{
		Slug: ref.Slug, Name: plan.Name, Source: "leetcode", Problems: slugs,
	})
	return slugs, nil
}
