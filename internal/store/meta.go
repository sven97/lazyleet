package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// Meta keys used by the app. Keep them here so callers don't hard-code strings.
const (
	MetaProgressSyncedAt = "progress_synced_at" // unix seconds; last user solve-status refresh
)

// SetMeta upserts a key/value pair in the meta scratch table.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES(?, ?)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// GetMeta returns the value for key. ok is false when the key is absent.
func (s *Store) GetMeta(ctx context.Context, key string) (value string, ok bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetMetaTime stores t as unix seconds under key.
func (s *Store) SetMetaTime(ctx context.Context, key string, t time.Time) error {
	return s.SetMeta(ctx, key, strconv.FormatInt(t.Unix(), 10))
}

// GetMetaTime reads a unix-seconds value written by SetMetaTime. ok is false
// when the key is absent or unparseable.
func (s *Store) GetMetaTime(ctx context.Context, key string) (t time.Time, ok bool, err error) {
	v, ok, err := s.GetMeta(ctx, key)
	if err != nil || !ok {
		return time.Time{}, false, err
	}
	secs, perr := strconv.ParseInt(v, 10, 64)
	if perr != nil {
		return time.Time{}, false, nil
	}
	return time.Unix(secs, 0), true, nil
}
