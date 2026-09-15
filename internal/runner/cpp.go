package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/testcase"
)

type cppRunner struct{}

func (cppRunner) Available() bool {
	_, err := exec.LookPath("g++")
	return err == nil
}

func (c cppRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if res, err, done := validateSpec(spec, "g++", c.Available()); done {
		return res, err
	}
	src, err := os.ReadFile(spec.SolutionPath)
	if err != nil {
		return Result{}, fmt.Errorf("runner: read solution: %w", err)
	}
	harness, err := buildCppHarness(spec.Meta, string(src), spec.Cases)
	if err != nil {
		return Result{BuildErr: err.Error()}, nil
	}

	return runCompiled(ctx, spec, "lazyleet-cpp-*",
		func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "main.cpp"), []byte(harness), 0o644)
		},
		func(ctx context.Context, dir string) *exec.Cmd {
			return exec.CommandContext(ctx, "g++", "-std=c++17", "-O2",
				"-o", filepath.Join(dir, "solution"), filepath.Join(dir, "main.cpp"))
		},
		func(ctx context.Context, dir string) *exec.Cmd {
			return exec.CommandContext(ctx, filepath.Join(dir, "solution"))
		},
	)
}

func buildCppHarness(meta leetcode.Meta, userSrc string, cases []testcase.Case) (string, error) {
	retType, err := lcToCppType(meta.Return.Type)
	if err != nil {
		return "", err
	}
	paramTypes := make([]string, len(meta.Params))
	for i, p := range meta.Params {
		paramTypes[i], err = lcToCppType(p.Type)
		if err != nil {
			return "", err
		}
	}

	var b strings.Builder
	b.WriteString(cppStdIncludes)
	b.WriteString(userSrc)
	b.WriteString("\n\n")
	b.WriteString(cppJSONHelpers)
	b.WriteString("int main() {\n")
	b.WriteString("  cout << \"{\\\"build_err\\\":\\\"\\\",\\\"cases\\\":[\";\n")
	b.WriteString("  bool first = true;\n")
	b.WriteString("  Solution sol;\n")

	for i, c := range cases {
		b.WriteString("  {\n")
		b.WriteString("    if (!first) cout << \",\"; first = false;\n")
		b.WriteString("    auto t0 = chrono::steady_clock::now();\n")
		b.WriteString(fmt.Sprintf("    int index = %d;\n", i))
		b.WriteString("    string status = \"ran\"; string actual; string err;\n")
		b.WriteString("    ostringstream outCapture;\n")
		b.WriteString("    streambuf* origCoutBuf = cout.rdbuf(outCapture.rdbuf());\n")
		b.WriteString("    try {\n")
		args := make([]string, len(meta.Params))
		for pi := range meta.Params {
			if pi >= len(c.In) {
				return "", fmt.Errorf("case %d: missing arg %d", i, pi)
			}
			expr, err := cppLiteral(paramTypes[pi], c.In[pi])
			if err != nil {
				return "", err
			}
			// vector params in LC are often non-const refs; make a named variable
			b.WriteString(fmt.Sprintf("      %s arg%d = %s;\n", paramTypes[pi], pi, expr))
			args[pi] = fmt.Sprintf("arg%d", pi)
		}
		b.WriteString(fmt.Sprintf("      %s got = sol.%s(%s);\n", retType, meta.Name, strings.Join(args, ", ")))
		b.WriteString("      actual = to_json(got);\n")
		b.WriteString("    } catch (const exception& e) {\n")
		b.WriteString("      status = \"error\"; err = e.what();\n")
		b.WriteString("    } catch (...) {\n")
		b.WriteString("      status = \"error\"; err = \"unknown error\";\n")
		b.WriteString("    }\n")
		b.WriteString("    cout.rdbuf(origCoutBuf);\n")
		b.WriteString("    string capturedStdout = outCapture.str();\n")
		b.WriteString("    double ms = chrono::duration<double, milli>(chrono::steady_clock::now() - t0).count();\n")
		b.WriteString("    cout << \"{\\\"index\\\":\" << index")
		b.WriteString(" << \",\\\"status\\\":\\\"\" << status << \"\\\"\"")
		b.WriteString(" << \",\\\"actual\\\":\" << json_string(actual)")
		b.WriteString(" << \",\\\"stdout\\\":\" << json_string(capturedStdout)")
		b.WriteString(" << \",\\\"err\\\":\" << json_string(err)")
		b.WriteString(" << \",\\\"elapsed_ms\\\":\" << ms << \"}\";\n")
		b.WriteString("  }\n")
	}
	b.WriteString("  cout << \"]}\";\n")
	b.WriteString("  return 0;\n}\n")
	return b.String(), nil
}

func cppLiteral(cppType, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	ct := strings.TrimSpace(cppType)
	switch {
	case ct == "char":
		return cppCharLiteral(raw)
	case ct == "int" || ct == "long long" || ct == "double" || ct == "bool":
		if ct == "long long" && !strings.Contains(strings.ToLower(raw), "ll") {
			return raw + "LL", nil
		}
		return raw, nil
	case ct == "string":
		if strings.HasPrefix(raw, "\"") {
			return raw, nil
		}
		b, _ := json.Marshal(raw)
		return "string(" + string(b) + ")", nil
	case strings.HasPrefix(ct, "vector<"):
		return cppVectorLiteral(ct, raw)
	default:
		return "", fmt.Errorf("cannot build C++ literal for type %s", cppType)
	}
}

// cppCharLiteral turns the JSON string value LeetCode uses for a `character`
// test-case argument (e.g. `"a"`) into a C++ char literal (e.g. 'a').
func cppCharLiteral(raw string) (string, error) {
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", fmt.Errorf("parse char: %w", err)
	}
	rs := []rune(s)
	if len(rs) != 1 {
		return "", fmt.Errorf("expected single-character string for char literal, got %q", s)
	}
	var esc string
	switch rs[0] {
	case '\\':
		esc = `\\`
	case '\'':
		esc = `\'`
	case '\n':
		esc = `\n`
	case '\r':
		esc = `\r`
	case '\t':
		esc = `\t`
	default:
		esc = string(rs[0])
	}
	return "'" + esc + "'", nil
}

func cppVectorLiteral(cppType, raw string) (string, error) {
	inner := cppType[len("vector<") : len(cppType)-1]
	// handle nested vector<vector<int>> — find matching close
	if strings.Count(cppType, "vector<") > 1 {
		inner = cppType[len("vector<") : len(cppType)-1]
	}
	var vals []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &vals); err != nil {
		return "", err
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		expr, err := cppLiteral(inner, string(v))
		if err != nil {
			return "", err
		}
		parts[i] = expr
	}
	return fmt.Sprintf("%s{%s}", cppType, strings.Join(parts, ",")), nil
}

// cppStdIncludes replaces <bits/stdc++.h>, a GCC/libstdc++-only umbrella
// header LeetCode's own judge can get away with but a local build can't
// portably rely on — it doesn't exist under Clang/libc++ (the default on
// macOS, where it fails with "file not found") or MSVC. This explicit list
// covers what LeetCode-style solutions commonly reach for.
const cppStdIncludes = `#include <algorithm>
#include <bitset>
#include <chrono>
#include <cmath>
#include <cstdint>
#include <deque>
#include <functional>
#include <iostream>
#include <limits>
#include <list>
#include <map>
#include <numeric>
#include <queue>
#include <set>
#include <sstream>
#include <stack>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <utility>
#include <vector>
using namespace std;

`

const cppJSONHelpers = `
static string json_escape(const string& s) {
  string o; o.reserve(s.size()+8);
  for (char c : s) {
    switch (c) {
      case '\\': o += "\\\\"; break;
      case '"': o += "\\\""; break;
      case '\n': o += "\\n"; break;
      case '\r': o += "\\r"; break;
      case '\t': o += "\\t"; break;
      default: o += c;
    }
  }
  return o;
}
static string json_string(const string& s) { return "\"" + json_escape(s) + "\""; }
static string to_json(int v) { return to_string(v); }
static string to_json(long long v) { return to_string(v); }
static string to_json(double v) { ostringstream o; o << v; return o.str(); }
static string to_json(bool v) { return v ? "true" : "false"; }
static string to_json(const string& v) { return json_string(v); }
static string to_json(char v) { return json_string(string(1, v)); }
template <typename T>
static string to_json(const vector<T>& v) {
  string o = "[";
  for (size_t i = 0; i < v.size(); i++) { if (i) o += ","; o += to_json(v[i]); }
  return o + "]";
}
`
