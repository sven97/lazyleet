package leetcode

import (
	"encoding/json"
	"testing"
)

func TestJudgeResultTolerantDecode(t *testing.T) {
	// submission check: strings, not arrays; quoted counts
	const submit = `{"state":"SUCCESS","status_msg":"Wrong Answer","total_correct":"5","total_testcases":"12",
	  "code_answer":"","expected_code_answer":"","code_output":"","compare_result":"11111000000","last_testcase":"1002"}`
	var r JudgeResult
	if err := json.Unmarshal([]byte(submit), &r); err != nil {
		t.Fatalf("submit-shape decode: %v", err)
	}
	if r.TotalCorrect != 5 || r.TotalTestcases != 12 {
		t.Fatalf("counts: %d/%d", r.TotalCorrect, r.TotalTestcases)
	}
	if r.CodeOutput != nil || r.CodeAnswer != nil {
		t.Fatalf("empty strings should decode to nil slices: %#v %#v", r.CodeOutput, r.CodeAnswer)
	}

	// interpret: arrays
	const interp = `{"state":"SUCCESS","code_answer":["3"],"code_output":["dbg"],"expected_code_answer":["3"],"total_correct":1,"total_testcases":1}`
	var r2 JudgeResult
	if err := json.Unmarshal([]byte(interp), &r2); err != nil {
		t.Fatalf("interpret-shape decode: %v", err)
	}
	if len(r2.CodeAnswer) != 1 || r2.CodeAnswer[0] != "3" || len(r2.CodeOutput) != 1 {
		t.Fatalf("array decode: %#v", r2)
	}
}
