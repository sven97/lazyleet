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

type javascriptRunner struct{}

func (javascriptRunner) Available() bool {
	_, err := exec.LookPath("node")
	return err == nil
}

func (j javascriptRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if res, err, done := validateSpec(spec, "node", j.Available()); done {
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

	cmd := exec.CommandContext(runCtx, "node", "-e", jsHarness)
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

// jsHarness loads user source in a VM, supports `var twoSum = function` and
// `class Solution { twoSum(...) }`, and returns JSON results on stdout.
const jsHarness = `
const fs = require("fs");
const vm = require("vm");
const data = JSON.parse(fs.readFileSync(0, "utf8"));
const entry = data.entry;

function emit(obj) {
  process.stdout.write(JSON.stringify(obj));
}

const sandbox = {
  console,
  module: { exports: {} },
  exports: {},
  require,
  ListNode: undefined,
  TreeNode: undefined,
};
sandbox.global = sandbox;
sandbox.globalThis = sandbox;

try {
  vm.createContext(sandbox);
  vm.runInContext(data.source, sandbox, { filename: "solution.js" });
} catch (e) {
  emit({ build_err: String(e && e.stack || e) });
  process.exit(0);
}

function resolveEntry() {
  if (typeof sandbox[entry] === "function") return (...args) => sandbox[entry](...args);
  if (typeof sandbox.Solution === "function") {
    const sol = new sandbox.Solution();
    if (typeof sol[entry] === "function") return (...args) => sol[entry](...args);
  }
  if (sandbox.module && sandbox.module.exports) {
    const ex = sandbox.module.exports;
    if (typeof ex[entry] === "function") return (...args) => ex[entry](...args);
    if (typeof ex === "function") {
      try {
        const sol = new ex();
        if (typeof sol[entry] === "function") return (...args) => sol[entry](...args);
      } catch (_) {}
    }
  }
  throw new Error("entry point not found: " + entry);
}

let fn;
try {
  fn = resolveEntry();
} catch (e) {
  emit({ build_err: String(e && e.stack || e) });
  process.exit(0);
}

const results = [];
for (let i = 0; i < data.cases.length; i++) {
  const c = data.cases[i];
  const entry_out = { index: i, status: "", actual: "", stdout: "", err: "", elapsed_ms: 0 };
  let args;
  try {
    args = c.in.map((raw) => JSON.parse(raw));
  } catch (e) {
    entry_out.status = "error";
    entry_out.err = "cannot parse input literal: " + String(e);
    results.push(entry_out);
    continue;
  }
  const t0 = process.hrtime.bigint();
  try {
    const actual = fn(...args);
    entry_out.elapsed_ms = Number(process.hrtime.bigint() - t0) / 1e6;
    entry_out.actual = JSON.stringify(actual === undefined ? null : actual);
    entry_out.status = "ran";
  } catch (e) {
    entry_out.elapsed_ms = Number(process.hrtime.bigint() - t0) / 1e6;
    entry_out.status = "error";
    entry_out.err = String(e && e.message || e);
  }
  results.push(entry_out);
}
emit({ build_err: "", cases: results });
`
