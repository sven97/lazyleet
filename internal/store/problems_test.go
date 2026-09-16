package store

import (
	"context"
	"testing"
	"time"
)

func seedProblems(t *testing.T, s *Store) {
	t.Helper()
	rows := []Problem{
		{FrontendID: 1, Slug: "two-sum", Title: "Two Sum", Difficulty: "Easy", ACRate: 52, Status: "ac", TopicTags: []string{"array", "hash-table"}},
		{FrontendID: 2, Slug: "add-two-numbers", Title: "Add Two Numbers", Difficulty: "Medium", ACRate: 40, TopicTags: []string{"linked-list"}},
		{FrontendID: 4, Slug: "median", Title: "Median of Two Sorted Arrays", Difficulty: "Hard", ACRate: 38, PaidOnly: true, TopicTags: []string{"array", "binary-search"}},
	}
	if err := s.UpsertProblems(context.Background(), rows); err != nil {
		t.Fatalf("UpsertProblems: %v", err)
	}
}

func TestUpsertAndListProblems(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()
	seedProblems(t, s)

	ctx := context.Background()
	if n, _ := s.ProblemCount(ctx); n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}

	all, err := s.ListProblems(ctx, ProblemFilter{})
	if err != nil {
		t.Fatalf("ListProblems: %v", err)
	}
	if len(all) != 3 || all[0].FrontendID != 1 || all[2].FrontendID != 4 {
		t.Fatalf("order wrong: %+v", all)
	}
	if len(all[0].TopicTags) != 2 || all[0].TopicTags[0] != "array" {
		t.Errorf("tags round trip: %+v", all[0].TopicTags)
	}

	easy, _ := s.ListProblems(ctx, ProblemFilter{Difficulty: "Easy"})
	if len(easy) != 1 || easy[0].Slug != "two-sum" {
		t.Errorf("difficulty filter: %+v", easy)
	}

	noPaid, _ := s.ListProblems(ctx, ProblemFilter{ExcludePaid: true})
	if len(noPaid) != 2 {
		t.Errorf("ExcludePaid: got %d, want 2", len(noPaid))
	}

	byTag, _ := s.ListProblems(ctx, ProblemFilter{Tag: "binary-search"})
	if len(byTag) != 1 || byTag[0].FrontendID != 4 {
		t.Errorf("tag filter: %+v", byTag)
	}

	// "Two" appears in all three titles; "Sum" only in the first.
	if got, _ := s.ListProblems(ctx, ProblemFilter{Search: "Two"}); len(got) != 3 {
		t.Errorf("search 'Two': got %d, want 3", len(got))
	}
	search, _ := s.ListProblems(ctx, ProblemFilter{Search: "Sum"})
	if len(search) != 1 || search[0].Slug != "two-sum" {
		t.Errorf("search 'Sum': got %d, want 1 (%+v)", len(search), search)
	}
	if byID, _ := s.ListProblems(ctx, ProblemFilter{Search: "4"}); len(byID) != 1 || byID[0].FrontendID != 4 {
		t.Errorf("search by id '4': %+v", byID)
	}
}

func TestUpsertPreservesQuestionID(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()
	ctx := context.Background()

	// list sync: question_id unknown (0)
	if err := s.UpsertProblems(ctx, []Problem{{FrontendID: 1, Slug: "two-sum", Title: "Two Sum", Difficulty: "Easy"}}); err != nil {
		t.Fatal(err)
	}
	// detail fetch fills it in
	if err := s.UpsertProblems(ctx, []Problem{{FrontendID: 1, QuestionID: 1, Slug: "two-sum", Title: "Two Sum", Difficulty: "Easy"}}); err != nil {
		t.Fatal(err)
	}
	// a later list sync (question_id 0 again) must not wipe it
	if err := s.UpsertProblems(ctx, []Problem{{FrontendID: 1, Slug: "two-sum", Title: "Two Sum", Difficulty: "Easy", Status: "ac"}}); err != nil {
		t.Fatal(err)
	}

	rows, _ := s.ListProblems(ctx, ProblemFilter{})
	if rows[0].QuestionID != 1 {
		t.Fatalf("question_id = %d, want 1 (list sync wiped it)", rows[0].QuestionID)
	}
	if rows[0].Status != "ac" {
		t.Errorf("status not updated: %q", rows[0].Status)
	}
}

func TestProblemsFresh(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()
	ctx := context.Background()

	if fresh, _ := s.ProblemsFresh(ctx, time.Hour); fresh {
		t.Error("empty cache should not be fresh")
	}
	seedProblems(t, s)
	if fresh, _ := s.ProblemsFresh(ctx, time.Hour); !fresh {
		t.Error("just-synced cache should be fresh")
	}
	if fresh, _ := s.ProblemsFresh(ctx, time.Nanosecond); fresh {
		t.Error("cache older than a nanosecond ttl should be stale")
	}
}

func TestStudyPlanRoundTrip(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()
	ctx := context.Background()

	if _, err := s.GetStudyPlan(ctx, "nope", 0); err != ErrNotCached {
		t.Fatalf("want ErrNotCached, got %v", err)
	}
	in := StudyPlan{Slug: "blind-75", Name: "Blind 75", Source: "bundled", Problems: []string{"two-sum", "valid-parentheses"}}
	if err := s.PutStudyPlan(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetStudyPlan(ctx, "blind-75", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Blind 75" || len(got.Problems) != 2 || got.Problems[1] != "valid-parentheses" {
		t.Fatalf("plan = %+v", got)
	}
	if _, err := s.GetStudyPlan(ctx, "blind-75", time.Hour); err != nil {
		t.Fatalf("plan within ttl should be fresh: %v", err)
	}
	if _, err := s.GetStudyPlan(ctx, "blind-75", time.Nanosecond); err != ErrNotCached {
		t.Fatalf("stale plan should be ErrNotCached, got %v", err)
	}
}

func TestProblemDetailTTL(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()
	ctx := context.Background()

	d := ProblemDetail{Slug: "two-sum", QuestionID: 1, StatementMD: "# Two Sum", CodeSnippetsJSON: `{"python3":"x"}`}
	if err := s.PutProblemDetail(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProblemDetail(ctx, "two-sum", time.Hour)
	if err != nil {
		t.Fatalf("GetProblemDetail: %v", err)
	}
	if got.StatementMD != "# Two Sum" || got.QuestionID != 1 {
		t.Fatalf("detail = %+v", got)
	}
	if _, err := s.GetProblemDetail(ctx, "two-sum", time.Nanosecond); err != ErrNotCached {
		t.Fatalf("stale detail should be ErrNotCached, got %v", err)
	}
}

func TestProblemDetailExampleCasesRoundTrip(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()
	ctx := context.Background()

	// a fresh row keeps the JSON we hand it
	if err := s.PutProblemDetail(ctx, ProblemDetail{
		Slug: "x", ExampleCasesJSON: `[{"in":["1002"],"out":"3"}]`,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProblemDetail(ctx, "x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExampleCasesJSON != `[{"in":["1002"],"out":"3"}]` {
		t.Fatalf("example_cases round-trip = %q", got.ExampleCasesJSON)
	}

	// a row written without it defaults to "[]" (covers migrated old rows)
	if err := s.PutProblemDetail(ctx, ProblemDetail{Slug: "y"}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetProblemDetail(ctx, "y", time.Hour)
	if got.ExampleCasesJSON != "[]" {
		t.Fatalf("missing example_cases should default to []: %q", got.ExampleCasesJSON)
	}
}

func TestReplaceProblemStatuses(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()
	seedProblems(t, s) // two-sum starts "ac"
	ctx := context.Background()

	n, err := s.ReplaceProblemStatuses(ctx,
		[]string{"add-two-numbers", "not-cached"}, // ac
		[]string{"median"},                        // tried
	)
	if err != nil {
		t.Fatalf("ReplaceProblemStatuses: %v", err)
	}
	if n != 1 {
		t.Fatalf("ac rows affected = %d, want 1 (not-cached is ignored)", n)
	}

	got := map[string]string{}
	rows, _ := s.ListProblems(ctx, ProblemFilter{})
	for _, r := range rows {
		got[r.Slug] = r.Status
	}
	want := map[string]string{"two-sum": "", "add-two-numbers": "ac", "median": "notac"}
	for slug, w := range want {
		if got[slug] != w {
			t.Errorf("%s status = %q, want %q (prior statuses must be cleared)", slug, got[slug], w)
		}
	}

	// updated_at must be untouched so catalog-staleness tracking is unaffected.
	before, _ := s.ProblemsLastSynced(ctx)
	if _, err := s.ReplaceProblemStatuses(ctx, []string{"two-sum"}, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := s.ProblemsLastSynced(ctx)
	if !before.Equal(after) {
		t.Errorf("ProblemsLastSynced moved %v -> %v; ReplaceProblemStatuses must not touch updated_at", before, after)
	}
}
