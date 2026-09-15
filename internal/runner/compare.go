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

// EqualOutputs reports whether actual matches expected for a problem with the
// given meta. Comparison rules:
//
//   - literals are JSON-decoded when possible; otherwise trimmed strings must match
//   - numbers compare with float tolerance (covers both ints-as-floats and floats)
//   - array/list return types compare order-insensitively (multiset equality),
//     which covers "return in any order" problems such as Two Sum and subsets
//   - nested arrays apply the same rule at each level
//
// Multiple distinct valid answers that are not permutations of one another are
// not yet modeled; use a single canonical expected output in testcases.jsonl.
func EqualOutputs(expected, actual string, meta leetcode.Meta) bool {
	ev, eOK := decodeLiteral(expected)
	av, aOK := decodeLiteral(actual)
	if !eOK || !aOK {
		return strings.TrimSpace(expected) == strings.TrimSpace(actual)
	}
	orderInsensitive := isArrayLike(meta.Return.Type)
	return deepEqual(ev, av, orderInsensitive)
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

func isArrayLike(t string) bool {
	t = strings.TrimSpace(strings.ToLower(t))
	if t == "" {
		return false
	}
	return strings.Contains(t, "[]") ||
		strings.HasPrefix(t, "list<") ||
		strings.HasPrefix(t, "vector<") ||
		t == "list" || t == "array"
}

func deepEqual(a, b any, orderInsensitive bool) bool {
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
		if !ok {
			return false
		}
		if orderInsensitive {
			return equalMultisets(av, bv)
		}
		if len(av) != len(bv) {
			return false
		}
		for i := range av {
			// Nested arrays stay order-insensitive when the top-level return is
			// an array type (subsets / combinations).
			if !deepEqual(av[i], bv[i], orderInsensitive) {
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
			if !deepEqual(v, bv[k], orderInsensitive) {
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

func equalMultisets(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
	for _, x := range a {
		found := false
		for j, y := range b {
			if used[j] {
				continue
			}
			if deepEqual(x, y, true) {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
