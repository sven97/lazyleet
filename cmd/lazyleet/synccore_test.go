package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
)

// TestFetchAndCacheProblemsStaleSessionPreservesStatus covers the fix for a
// stored-but-invalid session: a plain catalog sync should still update
// catalog metadata (title, difficulty, ...) instead of hard-failing, and must
// not overwrite previously cached solve status with the blank/anonymous
// status LeetCode returns once the session can't be verified.
func TestFetchAndCacheProblemsStaleSessionPreservesStatus(t *testing.T) {
	app := authTestApp(t)
	ctx := context.Background()
	db, err := app.openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.UpsertProblems(ctx, []store.Problem{
		{FrontendID: 1, Slug: "two-sum", Title: "Two Sum", Difficulty: "Easy", Status: "ac"},
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Operation string `json:"operationName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Operation == "globalData" {
			// Stored credentials are present but LeetCode no longer
			// recognizes the session.
			fmt.Fprint(w, `{"data":{"userStatus":{"isSignedIn":false,"username":""}}}`)
			return
		}
		fmt.Fprint(w, `{"data":{"problemsetQuestionList":{"total":1,"questions":[
			{"frontendId":"1","slug":"two-sum","title":"Two Sum (Updated)","difficulty":"Easy"}
		]}}}`)
	}))
	defer server.Close()

	client := leetcode.New("com", leetcode.WithBaseURL(server.URL), leetcode.WithRateLimit(10000, 100),
		leetcode.WithCredentials(leetcode.Credentials{Session: "stale", CSRFToken: "stale"}))

	if _, err := fetchAndCacheProblems(ctx, client, db, nil); err != nil {
		t.Fatalf("catalog sync should degrade, not fail, on a stale session: %v", err)
	}

	rows, err := db.ListProblems(ctx, store.ProblemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Status != "ac" {
		t.Fatalf("stale session wiped cached status: %+v", rows[0])
	}
	if rows[0].Title != "Two Sum (Updated)" {
		t.Fatalf("catalog metadata was not refreshed: %+v", rows[0])
	}
}

// syncTestServer serves the problemsetQuestionList operation differently based
// on the request's limit and orderBy, letting a test distinguish the head
// check (limit 1), the incremental fetch (limit 100, descending), and the full
// fetch (limit 100, default order).
type syncTestServer struct {
	total                           int
	questionsByKind                 map[string]string // key: "head" | "incr" | "full"
	headCount, incrCount, fullCount int
}

func (s *syncTestServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Operation string `json:"operationName"`
			Variables struct {
				Limit   int `json:"limit"`
				Filters struct {
					OrderBy string `json:"orderBy"`
				} `json:"filters"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Operation != "problemsetQuestionList" {
			fmt.Fprint(w, `{"data":{}}`)
			return
		}
		kind := "full"
		if req.Variables.Limit == 1 {
			kind = "head"
			s.headCount++
		} else if req.Variables.Filters.OrderBy == "FRONTEND_ID" {
			kind = "incr"
			s.incrCount++
		} else {
			s.fullCount++
		}
		fmt.Fprintf(w, `{"data":{"problemsetQuestionList":{"total":%d,"questions":%s}}}`, s.total, s.questionsByKind[kind])
	}
}

func TestSyncCatalogIfChangedNoop(t *testing.T) {
	app := authTestApp(t)
	ctx := context.Background()
	db, err := app.openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.UpsertProblems(ctx, []store.Problem{{FrontendID: 1, Slug: "two-sum", Title: "Two Sum"}}); err != nil {
		t.Fatal(err)
	}

	srv := &syncTestServer{total: 1, questionsByKind: map[string]string{
		"head": `[{"frontendId":"1","slug":"two-sum","title":"Two Sum","difficulty":"Easy"}]`,
	}}
	server := httptest.NewServer(srv.handler(t))
	defer server.Close()
	client := leetcode.New("com", leetcode.WithBaseURL(server.URL), leetcode.WithRateLimit(10000, 100))

	count, synced, err := syncCatalogIfChanged(ctx, client, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || synced {
		t.Fatalf("count=%d synced=%v, want 1/false", count, synced)
	}
	if srv.incrCount != 0 || srv.fullCount != 0 {
		t.Fatalf("no-op sync still fetched: incr=%d full=%d", srv.incrCount, srv.fullCount)
	}
}

func TestSyncCatalogIfChangedIncrementalAppend(t *testing.T) {
	app := authTestApp(t)
	ctx := context.Background()
	db, err := app.openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.UpsertProblems(ctx, []store.Problem{{FrontendID: 1, Slug: "two-sum", Title: "Two Sum"}}); err != nil {
		t.Fatal(err)
	}

	srv := &syncTestServer{total: 3, questionsByKind: map[string]string{
		"head": `[{"frontendId":"3","slug":"three","title":"Three","difficulty":"Easy"}]`,
		"incr": `[{"frontendId":"3","slug":"three","title":"Three","difficulty":"Easy"},{"frontendId":"2","slug":"two","title":"Two","difficulty":"Easy"}]`,
	}}
	server := httptest.NewServer(srv.handler(t))
	defer server.Close()
	client := leetcode.New("com", leetcode.WithBaseURL(server.URL), leetcode.WithRateLimit(10000, 100))

	count, synced, err := syncCatalogIfChanged(ctx, client, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 || !synced {
		t.Fatalf("count=%d synced=%v, want 3/true", count, synced)
	}
	if srv.fullCount != 0 {
		t.Fatalf("append should not trigger a full sync (full=%d)", srv.fullCount)
	}
	rows, err := db.ListProblems(ctx, store.ProblemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestSyncCatalogIfChangedRemovalFullSync(t *testing.T) {
	app := authTestApp(t)
	ctx := context.Background()
	db, err := app.openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.UpsertProblems(ctx, []store.Problem{
		{FrontendID: 1, Slug: "two-sum", Title: "Two Sum"},
		{FrontendID: 2, Slug: "old", Title: "Old"},
	}); err != nil {
		t.Fatal(err)
	}

	srv := &syncTestServer{total: 1, questionsByKind: map[string]string{
		"head": `[{"frontendId":"1","slug":"two-sum","title":"Two Sum","difficulty":"Easy"}]`,
		"full": `[{"frontendId":"1","slug":"two-sum","title":"Two Sum (renamed)","difficulty":"Easy"}]`,
	}}
	server := httptest.NewServer(srv.handler(t))
	defer server.Close()
	client := leetcode.New("com", leetcode.WithBaseURL(server.URL), leetcode.WithRateLimit(10000, 100))

	count, synced, err := syncCatalogIfChanged(ctx, client, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || !synced {
		t.Fatalf("count=%d synced=%v, want 1/true", count, synced)
	}
	if srv.fullCount != 1 {
		t.Fatalf("removal should trigger a full sync (full=%d)", srv.fullCount)
	}
	rows, err := db.ListProblems(ctx, store.ProblemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title != "Two Sum (renamed)" {
		t.Fatalf("full resync did not apply: %+v", rows)
	}
}
