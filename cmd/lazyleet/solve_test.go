package main

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
)

func TestQuestionCacheRoundTrip(t *testing.T) {
	orig, ok := leetcode.Fixture("two-sum")
	if !ok {
		t.Fatal("fixture missing")
	}

	orig.Hints = []string{"Use a **map**", "Look up the complement."}
	orig.HintsFetched = true
	row := store.Problem{FrontendID: 1, Title: "Two Sum", Difficulty: "Easy"}
	got := questionFromCache(cacheFromQuestion(orig), row)

	if !reflect.DeepEqual(got.Hints, orig.Hints) {
		t.Fatalf("hints lost: %q", got.Hints)
	}
	if !got.HintsFetched {
		t.Fatal("HintsFetched lost across cache round trip")
	}
	if got.Slug != orig.Slug || got.QuestionID != orig.QuestionID {
		t.Fatalf("identity lost: %+v", got)
	}
	if got.Meta.Name != "twoSum" || got.Meta.Arity() != 2 {
		t.Fatalf("meta lost: %+v", got.Meta)
	}
	if got.CodeSnippets["python3"] != orig.CodeSnippets["python3"] {
		t.Fatalf("snippet lost: %q", got.CodeSnippets["python3"])
	}
	if len(got.ExampleCases) != len(orig.ExampleCases) {
		t.Fatalf("example cases lost: %d vs %d", len(got.ExampleCases), len(orig.ExampleCases))
	}
	if got.ExampleCases[0].In[0] != "[2,7,11,15]" {
		t.Fatalf("example case content wrong: %+v", got.ExampleCases[0])
	}
	if got.Title != "Two Sum" || got.Difficulty != "Easy" {
		t.Fatalf("row fields not applied: %q %q", got.Title, got.Difficulty)
	}
}

// TestHintsFetchedDistinguishesEmptyFromStaleCache verifies that a problem
// with genuinely no hints (fetched, HintsFetched=true, Hints empty) is
// distinguishable from a cache row written before hints support existed
// (HintsFetched=false, the column default) — both end up with the same
// empty "hints" JSON, so HintsFetched is the only signal available.
func TestHintsFetchedDistinguishesEmptyFromStaleCache(t *testing.T) {
	orig, ok := leetcode.Fixture("two-sum")
	if !ok {
		t.Fatal("fixture missing")
	}
	row := store.Problem{FrontendID: 1, Title: "Two Sum", Difficulty: "Easy"}

	// Genuinely fetched, genuinely no hints.
	orig.Hints = nil
	orig.HintsFetched = true
	got := questionFromCache(cacheFromQuestion(orig), row)
	if len(got.Hints) != 0 || !got.HintsFetched {
		t.Fatalf("genuinely-empty hints mis-cached: hints=%q fetched=%v", got.Hints, got.HintsFetched)
	}

	// A row as it would look if written before hints support existed: same
	// empty hints JSON, but HintsFetched was never set.
	orig.HintsFetched = false
	staleCached := cacheFromQuestion(orig)
	got = questionFromCache(staleCached, row)
	if len(got.Hints) != 0 || got.HintsFetched {
		t.Fatalf("stale pre-hints cache should read back as not-fetched: hints=%q fetched=%v", got.Hints, got.HintsFetched)
	}
}

func TestQuestionFromCacheWithoutRow(t *testing.T) {
	orig, _ := leetcode.Fixture("two-sum")
	got := questionFromCache(cacheFromQuestion(orig), store.Problem{})
	if got.Title != "two-sum" {
		t.Fatalf("title should fall back to slug, got %q", got.Title)
	}
}

func TestCachedHintsRemainAvailableOffline(t *testing.T) {
	for _, slug := range []string{"two-sum", "uncatalogued-problem"} {
		for _, stale := range []bool{false, true} {
			t.Run(slug+map[bool]string{false: "/fresh", true: "/stale"}[stale], func(t *testing.T) {
				app := authTestApp(t)
				db, err := app.openStore()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				q, _ := leetcode.Fixture("two-sum")
				q.Slug = slug
				q.Hints = []string{"Cached hint one", "Cached hint two"}
				if err = db.PutProblemDetail(context.Background(), cacheFromQuestion(q)); err != nil {
					t.Fatal(err)
				}
				if stale {
					if _, err = db.DB().Exec(`UPDATE problem_detail SET fetched_at=0`); err != nil {
						t.Fatal(err)
					}
				}
				// Fail any attempt to construct a network client. Cached hints must still
				// work, including fixture slugs and expired details.
				if err = os.WriteFile(app.paths.AuthFile, []byte("invalid credentials JSON"), 0600); err != nil {
					t.Fatal(err)
				}
				got, _, err := app.resolveQuestion(context.Background(), slug, false)
				if err != nil || !reflect.DeepEqual(got.Hints, q.Hints) {
					t.Fatalf("offline hints: %q %v", got.Hints, err)
				}
				if _, _, err = app.resolveQuestion(context.Background(), slug, true); err == nil {
					t.Fatal("explicit refresh silently used cached data")
				}
			})
		}
	}
}
