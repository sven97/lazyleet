package main

import (
	"context"
	"github.com/sven97/lazyleet/internal/attempt"
)

// Each operation owns its handle so a command finishing after its workspace
// closes can still record the result safely.
type attemptHistory struct{ app *appContext }

func (h attemptHistory) RecordAttempt(ctx context.Context, e attempt.Entry) error {
	db, err := h.app.openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	return db.RecordAttempt(ctx, e)
}
func (h attemptHistory) RecentAttempts(ctx context.Context, slug string, limit int) ([]attempt.Entry, error) {
	db, err := h.app.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.RecentAttempts(ctx, slug, limit)
}
