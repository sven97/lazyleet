package leetcode

import "github.com/sven97/lazyleet/internal/testcase"

// Question is the full detail lazyleet needs to open a workspace for a problem.
// The Phase 1 API client will populate this from LeetCode's GraphQL; until then
// Fixture provides bundled samples so the workspace UI can be built and tested.
type Question struct {
	FrontendID   int     // the number shown to users (e.g. 1 for Two Sum)
	QuestionID   int     // LeetCode's internal id, needed for run/submit
	Slug         string  // URL slug, e.g. "two-sum"
	Title        string  // "Two Sum"
	Difficulty   string  // Easy | Medium | Hard
	ACRate       float64 // acceptance percentage, 0..100
	PaidOnly     bool
	Tags         []string          // topic tags (display names)
	Statement    string            // Markdown (converted from LeetCode HTML)
	Meta         Meta              // the entry point the judge calls
	CodeSnippets map[string]string // language slug -> starter code
	ExampleCases []testcase.Case   // worked examples derived from ExampleTestcases

	// Raw fields kept for caching and debugging.
	MetaData         string // LeetCode's metaData JSON string
	ExampleTestcases string // LeetCode's exampleTestcases block (N lines per case)
}

// Meta describes the function the solution must implement, derived from
// LeetCode's `metaData` JSON.
type Meta struct {
	Name   string      // function name, e.g. "twoSum"
	Params []MetaParam // ordered
	Return MetaType
	// Design problems (LRU Cache-style) will set Methods instead; Phase 5.
}

// MetaParam is one function parameter.
type MetaParam struct {
	Name string
	Type string // LeetCode type string: "integer[]", "integer", "string", "string[]", ...
}

// MetaType is a return type.
type MetaType struct {
	Type string
}

// Arity is the number of parameters the entry point takes.
func (m Meta) Arity() int { return len(m.Params) }
