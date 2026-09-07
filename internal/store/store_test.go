package store

import (
	"path/filepath"
	"sort"
	"testing"
)

func TestOpenAppliesAllMigrations(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v != len(migrations) {
		t.Fatalf("SchemaVersion = %d, want %d", v, len(migrations))
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lazyleet.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, err := s1.DB().Exec(`INSERT INTO problems
        (frontend_id, question_id, slug, title, difficulty, updated_at)
        VALUES (1, 1, 'two-sum', 'Two Sum', 'Easy', 0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	var n int
	if err := s2.DB().QueryRow(`SELECT COUNT(*) FROM problems`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("row count after reopen = %d, want 1 (migrations must not re-run)", n)
	}
}

func TestExpectedTablesExist(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	rows, err := s.DB().Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}

	want := []string{
		"problem_detail", "problems", "schema_migrations",
		"study_plans", "submissions", "workspace_state",
	}
	sort.Strings(got)
	for _, w := range want {
		if !contains(got, w) {
			t.Errorf("missing table %q (have %v)", w, got)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
