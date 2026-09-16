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
