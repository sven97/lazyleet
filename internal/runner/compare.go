package runner

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/sven97/lazyleet/internal/leetcode"
)

// floatTol is the absolute/relative tolerance used for floating comparisons,
// matching LeetCode's typical 1e-5 allowance.
const floatTol = 1e-5

// EqualOutputs reports whether actual matches expected. Comparison rules:
//
//   - literals are JSON-decoded when possible; otherwise trimmed strings must match
//   - numbers compare with float tolerance (covers both ints-as-floats and floats)
//   - arrays compare element-by-element, in order
//
// meta is accepted for callers' convenience (and future per-problem rules)
// but unused today: LeetCode's metadata doesn't tell us which problems
// accept "any order" answers (e.g. Subsets, Combination Sum), so we judge
// order-sensitively across the board. A correct order-insensitive answer may
// show as a local fail, but we never want a wrong-order answer to show as a
// local pass when the real judge would reject it. Multiple distinct valid
// answers that are not identical are not yet modeled; use a single canonical
// expected output in testcases.jsonl.
func EqualOutputs(expected, actual string, meta leetcode.Meta) bool {
	ev, eOK := decodeLiteral(expected)
	av, aOK := decodeLiteral(actual)
	if !eOK || !aOK {
		return strings.TrimSpace(expected) == strings.TrimSpace(actual)
	}
	return deepEqual(ev, av)
}

func decodeLiteral(s string) (any, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s, true
	}
	return v, true
}

func deepEqual(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		return numEqual(av, b)
	case json.Number:
		f, err := av.Float64()
		if err != nil {
			return false
		}
		return numEqual(f, b)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqual(v, bv[k]) {
				return false
			}
		}
		return true
	default:
		ab, _ := json.Marshal(a)
		bb, _ := json.Marshal(b)
		return string(ab) == string(bb)
	}
}

func numEqual(a float64, b any) bool {
	bf, ok := asFloat(b)
	if !ok {
		return false
	}
	diff := math.Abs(a - bf)
	if diff <= floatTol {
		return true
	}
	scale := math.Max(math.Abs(a), math.Abs(bf))
	return diff <= floatTol*math.Max(1, scale)
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
