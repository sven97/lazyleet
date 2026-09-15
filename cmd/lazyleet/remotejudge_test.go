package main

import (
	"testing"

	"github.com/sven97/lazyleet/internal/leetcode"
)

func TestMapOutcomeInterpretUsesCodeAnswerArrays(t *testing.T) {
	res := leetcode.JudgeResult{
		StatusMsg: "Wrong Answer (sample)", TotalCorrect: 1, TotalTestcases: 2,
		CodeAnswer: []string{"1", "0"}, ExpectedAnswer: []string{"2", "0"},
		// A Run Code response shouldn't need CodeOutput/ExpectedOutput.
	}
	out := mapOutcome("run", res)
	if len(out.Expected) != 2 || out.Expected[0] != "2" || len(out.Actual) != 2 || out.Actual[0] != "1" {
		t.Fatalf("Expected/Actual = %+v/%+v, want the code_answer/expected_code_answer arrays", out.Expected, out.Actual)
	}
}

// TestMapOutcomeSubmitFallsBackToCodeOutput guards the fix for a submission
// check leaving code_answer/expected_code_answer empty (confirmed by this
// package's own tolerant-decode test, and by community reverse-engineering
// of LeetCode's undocumented API — e.g. clearloop/leetcode-cli's
// VerifyResult, which reads code_output[0]/expected_output[0] specifically
// for a submission's Wrong Answer branch, never the plural fields there).
func TestMapOutcomeSubmitFallsBackToCodeOutput(t *testing.T) {
	res := leetcode.JudgeResult{
		StatusMsg: "Wrong Answer", TotalCorrect: 5, TotalTestcases: 12,
		LastTestcase: "1002",
		// CodeAnswer/ExpectedAnswer empty, as a real submission check sends.
		CodeOutput:     []string{"9"},
		ExpectedOutput: []string{"4"},
	}
	out := mapOutcome("submit", res)
	if len(out.Expected) != 1 || out.Expected[0] != "4" {
		t.Errorf("Expected = %+v, want [\"4\"] from ExpectedOutput", out.Expected)
	}
	if len(out.Actual) != 1 || out.Actual[0] != "9" {
		t.Errorf("Actual = %+v, want [\"9\"] from CodeOutput", out.Actual)
	}
}
