package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/testcase"
)

func twoSumMeta() leetcode.Meta {
	return leetcode.Meta{
		Name: "twoSum",
		Params: []leetcode.MetaParam{
			{Name: "nums", Type: "integer[]"},
			{Name: "target", Type: "integer"},
		},
		Return: leetcode.MetaType{Type: "integer[]"},
	}
}

func twoSumCases() []testcase.Case {
	return []testcase.Case{
		{In: []string{"[2,7,11,15]", "9"}, Out: "[0,1]"},
		{In: []string{"[3,2,4]", "6"}, Out: "[1,2]"},
		{In: []string{"[3,3]", "6"}, Out: "[0,1]"},
	}
}

func writeSolution(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "solution.py")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func requirePython(t *testing.T) pythonRunner {
	t.Helper()
	p := pythonRunner{bin: pythonBin("python3")}
	if !p.Available() {
		t.Skip("python3 not on PATH")
	}
	return p
}

func TestPythonRunnerAllPass(t *testing.T) {
	p := requirePython(t)
	sol := writeSolution(t, `
class Solution:
    def twoSum(self, nums, target):
        seen = {}
        for i, n in enumerate(nums):
            if target - n in seen:
                return [seen[target - n], i]
            seen[n] = i
`)
	res, err := p.Run(context.Background(), Spec{
		Lang: "python3", SolutionPath: sol, Meta: twoSumMeta(), Cases: twoSumCases(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK() {
		t.Fatalf("expected 3/3 pass, got %d/%d: %+v", res.Passed, res.Total, res.Cases)
	}
}

func TestPythonRunnerReportsWrongAnswer(t *testing.T) {
	p := requirePython(t)
	sol := writeSolution(t, `
class Solution:
    def twoSum(self, nums, target):
        return [0, 0]
`)
	res, err := p.Run(context.Background(), Spec{
		Lang: "python3", SolutionPath: sol, Meta: twoSumMeta(), Cases: twoSumCases(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.OK() || res.Passed != 0 {
		t.Fatalf("expected all fail, got %d/%d", res.Passed, res.Total)
	}
	if res.Cases[0].Status != StatusFail || res.Cases[0].Actual != "[0, 0]" {
		t.Fatalf("case 0 = %+v, want fail with actual [0, 0]", res.Cases[0])
	}
}

func TestPythonRunnerReportsRuntimeError(t *testing.T) {
	p := requirePython(t)
	sol := writeSolution(t, `
class Solution:
    def twoSum(self, nums, target):
        return nums[99]
`)
	res, err := p.Run(context.Background(), Spec{
		Lang: "python3", SolutionPath: sol, Meta: twoSumMeta(), Cases: twoSumCases()[:1],
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Cases[0].Status != StatusError || res.Cases[0].Err == "" {
		t.Fatalf("case 0 = %+v, want error status with a message", res.Cases[0])
	}
}

func TestPythonRunnerReportsBuildError(t *testing.T) {
	p := requirePython(t)
	sol := writeSolution(t, "class Solution:\n    def twoSum(self, nums, target)\n        return []\n") // missing colon
	res, err := p.Run(context.Background(), Spec{
		Lang: "python3", SolutionPath: sol, Meta: twoSumMeta(), Cases: twoSumCases(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.BuildErr == "" {
		t.Fatal("expected BuildErr for a syntax error")
	}
	if len(res.Cases) != 0 {
		t.Fatalf("expected no case results on build error, got %d", len(res.Cases))
	}
}

func TestPythonRunnerUnknownWhenNoExpected(t *testing.T) {
	p := requirePython(t)
	sol := writeSolution(t, `
class Solution:
    def twoSum(self, nums, target):
        return [0, 1]
`)
	res, err := p.Run(context.Background(), Spec{
		Lang: "python3", SolutionPath: sol, Meta: twoSumMeta(),
		Cases: []testcase.Case{{In: []string{"[2,7,11,15]", "9"}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Cases[0].Status != StatusUnknown {
		t.Fatalf("status = %q, want unknown", res.Cases[0].Status)
	}
}

func TestForUnknownLanguage(t *testing.T) {
	if _, ok := For("brainfuck"); ok {
		t.Fatal("For should not claim to handle brainfuck")
	}
}
