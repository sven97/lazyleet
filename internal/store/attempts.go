package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sven97/lazyleet/internal/attempt"
)

type attemptDetail struct{ Runtime, Memory, Text string }

func (s *Store) RecordAttempt(ctx context.Context, e attempt.Entry) error {
	if e.Slug == "" || e.Lang == "" {
		return fmt.Errorf("attempt requires problem and language")
	}
	if e.Kind != "local" && e.Kind != "run" && e.Kind != "submit" {
		return fmt.Errorf("invalid attempt kind %q", e.Kind)
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	detail, err := json.Marshal(attemptDetail{Runtime: e.Runtime, Memory: e.Memory, Text: e.Detail})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO submissions (slug,lang,submission_id,kind,verdict,passed,total,detail,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, e.Slug, e.Lang, e.RemoteID, e.Kind, e.Verdict, e.Passed, e.Total, string(detail), e.CreatedAt.Unix())
	return err
}

// RecentAttempts returns newest first, with ID as a stable tie-breaker when
// attempts begin within the same second. Limits are bounded to keep the view
// responsive even after many run-on-save attempts.
func (s *Store) RecentAttempts(ctx context.Context, slug string, limit int) ([]attempt.Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,slug,lang,submission_id,kind,verdict,COALESCE(passed,0),COALESCE(total,0),detail,created_at FROM submissions WHERE slug=? ORDER BY created_at DESC,id DESC LIMIT ?`, slug, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []attempt.Entry
	for rows.Next() {
		var e attempt.Entry
		var raw string
		var timestamp int64
		if err := rows.Scan(&e.ID, &e.Slug, &e.Lang, &e.RemoteID, &e.Kind, &e.Verdict, &e.Passed, &e.Total, &raw, &timestamp); err != nil {
			return nil, err
		}
		var d attemptDetail
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			return nil, fmt.Errorf("attempt %d detail: %w", e.ID, err)
		}
		e.Runtime, e.Memory, e.Detail = d.Runtime, d.Memory, d.Text
		e.CreatedAt = time.Unix(timestamp, 0)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
