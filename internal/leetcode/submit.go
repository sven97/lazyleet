package leetcode

import (
	"context"
	"fmt"
	"time"
)

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
