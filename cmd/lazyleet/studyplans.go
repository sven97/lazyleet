package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/plans"
	"github.com/sven97/lazyleet/internal/store"
)

type planFetcher func(context.Context, string) (leetcode.StudyPlan, error)

type resolvedPlan struct {
	plan leetcode.StudyPlan
	// refreshErr is non-nil when a stale copy kept the plan usable offline.
	refreshErr error
}

func (a *appContext) fetchPlan(ctx context.Context, slug string) (leetcode.StudyPlan, error) {
	client, err := a.newClient()
	if err != nil {
		return leetcode.StudyPlan{}, err
	}
	return client.StudyPlanDetail(ctx, slug)
}

// resolvePlan keeps bundled resolution entirely local, refreshes expired
// official plans, and preserves a usable stale copy on refresh failure.
func resolvePlan(ctx context.Context, db *store.Store, slug string, ttl time.Duration, force bool, fetch planFetcher) (resolvedPlan, error) {
	if b, ok := plans.Get(slug); ok {
		p, err := planFromCache(ctx, db, store.StudyPlan{Slug: b.Slug, Name: b.Name, Source: "bundled", Problems: b.Problems})
		return resolvedPlan{plan: p}, err
	}
	if !force {
		cached, err := db.GetStudyPlan(ctx, slug, ttl)
		if err != nil && !errors.Is(err, store.ErrNotCached) {
			return resolvedPlan{}, err
		}
		if err == nil && len(cached.Problems) > 0 {
			p, err := planFromCache(ctx, db, cached)
			return resolvedPlan{plan: p}, err
		}
	}
	p, err := fetch(ctx, slug)
	if err == nil && (p.Slug != slug || len(p.Questions) == 0) {
		err = fmt.Errorf("empty or mismatched study plan %q", slug)
	}
	if err != nil {
		// ttl=0 disables the staleness check, so this recovers any cached
		// copy regardless of age for a usable offline fallback.
		old, cacheErr := db.GetStudyPlan(ctx, slug, 0)
		if cacheErr != nil && !errors.Is(cacheErr, store.ErrNotCached) {
			return resolvedPlan{}, cacheErr
		}
		if cacheErr == nil && len(old.Problems) > 0 {
			oldPlan, convErr := planFromCache(ctx, db, old)
			return resolvedPlan{plan: oldPlan, refreshErr: err}, convErr
		}
		return resolvedPlan{}, err
	}
	questions, err := json.Marshal(p.Questions)
	if err != nil {
		return resolvedPlan{}, err
	}
	slugs := make([]string, len(p.Questions))
	for i, q := range p.Questions {
		slugs[i] = q.Slug
	}
	err = db.PutStudyPlan(ctx, store.StudyPlan{Slug: slug, Name: p.Name, Source: "leetcode", Problems: slugs, QuestionsJSON: string(questions)})
	return resolvedPlan{plan: p}, err
}

func planFromCache(ctx context.Context, db *store.Store, cached store.StudyPlan) (leetcode.StudyPlan, error) {
	p := leetcode.StudyPlan{Slug: cached.Slug, Name: cached.Name, Source: cached.Source}
	if cached.QuestionsJSON != "" && cached.QuestionsJSON != "[]" {
		if err := json.Unmarshal([]byte(cached.QuestionsJSON), &p.Questions); err != nil {
			return p, err
		}
		return p, nil
	}
	// Bundled plans and caches from older versions only contain ordered slugs.
	rows, err := db.ListProblems(ctx, store.ProblemFilter{})
	if err != nil {
		return p, err
	}
	bySlug := make(map[string]store.Problem, len(rows))
	for _, r := range rows {
		bySlug[r.Slug] = r
	}
	for _, slug := range cached.Problems {
		r := bySlug[slug]
		q := leetcode.StudyPlanQuestion{Slug: slug, Title: r.Title, FrontendID: r.FrontendID, Difficulty: r.Difficulty}
		if fixture, ok := leetcode.Fixture(slug); ok && q.Title == "" {
			q.Title, q.FrontendID, q.Difficulty = fixture.Title, fixture.FrontendID, fixture.Difficulty
		}
		p.Questions = append(p.Questions, q)
	}
	return p, nil
}
