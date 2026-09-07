package main

import (
	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
)

// openStore opens the SQLite database, creating and migrating it if needed.
// The caller must Close it.
func (a *appContext) openStore() (*store.Store, error) {
	if err := a.paths.EnsureDirs(); err != nil {
		return nil, err
	}
	return store.Open(a.paths.DatabaseFile)
}

// loadCredentials reads auth.json (anonymous if absent).
func (a *appContext) loadCredentials() (leetcode.Credentials, error) {
	return leetcode.LoadCredentials(a.paths.AuthFile)
}

// newClient builds a LeetCode client for the configured region, attaching
// stored credentials when present.
func (a *appContext) newClient() (*leetcode.Client, error) {
	creds, err := a.loadCredentials()
	if err != nil {
		return nil, err
	}
	opts := []leetcode.Option{}
	if !creds.Anonymous() {
		opts = append(opts, leetcode.WithCredentials(creds))
	}
	return leetcode.New(string(a.cfg.Region), opts...), nil
}
