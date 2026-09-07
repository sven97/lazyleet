package testcase

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestParseSkipsBlanksAndComments(t *testing.T) {
	in := `
# a comment
{"in": ["[2,7,11,15]", "9"], "out": "[0,1]"}

# another
{"in": ["[3,3]", "6"]}
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Case{
		{In: []string{"[2,7,11,15]", "9"}, Out: "[0,1]"},
		{In: []string{"[3,3]", "6"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWriteParseRoundTrip(t *testing.T) {
	cases := []Case{
		{In: []string{`"abc"`, "2"}, Out: `"ac"`},
		{In: []string{"[1,2,3]"}},
	}
	var buf bytes.Buffer
	if err := Write(&buf, cases); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Parse(&buf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(got, cases) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, cases)
	}
}

func TestParseRejectsCaseWithoutIn(t *testing.T) {
	if _, err := Parse(strings.NewReader(`{"out": "1"}`)); err == nil {
		t.Fatal("expected error for a case with no \"in\"")
	}
}

func TestFromLeetCodeExample(t *testing.T) {
	block := "[2,7,11,15]\n9\n[3,2,4]\n6\n[3,3]\n6\n"
	got, err := FromLeetCodeExample(block, 2)
	if err != nil {
		t.Fatalf("FromLeetCodeExample: %v", err)
	}
	want := []Case{
		{In: []string{"[2,7,11,15]", "9"}},
		{In: []string{"[3,2,4]", "6"}},
		{In: []string{"[3,3]", "6"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestFromLeetCodeExampleWrongArity(t *testing.T) {
	if _, err := FromLeetCodeExample("a\nb\nc\n", 2); err == nil {
		t.Fatal("expected error when line count is not a multiple of arity")
	}
}
