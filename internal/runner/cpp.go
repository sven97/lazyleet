package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

	dir, err := os.MkdirTemp("", "lazyleet-cpp-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)

	srcPath := filepath.Join(dir, "main.cpp")
	if err := os.WriteFile(srcPath, []byte(harness), 0o644); err != nil {
		return Result{}, err
	}
	binPath := filepath.Join(dir, "solution")

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	build := exec.CommandContext(runCtx, "g++", "-std=c++17", "-O2", "-o", binPath, srcPath)
	var buildOut bytes.Buffer
	build.Stderr = &buildOut
	build.Stdout = &buildOut
	if err := build.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return timedOut(spec, timeout), nil
		}
		msg := strings.TrimSpace(buildOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return Result{BuildErr: msg}, nil
	}

	cmd := exec.CommandContext(runCtx, binPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)
	if runCtx.Err() == context.DeadlineExceeded {
		return timedOut(spec, elapsed), nil
	}
	if runErr != nil && stdout.Len() == 0 {
		return Result{BuildErr: trimErr(stderr.String(), runErr)}, nil
	}
	var out harnessOut
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return Result{BuildErr: fmt.Sprintf("runner: unreadable harness output: %v\n%s", err, stderr.String())}, nil
	}
	return judgeHarness(spec, elapsed, out), nil
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
	b.WriteString("#include <bits/stdc++.h>\nusing namespace std;\n\n")
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
		b.WriteString("    double ms = chrono::duration<double, milli>(chrono::steady_clock::now() - t0).count();\n")
		b.WriteString("    cout << \"{\\\"index\\\":\" << index")
		b.WriteString(" << \",\\\"status\\\":\\\"\" << status << \"\\\"\"")
		b.WriteString(" << \",\\\"actual\\\":\" << json_string(actual)")
		b.WriteString(" << \",\\\"stdout\\\":\\\"\\\"\"")
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
	case ct == "int" || ct == "long long" || ct == "double" || ct == "bool" || ct == "char":
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
