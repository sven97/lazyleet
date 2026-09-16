package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
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
	// e.Runtime/e.Memory are free-form display strings (e.g. "12 ms", "4 MB",
	// or a local run's time.Duration.String() like "12ms"); they're always
	// kept verbatim in the JSON detail above for display. When they're also
	// numeric-parseable, mirror them into the typed runtime_ms/memory_kb
	// columns so they can be queried/sorted; otherwise leave those NULL.
	runtimeMS, memoryKB := parseRuntimeMS(e.Runtime), parseMemoryKB(e.Memory)
	_, err = s.db.ExecContext(ctx, `INSERT INTO submissions (slug,lang,submission_id,kind,verdict,runtime_ms,memory_kb,passed,total,detail,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, e.Slug, e.Lang, e.RemoteID, e.Kind, e.Verdict, runtimeMS, memoryKB, e.Passed, e.Total, string(detail), e.CreatedAt.Unix())
	return err
}

// parseRuntimeMS best-effort parses a human-readable runtime string into
// whole milliseconds: either a Go duration string (e.g. "12ms", "1.5s", as
// produced by a local run's time.Duration.String()) or a "<number> <unit>"
// pair (e.g. "12 ms", as reported by the remote judge). It returns an
// invalid sql.NullInt64 when the value isn't cleanly parseable (missing,
// "N/A", etc.), leaving the runtime_ms column NULL.
func parseRuntimeMS(s string) sql.NullInt64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullInt64{}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return sql.NullInt64{Int64: d.Milliseconds(), Valid: true}
	}
	v, unit, ok := splitNumberUnit(s)
	if !ok {
		return sql.NullInt64{}
	}
	switch strings.ToLower(unit) {
	case "ms":
		return sql.NullInt64{Int64: int64(math.Round(v)), Valid: true}
	case "s", "sec", "secs":
		return sql.NullInt64{Int64: int64(math.Round(v * 1000)), Valid: true}
	case "":
		return sql.NullInt64{Int64: int64(math.Round(v)), Valid: true}
	default:
		return sql.NullInt64{}
	}
}

// parseMemoryKB best-effort parses a human-readable memory string (e.g.
// "4 MB", "512 KB", as reported by the remote judge) into whole kilobytes.
// It returns an invalid sql.NullInt64 when the value isn't cleanly
// parseable, leaving the memory_kb column NULL.
func parseMemoryKB(s string) sql.NullInt64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullInt64{}
	}
	v, unit, ok := splitNumberUnit(s)
	if !ok {
		return sql.NullInt64{}
	}
	switch strings.ToLower(unit) {
	case "b":
		return sql.NullInt64{Int64: int64(math.Round(v / 1024)), Valid: true}
	case "kb", "":
		return sql.NullInt64{Int64: int64(math.Round(v)), Valid: true}
	case "mb":
		return sql.NullInt64{Int64: int64(math.Round(v * 1024)), Valid: true}
	case "gb":
		return sql.NullInt64{Int64: int64(math.Round(v * 1024 * 1024)), Valid: true}
	default:
		return sql.NullInt64{}
	}
}

// splitNumberUnit splits a "<number>" or "<number> <unit>" string (extra
// internal whitespace tolerated) into its numeric value and unit suffix.
func splitNumberUnit(s string) (value float64, unit string, ok bool) {
	fields := strings.Fields(s)
	switch len(fields) {
	case 1:
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return 0, "", false
		}
		return v, "", true
	case 2:
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return 0, "", false
		}
		return v, fields[1], true
	default:
		return 0, "", false
	}
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
