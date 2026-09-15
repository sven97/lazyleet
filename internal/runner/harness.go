package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sven97/lazyleet/internal/leetcode"
)

// designUnsupportedMsg is shown when Meta.Name is empty (LRU Cache-style).
const designUnsupportedMsg = "design problems (class/method sequences) are not supported by the local judge yet; use R to run on LeetCode"

// payload is the JSON handed to scripting-language harnesses on stdin.
type payload struct {
	Entry  string        `json:"entry"`
	Source string        `json:"source"`
	Cases  []payloadCase `json:"cases"`
}

type payloadCase struct {
	In  []string `json:"in"`
	Out string   `json:"out"`
}

// harnessOut is the JSON a language harness prints on stdout.
type harnessOut struct {
	BuildErr string `json:"build_err"`
	Cases    []struct {
		Index     int     `json:"index"`
		Status    string  `json:"status"`
		Actual    string  `json:"actual"`
		Stdout    string  `json:"stdout"`
		Err       string  `json:"err"`
		ElapsedMS float64 `json:"elapsed_ms"`
	} `json:"cases"`
}

// validateSpec returns a Result when the run should not be attempted, or an
// error when the Spec itself is unusable. ok=true means the Result/error is
// the final answer (caller should return it).
func validateSpec(spec Spec, toolchain string, available bool) (Result, error, bool) {
	if spec.Meta.Name == "" {
		return Result{BuildErr: designUnsupportedMsg}, nil, true
	}
	if len(spec.Cases) == 0 {
		return Result{}, fmt.Errorf("runner: no test cases"), true
	}
	if !available {
		return Result{}, fmt.Errorf("runner: %s not found on PATH", toolchain), true
	}
	return Result{}, nil, false
}

// judgeHarness maps raw harness output onto a Result, applying EqualOutputs.
func judgeHarness(spec Spec, elapsed time.Duration, out harnessOut) Result {
	if out.BuildErr != "" {
		return Result{BuildErr: out.BuildErr}
	}
	res := Result{Total: len(spec.Cases), Elapsed: elapsed}
	for i, hc := range out.Cases {
		cr := CaseResult{
			Index:   hc.Index,
			Actual:  hc.Actual,
			Stdout:  hc.Stdout,
			Err:     hc.Err,
			Elapsed: time.Duration(hc.ElapsedMS * float64(time.Millisecond)),
		}
		if i < len(spec.Cases) {
			cr.Input = spec.Cases[i].In
			cr.Expected = spec.Cases[i].Out
		}
		switch Status(hc.Status) {
		case StatusError, StatusTimeout:
			cr.Status = Status(hc.Status)
		default:
			cr.Status = judgeCase(cr.Expected, cr.Actual, spec.Meta)
			if cr.Status == StatusPass {
				res.Passed++
			}
		}
		res.Cases = append(res.Cases, cr)
	}
	return res
}

func judgeCase(expected, actual string, meta leetcode.Meta) Status {
	if strings.TrimSpace(expected) == "" {
		return StatusUnknown
	}
	if EqualOutputs(expected, actual, meta) {
		return StatusPass
	}
	return StatusFail
}

func marshalPayload(spec Spec, source string) ([]byte, error) {
	in := payload{Entry: spec.Meta.Name, Source: source}
	for _, c := range spec.Cases {
		in.Cases = append(in.Cases, payloadCase{In: c.In, Out: c.Out})
	}
	return json.Marshal(in)
}
