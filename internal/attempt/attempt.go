// Package attempt defines persisted local and remote judge outcomes.
package attempt

import (
	"context"
	"time"
)

// Entry is a completed attempt. Detail is human-readable judge output.
// CreatedAt is when the attempt started; solution snapshots are not recorded.
type Entry struct {
	ID        int64
	Slug      string
	Lang      string
	Kind      string // local | run | submit
	Verdict   string
	Passed    int
	Total     int
	Runtime   string
	Memory    string
	RemoteID  string
	Detail    string
	CreatedAt time.Time
}

type Repository interface {
	RecordAttempt(context.Context, Entry) error
	RecentAttempts(context.Context, string, int) ([]Entry, error)
}
