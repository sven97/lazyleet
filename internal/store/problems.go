package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Problem is a cached problem-list row. It mirrors the `problems` table; the
// leetcode package's DTO is mapped onto this by the sync layer so store and
// leetcode stay decoupled.
type Problem struct {
	FrontendID int
	QuestionID int
	Slug       string
	Title      string
	Difficulty string
	ACRate     float64
	PaidOnly   bool
	Status     string
	TopicTags  []string
	UpdatedAt  time.Time
}

// UpsertProblems writes rows in one transaction, replacing existing entries by
// frontend_id. question_id is only overwritten when the incoming value is
// non-zero (the list endpoint doesn't return it; detail fetches do). It does
// not delete rows absent from the input; use ReplaceProblems for a full catalog
// sync that prunes removed problems.
func (s *Store) UpsertProblems(ctx context.Context, rows []Problem) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.writeProblems(ctx, tx, rows); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplaceProblems writes rows as the complete problem catalog: it wipes the
// table and inserts exactly these rows, so problems LeetCode no longer lists
// are pruned. Any previously cached internal question_id (which the list
// endpoint never returns) is carried over onto the replacement rows.
func (s *Store) ReplaceProblems(ctx context.Context, rows []Problem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(rows) > 0 {
		ids, err := tx.QueryContext(ctx, `SELECT frontend_id, question_id FROM problems WHERE question_id != 0`)
		if err != nil {
			return err
		}
		for ids.Next() {
			var fid, qid int
			if err := ids.Scan(&fid, &qid); err != nil {
				ids.Close()
				return err
			}
			for i := range rows {
				if rows[i].FrontendID == fid && rows[i].QuestionID == 0 {
					rows[i].QuestionID = qid
				}
			}
		}
		ids.Close()
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM problems`); err != nil {
		return err
	}
	if err := s.writeProblems(ctx, tx, rows); err != nil {
		return err
	}
	return tx.Commit()
}

// writeProblems inserts (or upserts, via ON CONFLICT) rows into problems within
// an open transaction. When the table has just been cleared, the conflict path
// never triggers and this behaves as a plain insert.
func (s *Store) writeProblems(ctx context.Context, tx *sql.Tx, rows []Problem) error {
	if len(rows) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO problems (frontend_id, question_id, slug, title, difficulty, ac_rate, paid_only, status, topic_tags, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(frontend_id) DO UPDATE SET
    question_id = CASE WHEN excluded.question_id != 0 THEN excluded.question_id ELSE problems.question_id END,
    slug = excluded.slug,
    title = excluded.title,
    difficulty = excluded.difficulty,
    ac_rate = excluded.ac_rate,
    paid_only = excluded.paid_only,
    status = excluded.status,
    topic_tags = excluded.topic_tags,
    updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, r := range rows {
		tags, _ := json.Marshal(r.TopicTags)
		if _, err := stmt.ExecContext(ctx,
			r.FrontendID, r.QuestionID, r.Slug, r.Title, r.Difficulty,
			r.ACRate, boolToInt(r.PaidOnly), r.Status, string(tags), now,
		); err != nil {
			return fmt.Errorf("upsert problem %d (%s): %w", r.FrontendID, r.Slug, err)
		}
	}
	return nil
}

// ProblemFilter narrows ListProblems. Zero value returns everything.
type ProblemFilter struct {
	Difficulty    string // Easy | Medium | Hard
	Status        string // ac | notac
	Search        string // matched against title and frontend_id
	Tag           string // single topic tag slug
	ExcludePaid   bool
	Limit, Offset int
}

// ListProblems returns cached rows matching the filter, ordered by frontend_id.
func (s *Store) ListProblems(ctx context.Context, f ProblemFilter) ([]Problem, error) {
	var where []string
	var args []any

	if f.Difficulty != "" {
		where = append(where, "difficulty = ?")
		args = append(args, f.Difficulty)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.ExcludePaid {
		where = append(where, "paid_only = 0")
	}
	if f.Search != "" {
		where = append(where, "(title LIKE ? OR CAST(frontend_id AS TEXT) LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}
	if f.Tag != "" {
		where = append(where, "topic_tags LIKE ?")
		args = append(args, `%"`+f.Tag+`"%`)
	}

	q := `SELECT frontend_id, question_id, slug, title, difficulty, ac_rate, paid_only, status, topic_tags, updated_at FROM problems`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY frontend_id"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Problem
	for rows.Next() {
		var (
			p        Problem
			paid     int
			tagsJSON string
			updated  int64
		)
		if err := rows.Scan(&p.FrontendID, &p.QuestionID, &p.Slug, &p.Title, &p.Difficulty,
			&p.ACRate, &paid, &p.Status, &tagsJSON, &updated); err != nil {
			return nil, err
		}
		p.PaidOnly = paid != 0
		p.UpdatedAt = time.Unix(updated, 0)
		_ = json.Unmarshal([]byte(tagsJSON), &p.TopicTags)
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProblem returns a single cached problem row by slug, or ErrNotCached.
func (s *Store) GetProblem(ctx context.Context, slug string) (Problem, error) {
	var (
		p        Problem
		paid     int
		tagsJSON string
		updated  int64
	)
	err := s.db.QueryRowContext(ctx, `
SELECT frontend_id, question_id, slug, title, difficulty, ac_rate, paid_only, status, topic_tags, updated_at
FROM problems WHERE slug = ?`, slug).
		Scan(&p.FrontendID, &p.QuestionID, &p.Slug, &p.Title, &p.Difficulty,
			&p.ACRate, &paid, &p.Status, &tagsJSON, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Problem{}, ErrNotCached
	}
	if err != nil {
		return Problem{}, err
	}
	p.PaidOnly = paid != 0
	p.UpdatedAt = time.Unix(updated, 0)
	_ = json.Unmarshal([]byte(tagsJSON), &p.TopicTags)
	return p, nil
}

// SetProblemStatus updates the solve status ("" | ac | notac) for one problem.
// A no-op if the slug is not cached.
func (s *Store) SetProblemStatus(ctx context.Context, slug, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE problems SET status = ? WHERE slug = ?`, status, slug)
	return err
}

// ReplaceProblemStatuses rewrites the solve status of every cached problem from
// the user's solved ("ac") and attempted ("notac") slug sets — a lightweight
// alternative to a full catalog sync. It clears all existing statuses first so
// an un-accepted problem doesn't linger. updated_at is deliberately left alone
// so catalog-staleness tracking is unaffected. Returns how many rows were
// marked "ac".
func (s *Store) ReplaceProblemStatuses(ctx context.Context, acSlugs, triedSlugs []string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE problems SET status = '' WHERE status != ''`); err != nil {
		return 0, err
	}

	mark := func(slugs []string, status string) (int64, error) {
		var affected int64
		const chunk = 800 // stay well under SQLite's variable limit
		for start := 0; start < len(slugs); start += chunk {
			end := min(start+chunk, len(slugs))
			batch := slugs[start:end]
			ph := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
			args := make([]any, 0, len(batch)+1)
			args = append(args, status)
			for _, sl := range batch {
				args = append(args, sl)
			}
			res, err := tx.ExecContext(ctx,
				`UPDATE problems SET status = ? WHERE slug IN (`+ph+`)`, args...)
			if err != nil {
				return affected, err
			}
			n, _ := res.RowsAffected()
			affected += n
		}
		return affected, nil
	}

	acCount, err := mark(acSlugs, "ac")
	if err != nil {
		return 0, err
	}
	if _, err := mark(triedSlugs, "notac"); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(acCount), nil
}

// ProblemCount returns how many problem rows are cached.
func (s *Store) ProblemCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM problems`).Scan(&n)
	return n, err
}

// MaxFrontendID returns the highest cached frontend id, or 0 when the cache is
// empty. Used alongside ProblemCount to cheaply detect catalog drift: LeetCode
// assigns monotonically increasing frontend ids, so a higher remote max means
// new problems have been published.
func (s *Store) MaxFrontendID(ctx context.Context) (int, error) {
	var v sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(frontend_id) FROM problems`).Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// ProblemsLastSynced returns the newest updated_at across the problem cache, or
// the zero time if the cache is empty.
func (s *Store) ProblemsLastSynced(ctx context.Context) (time.Time, error) {
	var v sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(updated_at) FROM problems`).Scan(&v); err != nil {
		return time.Time{}, err
	}
	if !v.Valid {
		return time.Time{}, nil
	}
	return time.Unix(v.Int64, 0), nil
}

// ProblemsFresh reports whether the problem cache exists and is younger than ttl.
func (s *Store) ProblemsFresh(ctx context.Context, ttl time.Duration) (bool, error) {
	last, err := s.ProblemsLastSynced(ctx)
	if err != nil || last.IsZero() {
		return false, err
	}
	return time.Since(last) < ttl, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
