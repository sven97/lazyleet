package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sven97/lazyleet/internal/config"
	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
)

func planTestDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestPlanCacheLifecycle(t *testing.T) {
	ctx := context.Background()
	db := planTestDB(t)
	calls := 0
	fetch := func(_ context.Context, slug string) (leetcode.StudyPlan, error) {
		calls++
		return leetcode.StudyPlan{Slug: slug, Name: "Plan", Source: "leetcode", Questions: []leetcode.StudyPlanQuestion{{Slug: "two-sum", FrontendID: 1, Title: "Two Sum", Difficulty: "Easy", Group: "Arrays"}}}, nil
	}
	result, err := resolvePlan(ctx, db, "leetcode-75", time.Hour, false, fetch)
	if err != nil || len(result.plan.Questions) != 1 {
		t.Fatalf("initial fetch: %+v %v", result, err)
	}
	result, err = resolvePlan(ctx, db, "leetcode-75", time.Hour, false, fetch)
	if err != nil || calls != 1 || result.plan.Questions[0].Group != "Arrays" {
		t.Fatalf("fresh cache lost metadata or fetched: %+v calls=%d err=%v", result, calls, err)
	}
	if _, err = db.DB().Exec(`UPDATE study_plans SET fetched_at=0`); err != nil {
		t.Fatal(err)
	}
	if _, err = resolvePlan(ctx, db, "leetcode-75", time.Hour, false, fetch); err != nil || calls != 2 {
		t.Fatalf("TTL did not refresh: calls=%d err=%v", calls, err)
	}
	if _, err = resolvePlan(ctx, db, "leetcode-75", time.Hour, true, fetch); err != nil || calls != 3 {
		t.Fatalf("force did not refresh: calls=%d err=%v", calls, err)
	}
	if _, err = db.DB().Exec(`UPDATE study_plans SET fetched_at=0`); err != nil {
		t.Fatal(err)
	}
	offline := func(context.Context, string) (leetcode.StudyPlan, error) {
		return leetcode.StudyPlan{}, errors.New("offline")
	}
	result, err = resolvePlan(ctx, db, "leetcode-75", time.Hour, false, offline)
	if err != nil || result.refreshErr == nil || result.plan.Questions[0].Group != "Arrays" {
		t.Fatalf("offline fallback: %+v %v", result, err)
	}
	cached, err := db.GetStudyPlan(ctx, "leetcode-75")
	if err != nil || cached.FetchedAt.Unix() != 0 {
		t.Fatalf("failed refresh changed timestamp: %+v %v", cached, err)
	}
	if _, err = resolvePlan(ctx, db, "missing", time.Hour, false, offline); err == nil {
		t.Fatal("uncached offline plan should fail")
	}
}

func TestLegacyAndBundledPlans(t *testing.T) {
	db := planTestDB(t)
	ctx := context.Background()
	fetch := func(context.Context, string) (leetcode.StudyPlan, error) {
		t.Fatal("unexpected network request")
		return leetcode.StudyPlan{}, nil
	}
	result, err := resolvePlan(ctx, db, "lazyleet-starter", time.Hour, true, fetch)
	if err != nil || len(result.plan.Questions) != 10 {
		t.Fatalf("bundled plan: %+v %v", result, err)
	}
	if err := db.PutStudyPlan(ctx, store.StudyPlan{Slug: "legacy", Name: "Legacy", Source: "leetcode", Problems: []string{"two-sum"}}); err != nil {
		t.Fatal(err)
	}
	result, err = resolvePlan(ctx, db, "legacy", time.Hour, false, fetch)
	if err != nil || result.plan.Questions[0].FrontendID != 1 {
		t.Fatalf("legacy cache: %+v %v", result, err)
	}
}

func planTestApp(t *testing.T) *appContext {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	app := &appContext{paths: config.ResolvePaths(), cfg: config.Default()}
	if err := app.paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	// Any accidental client creation fails locally instead of accessing LeetCode.
	if err := os.WriteFile(app.paths.AuthFile, []byte("invalid json"), 0600); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestDebugBundledPlanDoesNotReadCredentials(t *testing.T) {
	app := planTestApp(t)
	cmd := newDebugCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"plan", "lazyleet-starter"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "10 problems") || !strings.Contains(out.String(), "two-sum") {
		t.Fatal(out.String())
	}
}

func TestSyncPlansWithFreshCatalog(t *testing.T) {
	app := planTestApp(t)
	db, err := app.openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.UpsertProblems(context.Background(), []store.Problem{{FrontendID: 1, Slug: "two-sum", Title: "Two Sum"}}); err != nil {
		t.Fatal(err)
	}
	cmd := newSyncCmd(app)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--plans"})
	// The catalog is fresh, but official plans are missing. Attempting their
	// fetch must report our invalid credentials, proving they weren't skipped.
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "leetcode-75") {
		t.Fatalf("plan prefetch skipped with fresh catalog: %v", err)
	}
}

func TestPrefetchAttemptsEveryPlanAndReportsStaleFallback(t *testing.T) {
	db := planTestDB(t)
	app := &appContext{cfg: config.Default()}
	ctx := context.Background()
	if err := db.PutStudyPlan(ctx, store.StudyPlan{Slug: "leetcode-75", Name: "Old", Problems: []string{"two-sum"}}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := syncPlans(ctx, app, db, io.Discard, true, true, func(_ context.Context, slug string) (leetcode.StudyPlan, error) {
		calls++
		if slug == "leetcode-75" {
			return leetcode.StudyPlan{}, errors.New("offline")
		}
		return leetcode.StudyPlan{Slug: slug, Name: slug, Questions: []leetcode.StudyPlanQuestion{{Slug: "two-sum"}}}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "leetcode-75") || calls != len(officialPlans) {
		t.Fatalf("partial failure: calls=%d err=%v", calls, err)
	}
	for _, ref := range officialPlans {
		if _, err := db.GetStudyPlan(ctx, ref.slug); err != nil {
			t.Fatal(err)
		}
	}
}
