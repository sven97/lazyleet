// Package runner is lazyleet's local judge: it wraps the user's solution,
// feeds it each test case in a sandboxed subprocess, and compares the output.
//
// Drivers: python3/python, javascript, golang, java, cpp. Comparison is
// order-sensitive with float tolerance (see EqualOutputs). Design
// problems (empty Meta.Name) return a clear unsupported BuildErr.
package runner

import (
	"context"
	"time"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/testcase"
)

// Status is the outcome of one case.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"    // ran, produced the wrong answer
	StatusError   Status = "error"   // threw / crashed
	StatusTimeout Status = "timeout" // exceeded the per-run deadline
	StatusUnknown Status = "unknown" // ran, but no expected output to compare against
)

// Spec is everything a driver needs for one run.
type Spec struct {
	Lang         string
	SolutionPath string
	Meta         leetcode.Meta
	Cases        []testcase.Case
	Timeout      time.Duration // per whole run; 0 -> DefaultTimeout
}

// DefaultTimeout applies when Spec.Timeout is zero. It bounds only case
// execution; the compiled-language drivers (cpp/java/golang) give the
// compile step its own separate budget (CompileTimeout) so a slow build
// can't eat into the run budget and report a correct, fast solution as a
// spurious timeout.
const DefaultTimeout = 10 * time.Second

// CompileTimeout bounds the compile step for cpp/java/golang, independent of
// the run timeout above.
const CompileTimeout = 20 * time.Second

// CaseResult is the outcome for a single case.
type CaseResult struct {
	Index    int
	Status   Status
	Input    []string
	Expected string
	Actual   string
	Stdout   string
	Err      string
	Elapsed  time.Duration
}

// Result is the outcome of a whole run.
type Result struct {
	Cases    []CaseResult
	Passed   int
	Total    int
	Elapsed  time.Duration
	BuildErr string // compile / import failure; if set, Cases is empty
}

// OK reports whether every case passed (and there was at least one).
func (r Result) OK() bool { return r.BuildErr == "" && r.Total > 0 && r.Passed == r.Total }

// Runner is a language-specific driver.
type Runner interface {
	// Available reports whether the toolchain for this language is installed.
	Available() bool
	// Run executes every case in spec and returns the aggregate result. A
	// non-nil error means the run could not be attempted at all (bad spec,
	// missing toolchain); a solution that fails to compile is reported via
	// Result.BuildErr, not error.
	Run(ctx context.Context, spec Spec) (Result, error)
}

// For returns the driver for a language slug, or ok=false if none is
// implemented yet.
func For(lang string) (Runner, bool) {
	switch lang {
	case "python3", "python":
		return pythonRunner{bin: pythonBin(lang)}, true
	case "javascript":
		return javascriptRunner{}, true
	case "golang", "go":
		return golangRunner{}, true
	case "java":
		return javaRunner{}, true
	case "cpp", "c++":
		return cppRunner{}, true
	default:
		return nil, false
	}
}
