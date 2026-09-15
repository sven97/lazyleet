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

	dir, err := os.MkdirTemp("", "lazyleet-go-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)

	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		return Result{}, err
	}
	binPath := filepath.Join(dir, "solution")

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	build := exec.CommandContext(runCtx, "go", "build", "-o", binPath, mainPath)
	build.Dir = dir
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
	b.WriteString("import (\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"os\"\n\t\"time\"\n)\n")
	b.WriteString(userSrc)
	b.WriteString("\n\n")
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
			b.WriteString(fmt.Sprintf("\t\t\tvar arg%d %s\n", pi, paramTypes[pi]))
			lit, err := quoteJSONString(c.In[pi])
			if err != nil {
				return "", err
			}
			b.WriteString(fmt.Sprintf("\t\t\tif err := json.Unmarshal([]byte(%s), &arg%d); err != nil { co.Status = \"error\"; co.Err = err.Error(); co.ElapsedMS = float64(time.Since(t0).Microseconds()) / 1000.0; results = append(results, co); return }\n", lit, pi))
		}
		args := make([]string, len(meta.Params))
		for pi := range meta.Params {
			args[pi] = fmt.Sprintf("arg%d", pi)
		}
		b.WriteString(fmt.Sprintf("\t\t\tvar actual %s = %s(%s)\n", retType, meta.Name, strings.Join(args, ", ")))
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
