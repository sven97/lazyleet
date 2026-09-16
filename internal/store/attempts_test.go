package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/sven97/lazyleet/internal/attempt"
)

func TestAttemptsSurviveReopenAndAreScopedOrderedAndLimited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	for i, kind := range []string{"local", "run", "submit"} {
		e := attempt.Entry{Slug: "two-sum", Lang: "python3", Kind: kind, Verdict: "Passed", Passed: 2, Total: 2, Runtime: "12 ms", Memory: "4 MB", RemoteID: "123", Detail: "Expected: [0,1]\nActual: [0,1]", CreatedAt: at}
		if i == 1 {
			e.Lang = "golang"
		}
		if err := db.RecordAttempt(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RecordAttempt(context.Background(), attempt.Entry{Slug: "other", Lang: "java", Kind: "local", Verdict: "Build error"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.RecentAttempts(context.Background(), "two-sum", 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("read history: %+v %v", got, err)
	}
	if got[0].Kind != "submit" || got[1].Kind != "run" || got[1].Lang != "golang" {
		t.Fatalf("order/language lost: %+v", got)
	}
	e := got[0]
	if e.ID == 0 || e.RemoteID != "123" || e.Runtime != "12 ms" || e.Memory != "4 MB" || e.Total != 2 || e.Passed != 2 || e.Detail == "" || !e.CreatedAt.Equal(at) {
		t.Fatalf("fields lost: %+v", e)
	}
	empty, err := db.RecentAttempts(context.Background(), "never-attempted", 50)
	if err != nil || len(empty) != 0 {
		t.Fatal("history leaked across problems")
	}
	if err := db.RecordAttempt(context.Background(), attempt.Entry{Slug: "two-sum", Lang: "python3", Kind: "bad"}); err == nil {
		t.Fatal("invalid kind stored")
	}
}

func TestRecordAttemptPopulatesTypedRuntimeAndMemoryColumns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, tc := range []struct {
		name            string
		runtime, memory string
		wantRuntimeMS   sql.NullInt64
		wantMemoryKB    sql.NullInt64
	}{
		{"remote-style", "12 ms", "4 MB", sql.NullInt64{Int64: 12, Valid: true}, sql.NullInt64{Int64: 4096, Valid: true}},
		{"go-duration", "1.5s", "", sql.NullInt64{Int64: 1500, Valid: true}, sql.NullInt64{}},
		{"unparseable", "N/A", "unavailable", sql.NullInt64{}, sql.NullInt64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := attempt.Entry{Slug: "two-sum", Lang: "python3", Kind: "local", Verdict: "Passed", Runtime: tc.runtime, Memory: tc.memory, CreatedAt: time.Now()}
			if err := db.RecordAttempt(context.Background(), e); err != nil {
				t.Fatal(err)
			}
			var runtimeMS, memoryKB sql.NullInt64
			row := db.db.QueryRow(`SELECT runtime_ms, memory_kb FROM submissions ORDER BY id DESC LIMIT 1`)
			if err := row.Scan(&runtimeMS, &memoryKB); err != nil {
				t.Fatal(err)
			}
			if runtimeMS != tc.wantRuntimeMS || memoryKB != tc.wantMemoryKB {
				t.Fatalf("runtime=%q memory=%q: got runtime_ms=%v memory_kb=%v, want %v/%v", tc.runtime, tc.memory, runtimeMS, memoryKB, tc.wantRuntimeMS, tc.wantMemoryKB)
			}
			// The human-readable display strings must still round-trip as-is.
			got, err := db.RecentAttempts(context.Background(), "two-sum", 1)
			if err != nil || len(got) != 1 || got[0].Runtime != tc.runtime || got[0].Memory != tc.memory {
				t.Fatalf("display strings changed: %+v %v", got, err)
			}
		})
	}
}
