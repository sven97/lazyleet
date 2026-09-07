package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type pythonRunner struct{ bin string }

func pythonBin(lang string) string {
	if lang == "python" {
		if _, err := exec.LookPath("python"); err == nil {
			return "python"
		}
	}
	return "python3"
}

func (p pythonRunner) Available() bool {
	_, err := exec.LookPath(p.bin)
	return err == nil
}

// payload is the JSON handed to the harness on stdin.
type payload struct {
	Entry  string        `json:"entry"`
	Source string        `json:"source"`
	Cases  []payloadCase `json:"cases"`
}

type payloadCase struct {
	In  []string `json:"in"`
	Out string   `json:"out"`
}

// harnessOut is the JSON the harness prints on stdout.
type harnessOut struct {
	BuildErr string `json:"build_err"`
	Cases    []struct {
		Index     int     `json:"index"`
		Status    string  `json:"status"`
		Actual    string  `json:"actual"`
		Stdout    string  `json:"stdout"`
		Err       string  `json:"err"`
		ElapsedMS float64 `json:"elapsed_ms"`
	} `json:"cases"`
}

func (p pythonRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if spec.Meta.Name == "" {
		return Result{}, errors.New("runner: spec.Meta.Name is empty")
	}
	if len(spec.Cases) == 0 {
		return Result{}, errors.New("runner: no test cases")
	}
	if !p.Available() {
		return Result{}, fmt.Errorf("runner: %s not found on PATH", p.bin)
	}

	src, err := os.ReadFile(spec.SolutionPath)
	if err != nil {
		return Result{}, fmt.Errorf("runner: read solution: %w", err)
	}

	in := payload{Entry: spec.Meta.Name, Source: string(src)}
	for _, c := range spec.Cases {
		in.Cases = append(in.Cases, payloadCase{In: c.In, Out: c.Out})
	}
	inJSON, err := json.Marshal(in)
	if err != nil {
		return Result{}, err
	}

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, p.bin, "-I", "-c", pythonHarness)
	cmd.Stdin = bytes.NewReader(inJSON)
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
	if out.BuildErr != "" {
		return Result{BuildErr: out.BuildErr}, nil
	}

	res := Result{Total: len(spec.Cases), Elapsed: elapsed}
	for i, hc := range out.Cases {
		cr := CaseResult{
			Index:   hc.Index,
			Status:  Status(hc.Status),
			Actual:  hc.Actual,
			Stdout:  hc.Stdout,
			Err:     hc.Err,
			Elapsed: time.Duration(hc.ElapsedMS * float64(time.Millisecond)),
		}
		if i < len(spec.Cases) {
			cr.Input = spec.Cases[i].In
			cr.Expected = spec.Cases[i].Out
		}
		if cr.Status == StatusPass {
			res.Passed++
		}
		res.Cases = append(res.Cases, cr)
	}
	return res, nil
}

func timedOut(spec Spec, elapsed time.Duration) Result {
	r := Result{Total: len(spec.Cases), Elapsed: elapsed}
	for i, c := range spec.Cases {
		r.Cases = append(r.Cases, CaseResult{
			Index: i, Status: StatusTimeout, Input: c.In, Expected: c.Out,
			Err: "run exceeded the time limit",
		})
	}
	return r
}

func trimErr(stderr string, err error) string {
	if stderr != "" {
		return stderr
	}
	return err.Error()
}

// pythonHarness runs entirely from -c. It reads a JSON payload on stdin, execs
// the user's source in a namespace preloaded with the imports LeetCode's Python
// environment provides, invokes Solution().<entry> for each case, and prints a
// JSON result on stdout. It never raises: a load failure becomes {"build_err"}.
const pythonHarness = `
import sys, json, io, traceback, time
from contextlib import redirect_stdout

def emit(obj):
    sys.stdout.write(json.dumps(obj))
    sys.stdout.flush()

data = json.load(sys.stdin)
entry = data["entry"]

preamble = (
    "from typing import List, Optional, Dict, Set, Tuple, Deque\n"
    "import collections, heapq, bisect, math, itertools, functools, re, string\n"
    "from collections import defaultdict, deque, Counter, OrderedDict\n"
)
ns = {}
try:
    exec(compile(preamble + data["source"], "solution", "exec"), ns)
    Solution = ns["Solution"]
except Exception:
    emit({"build_err": traceback.format_exc()})
    sys.exit(0)

def normalize(literal):
    literal = literal.strip()
    if literal == "":
        return None, False
    try:
        return json.loads(literal), True
    except Exception:
        return literal, True

results = []
for i, case in enumerate(data["cases"]):
    args = []
    parse_error = None
    for raw in case["in"]:
        try:
            args.append(json.loads(raw))
        except Exception:
            parse_error = "cannot parse input literal: %r" % raw
            break

    entry_out = {"index": i, "status": "", "actual": "", "stdout": "", "err": "", "elapsed_ms": 0.0}
    if parse_error:
        entry_out["status"] = "error"
        entry_out["err"] = parse_error
        results.append(entry_out)
        continue

    buf = io.StringIO()
    t0 = time.perf_counter()
    try:
        with redirect_stdout(buf):
            actual = getattr(Solution(), entry)(*args)
        entry_out["elapsed_ms"] = (time.perf_counter() - t0) * 1000.0
        entry_out["stdout"] = buf.getvalue()
        entry_out["actual"] = json.dumps(actual)
        expected, has_expected = normalize(case.get("out", ""))
        if not has_expected:
            entry_out["status"] = "unknown"
        elif actual == expected:
            entry_out["status"] = "pass"
        else:
            entry_out["status"] = "fail"
    except Exception:
        entry_out["elapsed_ms"] = (time.perf_counter() - t0) * 1000.0
        entry_out["stdout"] = buf.getvalue()
        entry_out["status"] = "error"
        tb = traceback.format_exc().strip().splitlines()
        entry_out["err"] = tb[-1] if tb else "error"
    results.append(entry_out)

emit({"build_err": "", "cases": results})
`
