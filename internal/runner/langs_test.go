package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/testcase"
)

func writeLangSolution(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireRunner(t *testing.T, lang string) Runner {
	t.Helper()
	r, ok := For(lang)
	if !ok {
		t.Fatalf("For(%q) not implemented", lang)
	}
	if !r.Available() {
		t.Skipf("%s toolchain not on PATH", lang)
	}
	return r
}

func TestJavaScriptRunnerTwoSum(t *testing.T) {
	r := requireRunner(t, "javascript")
	sol := writeLangSolution(t, "solution.js", `
var twoSum = function(nums, target) {
  const seen = new Map();
  for (let i = 0; i < nums.length; i++) {
    if (seen.has(target - nums[i])) return [seen.get(target - nums[i]), i];
    seen.set(nums[i], i);
  }
};
`)
	res, err := r.Run(context.Background(), Spec{
		Lang: "javascript", SolutionPath: sol, Meta: twoSumMeta(), Cases: twoSumCases(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK() {
		t.Fatalf("expected 3/3 pass, got %d/%d build=%q cases=%+v", res.Passed, res.Total, res.BuildErr, res.Cases)
	}
}

func TestGolangRunnerTwoSum(t *testing.T) {
	r := requireRunner(t, "golang")
	sol := writeLangSolution(t, "solution.go", `
func twoSum(nums []int, target int) []int {
	seen := map[int]int{}
	for i, n := range nums {
		if j, ok := seen[target-n]; ok {
			return []int{j, i}
		}
		seen[n] = i
	}
	return nil
}
`)
	res, err := r.Run(context.Background(), Spec{
		Lang: "golang", SolutionPath: sol, Meta: twoSumMeta(), Cases: twoSumCases(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK() {
		t.Fatalf("expected 3/3 pass, got %d/%d build=%q cases=%+v", res.Passed, res.Total, res.BuildErr, res.Cases)
	}
}

func TestJavaRunnerTwoSum(t *testing.T) {
	r := requireRunner(t, "java")
	sol := writeLangSolution(t, "Solution.java", `
class Solution {
    public int[] twoSum(int[] nums, int target) {
        java.util.Map<Integer,Integer> seen = new java.util.HashMap<>();
        for (int i = 0; i < nums.length; i++) {
            if (seen.containsKey(target - nums[i])) {
                return new int[]{seen.get(target - nums[i]), i};
            }
            seen.put(nums[i], i);
        }
        return new int[]{};
    }
}
`)
	res, err := r.Run(context.Background(), Spec{
		Lang: "java", SolutionPath: sol, Meta: twoSumMeta(), Cases: twoSumCases(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK() {
		t.Fatalf("expected 3/3 pass, got %d/%d build=%q cases=%+v", res.Passed, res.Total, res.BuildErr, res.Cases)
	}
}

func TestCppRunnerTwoSum(t *testing.T) {
	r := requireRunner(t, "cpp")
	sol := writeLangSolution(t, "solution.cpp", `
class Solution {
public:
    vector<int> twoSum(vector<int>& nums, int target) {
        unordered_map<int,int> seen;
        for (int i = 0; i < (int)nums.size(); i++) {
            if (seen.count(target - nums[i])) return {seen[target - nums[i]], i};
            seen[nums[i]] = i;
        }
        return {};
    }
};
`)
	res, err := r.Run(context.Background(), Spec{
		Lang: "cpp", SolutionPath: sol, Meta: twoSumMeta(), Cases: twoSumCases(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK() {
		t.Fatalf("expected 3/3 pass, got %d/%d build=%q cases=%+v", res.Passed, res.Total, res.BuildErr, res.Cases)
	}
}

func TestDesignProblemUnsupported(t *testing.T) {
	r := requireRunner(t, "python3")
	sol := writeLangSolution(t, "solution.py", "class Solution:\n    pass\n")
	res, err := r.Run(context.Background(), Spec{
		Lang: "python3", SolutionPath: sol,
		Meta:  leetcode.Meta{}, // empty Name => design
		Cases: []testcase.Case{{In: []string{"1"}}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.BuildErr == "" || !strings.Contains(res.BuildErr, "design problems") {
		t.Fatalf("BuildErr = %q, want design-problems message", res.BuildErr)
	}
	if len(res.Cases) != 0 {
		t.Fatalf("expected no case results, got %d", len(res.Cases))
	}
}

func TestForLanguages(t *testing.T) {
	for _, lang := range []string{"python3", "javascript", "golang", "java", "cpp"} {
		if _, ok := For(lang); !ok {
			t.Errorf("For(%q) should be implemented", lang)
		}
	}
}

func TestOrderInsensitiveViaPython(t *testing.T) {
	r := requireRunner(t, "python3")
	sol := writeLangSolution(t, "solution.py", `
class Solution:
    def twoSum(self, nums, target):
        return [1, 0]  # reversed vs expected [0,1]
`)
	res, err := r.Run(context.Background(), Spec{
		Lang: "python3", SolutionPath: sol, Meta: twoSumMeta(),
		Cases: []testcase.Case{{In: []string{"[2,7,11,15]", "9"}, Out: "[0,1]"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK() {
		t.Fatalf("order-insensitive compare should pass, got %+v", res.Cases)
	}
}
