package runner

import (
	"bytes"
	"context"
	"encoding/json"
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

func (p pythonRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if res, err, done := validateSpec(spec, p.bin, p.Available()); done {
		return res, err
	}

	src, err := os.ReadFile(spec.SolutionPath)
	if err != nil {
		return Result{}, fmt.Errorf("runner: read solution: %w", err)
	}
	inJSON, err := marshalPayload(spec, string(src))
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
	return judgeHarness(spec, elapsed, out), nil
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
// the user's source, invokes Solution().<entry> for each case, and prints JSON
// on stdout. Comparison is done on the Go side (EqualOutputs).
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
        entry_out["status"] = "ran"
    except Exception:
        entry_out["elapsed_ms"] = (time.perf_counter() - t0) * 1000.0
        entry_out["stdout"] = buf.getvalue()
        entry_out["status"] = "error"
        tb = traceback.format_exc().strip().splitlines()
        entry_out["err"] = tb[-1] if tb else "error"
    results.append(entry_out)

emit({"build_err": "", "cases": results})
`
