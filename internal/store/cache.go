package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotCached is returned when a cache lookup misses.
var ErrNotCached = errors.New("not cached")

// --- study plans ----------------------------------------------------------

// StudyPlan is a cached study plan (ordered slug list plus display name).
type StudyPlan struct {
	Slug      string
	Name      string
	Source    string // leetcode | bundled
	Problems  []string
	FetchedAt time.Time
}

// PutStudyPlan upserts a plan.
func (s *Store) PutStudyPlan(ctx context.Context, p StudyPlan) error {
	blob, err := json.Marshal(p.Problems)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO study_plans (slug, name, source, problems, fetched_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(slug) DO UPDATE SET
    name = excluded.name, source = excluded.source,
    problems = excluded.problems, fetched_at = excluded.fetched_at`,
		p.Slug, p.Name, p.Source, string(blob), time.Now().Unix())
	return err
}

// GetStudyPlan returns a cached plan, or ErrNotCached.
func (s *Store) GetStudyPlan(ctx context.Context, slug string) (StudyPlan, error) {
	var (
		p         StudyPlan
		probJSON  string
		fetchedAt int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT slug, name, source, problems, fetched_at FROM study_plans WHERE slug = ?`, slug).
		Scan(&p.Slug, &p.Name, &p.Source, &probJSON, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return StudyPlan{}, ErrNotCached
	}
	if err != nil {
		return StudyPlan{}, err
	}
	p.FetchedAt = time.Unix(fetchedAt, 0)
	_ = json.Unmarshal([]byte(probJSON), &p.Problems)
	return p, nil
}

// --- problem detail -----------------------------------------------------------

// ProblemDetail is a cached problem-detail blob. The heavy fields
// (statement/meta/snippets) are stored as-is; callers own the JSON shape.
type ProblemDetail struct {
	Slug             string
	QuestionID       int
	StatementMD      string
	MetaJSON         string
	ExampleTestcases string
	ExampleCasesJSON string // JSON: []testcase.Case (inputs + expected outputs)
	SampleTestcase   string
	CodeSnippetsJSON string // JSON: langSlug -> code
	HintsJSON        string
	FetchedAt        time.Time
}

// PutProblemDetail upserts a detail blob.
func (s *Store) PutProblemDetail(ctx context.Context, d ProblemDetail) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO problem_detail (slug, question_id, content_html, meta_data, example_testcases, example_cases, sample_testcase, code_snippets, hints, similar, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', ?)
ON CONFLICT(slug) DO UPDATE SET
    question_id = excluded.question_id,
    content_html = excluded.content_html,
    meta_data = excluded.meta_data,
    example_testcases = excluded.example_testcases,
    example_cases = excluded.example_cases,
    sample_testcase = excluded.sample_testcase,
    code_snippets = excluded.code_snippets,
    hints = excluded.hints,
    fetched_at = excluded.fetched_at`,
		d.Slug, d.QuestionID, d.StatementMD, orJSON(d.MetaJSON), d.ExampleTestcases,
		orJSONArray(d.ExampleCasesJSON), d.SampleTestcase, orJSON(d.CodeSnippetsJSON),
		orJSONArray(d.HintsJSON), time.Now().Unix())
	return err
}

// GetProblemDetail returns a cached detail blob younger than ttl, or
// ErrNotCached (also when the row exists but is stale).
func (s *Store) GetProblemDetail(ctx context.Context, slug string, ttl time.Duration) (ProblemDetail, error) {
	var (
		d         ProblemDetail
		fetchedAt int64
	)
	err := s.db.QueryRowContext(ctx, `
SELECT slug, question_id, content_html, meta_data, example_testcases, example_cases, sample_testcase, code_snippets, hints, fetched_at
FROM problem_detail WHERE slug = ?`, slug).
		Scan(&d.Slug, &d.QuestionID, &d.StatementMD, &d.MetaJSON, &d.ExampleTestcases,
			&d.ExampleCasesJSON, &d.SampleTestcase, &d.CodeSnippetsJSON, &d.HintsJSON, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProblemDetail{}, ErrNotCached
	}
	if err != nil {
		return ProblemDetail{}, err
	}
	d.FetchedAt = time.Unix(fetchedAt, 0)
	if ttl > 0 && time.Since(d.FetchedAt) >= ttl {
		return ProblemDetail{}, ErrNotCached
	}
	return d, nil
}

func orJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}

func orJSONArray(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}
