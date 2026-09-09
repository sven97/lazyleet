package leetcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// flexStrings decodes a field LeetCode sends as either a JSON array of strings
// (interpret / Run Code) or a bare string, often "" (submission check).
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s != "" {
			*f = flexStrings{s}
		}
		return nil
	}
	var ss []string
	if err := json.Unmarshal(b, &ss); err != nil {
		return err
	}
	*f = ss
	return nil
}

// flexInt decodes an int that LeetCode may send as a number, a quoted number,
// or null.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(bytes.Trim(b, `"`))
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	n, err := strconv.Atoi(string(b))
	if err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

// These endpoints require authentication. They are the client half of Phase 6;
// the workspace UI wires them later.

// InterpretResult identifies a pending "run against custom input" job.
type InterpretResult struct {
	InterpretID string `json:"interpret_id"`
	TestCase    string `json:"test_case"`
}

// SubmitResult identifies a pending submission.
type SubmitResult struct {
	SubmissionID int64 `json:"submission_id"`
}

// JudgeResult is the terminal state of a run or submission check.
type JudgeResult struct {
	State             string   `json:"state"` // PENDING | STARTED | SUCCESS
	StatusMsg         string   `json:"status_msg"`
	RunSuccess        bool     `json:"run_success"`
	CorrectAnswer     bool     `json:"correct_answer"` // set by interpret (Run Code)
	TotalCorrect      int      `json:"total_correct"`
	TotalTestcases    int      `json:"total_testcases"`
	StatusRuntime     string   `json:"status_runtime"`
	StatusMemory      string   `json:"status_memory"`
	RuntimePercentile float64  `json:"runtime_percentile"`
	MemoryPercentile  float64  `json:"memory_percentile"`
	CodeAnswer        []string `json:"code_answer"`
	ExpectedAnswer    []string `json:"expected_code_answer"`
	CompareResult     string   `json:"compare_result"`
	LastTestcase      string   `json:"last_testcase"`
	CodeOutput        []string `json:"code_output"`
	FullCompileError  string   `json:"full_compile_error"`
	FullRuntimeError  string   `json:"full_runtime_error"`
}

// UnmarshalJSON tolerates LeetCode's shifting types: code_answer /
// expected_code_answer / code_output arrive as a []string from interpret but as
// a bare (often empty) string from a submission check; total_correct /
// total_testcases occasionally arrive quoted or null.
func (r *JudgeResult) UnmarshalJSON(b []byte) error {
	type alias struct {
		State             string      `json:"state"`
		StatusMsg         string      `json:"status_msg"`
		RunSuccess        bool        `json:"run_success"`
		CorrectAnswer     bool        `json:"correct_answer"`
		TotalCorrect      flexInt     `json:"total_correct"`
		TotalTestcases    flexInt     `json:"total_testcases"`
		StatusRuntime     string      `json:"status_runtime"`
		StatusMemory      string      `json:"status_memory"`
		RuntimePercentile float64     `json:"runtime_percentile"`
		MemoryPercentile  float64     `json:"memory_percentile"`
		CodeAnswer        flexStrings `json:"code_answer"`
		ExpectedAnswer    flexStrings `json:"expected_code_answer"`
		CompareResult     string      `json:"compare_result"`
		LastTestcase      string      `json:"last_testcase"`
		CodeOutput        flexStrings `json:"code_output"`
		FullCompileError  string      `json:"full_compile_error"`
		FullRuntimeError  string      `json:"full_runtime_error"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*r = JudgeResult{
		State: a.State, StatusMsg: a.StatusMsg, RunSuccess: a.RunSuccess,
		CorrectAnswer: a.CorrectAnswer,
		TotalCorrect:  int(a.TotalCorrect), TotalTestcases: int(a.TotalTestcases),
		StatusRuntime: a.StatusRuntime, StatusMemory: a.StatusMemory,
		RuntimePercentile: a.RuntimePercentile, MemoryPercentile: a.MemoryPercentile,
		CodeAnswer: a.CodeAnswer, ExpectedAnswer: a.ExpectedAnswer,
		CompareResult: a.CompareResult, LastTestcase: a.LastTestcase,
		CodeOutput:       a.CodeOutput,
		FullCompileError: a.FullCompileError, FullRuntimeError: a.FullRuntimeError,
	}
	return nil
}

// Accepted reports whether a submission passed.
func (r JudgeResult) Accepted() bool { return r.StatusMsg == "Accepted" }

// Done reports whether the judge has finished.
func (r JudgeResult) Done() bool { return r.State == "SUCCESS" }

func (c *Client) problemReferer(slug string) string {
	return c.site.base + "/problems/" + slug + "/"
}

// Interpret runs typed_code against dataInput (LeetCode's "Run Code").
func (c *Client) Interpret(ctx context.Context, slug string, questionID int, lang, code, dataInput string) (InterpretResult, error) {
	if !c.Authenticated() {
		return InterpretResult{}, &APIError{Op: "interpret_solution", Message: "not authenticated (run `lazyleet auth`)"}
	}
	payload := map[string]any{
		"lang":        lang,
		"question_id": fmt.Sprint(questionID),
		"typed_code":  code,
		"data_input":  dataInput,
	}
	var out InterpretResult
	err := c.postJSON(ctx, "interpret_solution", "/problems/"+slug+"/interpret_solution/", payload, &out, c.problemReferer(slug))
	return out, err
}

// Submit submits typed_code for full judging.
func (c *Client) Submit(ctx context.Context, slug string, questionID int, lang, code string) (SubmitResult, error) {
	if !c.Authenticated() {
		return SubmitResult{}, &APIError{Op: "submit", Message: "not authenticated (run `lazyleet auth`)"}
	}
	payload := map[string]any{
		"lang":        lang,
		"question_id": fmt.Sprint(questionID),
		"typed_code":  code,
	}
	var out SubmitResult
	err := c.postJSON(ctx, "submit", "/problems/"+slug+"/submit/", payload, &out, c.problemReferer(slug))
	return out, err
}

// CheckResult fetches the current state of a run/submission by its id
// (interpret_id or submission_id).
func (c *Client) CheckResult(ctx context.Context, slug, id string) (JudgeResult, error) {
	var out JudgeResult
	err := c.get(ctx, "check", "/submissions/detail/"+id+"/check/", &out, c.problemReferer(slug))
	return out, err
}

// PollResult polls CheckResult until the judge finishes, ctx is cancelled, or
// the deadline passes. It calls onTick (if non-nil) with each interim state.
func (c *Client) PollResult(ctx context.Context, slug, id string, onTick func(JudgeResult)) (JudgeResult, error) {
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	for {
		res, err := c.CheckResult(ctx, slug, id)
		if err != nil {
			return JudgeResult{}, err
		}
		if res.Done() {
			return res, nil
		}
		if onTick != nil {
			onTick(res)
		}
		select {
		case <-ctx.Done():
			return JudgeResult{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
