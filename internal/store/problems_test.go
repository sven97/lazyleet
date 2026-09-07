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

	if _, err := s.GetStudyPlan(ctx, "nope"); err != ErrNotCached {
		t.Fatalf("want ErrNotCached, got %v", err)
	}
	in := StudyPlan{Slug: "blind-75", Name: "Blind 75", Source: "bundled", Problems: []string{"two-sum", "valid-parentheses"}}
	if err := s.PutStudyPlan(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetStudyPlan(ctx, "blind-75")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Blind 75" || len(got.Problems) != 2 || got.Problems[1] != "valid-parentheses" {
		t.Fatalf("plan = %+v", got)
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
