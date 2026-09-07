// Package testcase defines lazyleet's on-disk test-case format and helpers to
// read, write, and derive cases. A case is a list of raw argument literals
// (one per function parameter, in order) plus an optional expected-result
// literal. Literals are stored verbatim as they appear on LeetCode
// (e.g. "[2,7,11,15]", "9", "\"abc\"").
//
// The file format is JSON Lines: one JSON object per line. Blank lines and
// lines beginning with '#' are ignored, so users can annotate the file.
package testcase

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Case is a single test case.
type Case struct {
	In  []string `json:"in"`            // raw argument literals, in parameter order
	Out string   `json:"out,omitempty"` // expected result literal; "" means unknown
}

// Parse reads JSON-Lines cases from r.
func Parse(r io.Reader) ([]Case, error) {
	var cases []Case
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		var c Case
		if err := json.Unmarshal([]byte(text), &c); err != nil {
			return nil, fmt.Errorf("testcase line %d: %w", line, err)
		}
		if len(c.In) == 0 {
			return nil, fmt.Errorf("testcase line %d: %q has no \"in\" values", line, text)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return cases, nil
}

// Write renders cases as JSON Lines, with a short header comment.
func Write(w io.Writer, cases []Case) error {
	var b bytes.Buffer
	b.WriteString("# lazyleet test cases — one JSON object per line.\n")
	b.WriteString("#   {\"in\": [\"<arg1>\", \"<arg2>\"], \"out\": \"<expected>\"}\n")
	b.WriteString("# \"out\" is optional; leave it off if you don't know the answer yet.\n")
	for _, c := range cases {
		enc, err := json.Marshal(c)
		if err != nil {
			return err
		}
		b.Write(enc)
		b.WriteByte('\n')
	}
	_, err := w.Write(b.Bytes())
	return err
}

// FromLeetCodeExample splits a raw LeetCode `exampleTestcases` block into cases.
// That block is arity lines per case (one literal per parameter), with no
// expected outputs — so every returned case has an empty Out.
func FromLeetCodeExample(block string, arity int) ([]Case, error) {
	if arity <= 0 {
		return nil, fmt.Errorf("arity must be > 0, got %d", arity)
	}
	var lines []string
	for _, ln := range strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n") {
		if s := strings.TrimSpace(ln); s != "" {
			lines = append(lines, s)
		}
	}
	if len(lines)%arity != 0 {
		return nil, fmt.Errorf("example block has %d non-empty lines, not a multiple of arity %d", len(lines), arity)
	}
	var cases []Case
	for i := 0; i < len(lines); i += arity {
		in := make([]string, arity)
		copy(in, lines[i:i+arity])
		cases = append(cases, Case{In: in})
	}
	return cases, nil
}
