package runner

import (
	"testing"

	"github.com/sven97/lazyleet/internal/leetcode"
)

func TestEqualOutputsExact(t *testing.T) {
	meta := leetcode.Meta{Return: leetcode.MetaType{Type: "integer"}}
	if !EqualOutputs("3", "3", meta) {
		t.Fatal("exact int should match")
	}
	if EqualOutputs("3", "4", meta) {
		t.Fatal("different ints should not match")
	}
}

func TestEqualOutputsOrderInsensitiveArray(t *testing.T) {
	meta := leetcode.Meta{Return: leetcode.MetaType{Type: "integer[]"}}
	if !EqualOutputs("[0,1]", "[1,0]", meta) {
		t.Fatal("two-sum style any-order pair should match")
	}
	if EqualOutputs("[0,1]", "[0,2]", meta) {
		t.Fatal("different multiset should not match")
	}
}

func TestEqualOutputsNestedOrderInsensitive(t *testing.T) {
	meta := leetcode.Meta{Return: leetcode.MetaType{Type: "list<list<integer>>"}}
	if !EqualOutputs("[[1,2],[3]]", "[[3],[2,1]]", meta) {
		t.Fatal("subsets-style nested any-order should match")
	}
}

func TestEqualOutputsFloatTolerance(t *testing.T) {
	meta := leetcode.Meta{Return: leetcode.MetaType{Type: "double"}}
	if !EqualOutputs("0.5", "0.5000001", meta) {
		t.Fatal("floats within 1e-5 should match")
	}
	if EqualOutputs("0.5", "0.6", meta) {
		t.Fatal("floats outside tolerance should not match")
	}
}

func TestEqualOutputsOrderedNonArray(t *testing.T) {
	meta := leetcode.Meta{Return: leetcode.MetaType{Type: "string"}}
	if !EqualOutputs(`"ab"`, `"ab"`, meta) {
		t.Fatal("strings should match")
	}
	if EqualOutputs(`"ab"`, `"ba"`, meta) {
		t.Fatal("string order matters")
	}
}
