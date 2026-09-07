package main

import (
	"testing"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
)

func TestQuestionCacheRoundTrip(t *testing.T) {
	orig, ok := leetcode.Fixture("two-sum")
	if !ok {
		t.Fatal("fixture missing")
	}

	row := store.Problem{FrontendID: 1, Title: "Two Sum", Difficulty: "Easy"}
	got := questionFromCache(cacheFromQuestion(orig), row)

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

func TestQuestionFromCacheWithoutRow(t *testing.T) {
	orig, _ := leetcode.Fixture("two-sum")
	got := questionFromCache(cacheFromQuestion(orig), store.Problem{})
	if got.Title != "two-sum" {
		t.Fatalf("title should fall back to slug, got %q", got.Title)
	}
}
