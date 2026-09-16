package main

import (
	"context"
	"testing"

	"github.com/sven97/lazyleet/internal/attempt"
)

func TestAttemptHistoryOpensIndependentHandles(t *testing.T) {
	app := authTestApp(t)
	writer := attemptHistory{app: app}
	if err := writer.RecordAttempt(context.Background(), attempt.Entry{Slug: "two-sum", Lang: "python3", Kind: "submit", Verdict: "Accepted"}); err != nil {
		t.Fatal(err)
	}
	reader := attemptHistory{app: app}
	got, err := reader.RecentAttempts(context.Background(), "two-sum", 50)
	if err != nil || len(got) != 1 || got[0].Verdict != "Accepted" {
		t.Fatalf("history lost across handles: %+v %v", got, err)
	}
}
