package leetcode

import "github.com/sven97/lazyleet/internal/testcase"

// Fixture returns a bundled sample Question by slug. It exists so the workspace
// UI (Tier C) can be built before the Phase 1 API client. Returns ok=false for
// an unknown slug.
func Fixture(slug string) (Question, bool) {
	q, ok := fixtures[slug]
	return q, ok
}

// FixtureSlugs lists the available bundled fixtures.
func FixtureSlugs() []string {
	out := make([]string, 0, len(fixtures))
	for s := range fixtures {
		out = append(out, s)
	}
	return out
}

var fixtures = map[string]Question{
	"two-sum": {
		FrontendID: 1,
		QuestionID: 1,
		Slug:       "two-sum",
		Title:      "Two Sum",
		Difficulty: "Easy",
		Statement: `Given an array of integers ` + "`nums`" + ` and an integer ` + "`target`" + `, return
*indices of the two numbers such that they add up to* ` + "`target`" + `.

You may assume that each input would have **exactly one solution**, and you
may not use the same element twice.

You can return the answer in any order.

## Example 1

` + "```" + `
Input:  nums = [2,7,11,15], target = 9
Output: [0,1]
Explanation: nums[0] + nums[1] == 9, so we return [0, 1].
` + "```" + `

## Example 2

` + "```" + `
Input:  nums = [3,2,4], target = 6
Output: [1,2]
` + "```" + `

## Example 3

` + "```" + `
Input:  nums = [3,3], target = 6
Output: [0,1]
` + "```" + `

## Constraints

- ` + "`2 <= nums.length <= 10^4`" + `
- ` + "`-10^9 <= nums[i] <= 10^9`" + `
- ` + "`-10^9 <= target <= 10^9`" + `
- **Only one valid answer exists.**
`,
		Meta: Meta{
			Name: "twoSum",
			Params: []MetaParam{
				{Name: "nums", Type: "integer[]"},
				{Name: "target", Type: "integer"},
			},
			Return: MetaType{Type: "integer[]"},
		},
		CodeSnippets: map[string]string{
			"python3": "class Solution:\n    def twoSum(self, nums: List[int], target: int) -> List[int]:\n        \n",
			"python":  "class Solution(object):\n    def twoSum(self, nums, target):\n        \"\"\"\n        :type nums: List[int]\n        :type target: int\n        :rtype: List[int]\n        \"\"\"\n        \n",
		},
		MetaData:         `{"name":"twoSum","params":[{"name":"nums","type":"integer[]"},{"name":"target","type":"integer"}],"return":{"type":"integer[]"}}`,
		ExampleTestcases: "[2,7,11,15]\n9\n[3,2,4]\n6\n[3,3]\n6",
		ExampleCases: []testcase.Case{
			{In: []string{"[2,7,11,15]", "9"}, Out: "[0,1]"},
			{In: []string{"[3,2,4]", "6"}, Out: "[1,2]"},
			{In: []string{"[3,3]", "6"}, Out: "[0,1]"},
		},
	},
}
