package tui

import "context"

// RemoteOutcome is the result of running or submitting against LeetCode's judge,
// mapped out of the leetcode package so tui stays transport-agnostic.
type RemoteOutcome struct {
	Kind       string // "run" | "submit"
	Verdict    string // human-readable, e.g. "Accepted", "Wrong Answer"
	Accepted   bool
	Passed     int
	Total      int
	Runtime    string  // e.g. "12 ms"
	Memory     string  // e.g. "17.1 MB"
	RuntimePct float64 // percentile, 0..100
	MemoryPct  float64
	LastCase   string   // failing test-case input (LeetCode line format)
	Stdout     []string // captured prints, per case
	Expected   []string
	Actual     []string
	CompileErr string
	RuntimeErr string
}

// RemoteJudge runs and submits code against LeetCode. The cmd layer implements
// it over a *leetcode.Client; tests use a fake. A nil RemoteJudge (or one whose
// Available reports false) disables the R/s keys with a hint.
type RemoteJudge interface {
	// Available reports whether remote run/submit can be attempted (i.e. the
	// user is authenticated and the question id is known).
	Available() bool
	// Run executes code against dataInput (LeetCode's "Run Code"), polling to
	// completion.
	Run(ctx context.Context, code, dataInput string) (RemoteOutcome, error)
	// Submit submits code for full judging, polling to completion.
	Submit(ctx context.Context, code string) (RemoteOutcome, error)
}
