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

type golangRunner struct{}

func (golangRunner) Available() bool {
	_, err := exec.LookPath("go")
	return err == nil
}

func (g golangRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if res, err, done := validateSpec(spec, "go", g.Available()); done {
		return res, err
	}
	src, err := os.ReadFile(spec.SolutionPath)
	if err != nil {
		return Result{}, fmt.Errorf("runner: read solution: %w", err)
	}
	mainSrc, err := buildGoHarness(spec.Meta, string(src), spec.Cases)
	if err != nil {
		return Result{BuildErr: err.Error()}, nil
	}

	return runCompiled(ctx, spec, "lazyleet-go-*",
		func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644)
		},
		func(ctx context.Context, dir string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(dir, "solution"), filepath.Join(dir, "main.go"))
			cmd.Dir = dir
			return cmd
		},
		func(ctx context.Context, dir string) *exec.Cmd {
			return exec.CommandContext(ctx, filepath.Join(dir, "solution"))
		},
	)
}

func buildGoHarness(meta leetcode.Meta, userSrc string, cases []testcase.Case) (string, error) {
	userSrc = stripGoPackage(userSrc)
	retType, err := lcToGoType(meta.Return.Type)
	if err != nil {
		return "", err
	}
	paramTypes := make([]string, len(meta.Params))
	for i, p := range meta.Params {
		paramTypes[i], err = lcToGoType(p.Type)
		if err != nil {
			return "", err
		}
	}

	var b strings.Builder
	b.WriteString("package main\n")
	b.WriteString("import (\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"io\"\n\t\"os\"\n\t\"time\"\n)\n")
	b.WriteString(userSrc)
	b.WriteString("\n\n")
	// captureStdout swaps os.Stdout for a pipe while fn runs, so any
	// fmt.Println in the user's solution doesn't corrupt the single JSON
	// result blob this harness writes to the real stdout at the end.
	b.WriteString("func captureStdout(fn func()) (out string) {\n")
	b.WriteString("\told := os.Stdout\n")
	b.WriteString("\tr, w, perr := os.Pipe()\n")
	b.WriteString("\tif perr != nil {\n\t\tfn()\n\t\treturn \"\"\n\t}\n")
	b.WriteString("\tos.Stdout = w\n")
	b.WriteString("\tdone := make(chan string, 1)\n")
	b.WriteString("\tgo func() {\n\t\tbuf, _ := io.ReadAll(r)\n\t\tdone <- string(buf)\n\t}()\n")
	b.WriteString("\tdefer func() {\n\t\tos.Stdout = old\n\t\tw.Close()\n\t\tout = <-done\n\t}()\n")
	b.WriteString("\tfn()\n")
	b.WriteString("\treturn\n")
	b.WriteString("}\n\n")
	b.WriteString("func main() {\n")
	b.WriteString("\ttype caseOut struct {\n")
	b.WriteString("\t\tIndex int `json:\"index\"`\n")
	b.WriteString("\t\tStatus string `json:\"status\"`\n")
	b.WriteString("\t\tActual string `json:\"actual\"`\n")
	b.WriteString("\t\tStdout string `json:\"stdout\"`\n")
	b.WriteString("\t\tErr string `json:\"err\"`\n")
	b.WriteString("\t\tElapsedMS float64 `json:\"elapsed_ms\"`\n")
	b.WriteString("\t}\n")
	b.WriteString("\tresults := make([]caseOut, 0)\n")

	for i, c := range cases {
		b.WriteString(fmt.Sprintf("\t{\n\t\tco := caseOut{Index: %d}\n\t\tt0 := time.Now()\n", i))
		b.WriteString("\t\tfunc() {\n\t\t\tdefer func() {\n")
		b.WriteString("\t\t\t\tif r := recover(); r != nil {\n")
		b.WriteString("\t\t\t\t\tco.Status = \"error\"\n")
		b.WriteString("\t\t\t\t\tco.Err = fmt.Sprint(r)\n")
		b.WriteString("\t\t\t\t\tco.ElapsedMS = float64(time.Since(t0).Microseconds()) / 1000.0\n")
		b.WriteString("\t\t\t\t\tresults = append(results, co)\n")
		b.WriteString("\t\t\t\t}\n\t\t\t}()\n")
		for pi := range meta.Params {
			if pi >= len(c.In) {
				return "", fmt.Errorf("case %d: missing arg %d", i, pi)
			}
			lit, err := quoteJSONString(c.In[pi])
			if err != nil {
				return "", err
			}
			// LeetCode encodes a `character` test-case value as a one-rune JSON
			// string (e.g. "a"). Go's json package cannot unmarshal a JSON
			// string directly into a numeric type (byte/[]byte), so decode into
			// a string first and pull the byte(s) out of it.
			ptype := normalizeLCType(meta.Params[pi].Type)
			switch ptype {
			case "character", "char":
				b.WriteString(fmt.Sprintf("\t\t\tvar arg%dStr string\n", pi))
				b.WriteString(fmt.Sprintf("\t\t\tif err := json.Unmarshal([]byte(%s), &arg%dStr); err != nil { co.Status = \"error\"; co.Err = err.Error(); co.ElapsedMS = float64(time.Since(t0).Microseconds()) / 1000.0; results = append(results, co); return }\n", lit, pi))
				b.WriteString(fmt.Sprintf("\t\t\targ%d := arg%dStr[0]\n", pi, pi))
			case "character[]", "char[]":
				b.WriteString(fmt.Sprintf("\t\t\tvar arg%dStrs []string\n", pi))
				b.WriteString(fmt.Sprintf("\t\t\tif err := json.Unmarshal([]byte(%s), &arg%dStrs); err != nil { co.Status = \"error\"; co.Err = err.Error(); co.ElapsedMS = float64(time.Since(t0).Microseconds()) / 1000.0; results = append(results, co); return }\n", lit, pi))
				b.WriteString(fmt.Sprintf("\t\t\targ%d := make([]byte, len(arg%dStrs))\n", pi, pi))
				b.WriteString(fmt.Sprintf("\t\t\tfor _i, _s := range arg%dStrs { arg%d[_i] = _s[0] }\n", pi, pi))
			default:
				b.WriteString(fmt.Sprintf("\t\t\tvar arg%d %s\n", pi, paramTypes[pi]))
				b.WriteString(fmt.Sprintf("\t\t\tif err := json.Unmarshal([]byte(%s), &arg%d); err != nil { co.Status = \"error\"; co.Err = err.Error(); co.ElapsedMS = float64(time.Since(t0).Microseconds()) / 1000.0; results = append(results, co); return }\n", lit, pi))
			}
		}
		args := make([]string, len(meta.Params))
		for pi := range meta.Params {
			args[pi] = fmt.Sprintf("arg%d", pi)
		}
		b.WriteString(fmt.Sprintf("\t\t\tvar actual %s\n", retType))
		b.WriteString(fmt.Sprintf("\t\t\tco.Stdout = captureStdout(func() { actual = %s(%s) })\n", meta.Name, strings.Join(args, ", ")))
		b.WriteString("\t\t\tenc, err := json.Marshal(actual)\n")
		b.WriteString("\t\t\tif err != nil { co.Status = \"error\"; co.Err = err.Error(); co.ElapsedMS = float64(time.Since(t0).Microseconds()) / 1000.0; results = append(results, co); return }\n")
		b.WriteString("\t\t\tco.Actual = string(enc)\n")
		b.WriteString("\t\t\tco.Status = \"ran\"\n")
		b.WriteString("\t\t\tco.ElapsedMS = float64(time.Since(t0).Microseconds()) / 1000.0\n")
		b.WriteString("\t\t\tresults = append(results, co)\n")
		b.WriteString("\t\t}()\n\t}\n")
	}
	b.WriteString("\t_ = json.NewEncoder(os.Stdout).Encode(map[string]any{\"build_err\": \"\", \"cases\": results})\n")
	b.WriteString("}\n")
	return b.String(), nil
}

func stripGoPackage(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "package ") {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

func quoteJSONString(raw string) (string, error) {
	b, err := json.Marshal(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
