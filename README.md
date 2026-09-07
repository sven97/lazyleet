# lazyleet

A lazygit-style terminal UI for practising on LeetCode: browse problems and
study plans, then solve them in a side-by-side workspace with a local test
runner and one-key submit — in your own editor.

> **Status: early development.** See [`PLAN.md`](PLAN.md) for the phase-by-phase
> build plan, progress log, and findings. Working now: browse mode (`lazyleet`)
> — a sources sidebar, a fuzzy-filterable problem list, study plans with
> progress, and a live statement preview, all read from a local SQLite cache;
> and the "Tier C" coding workspace (statement + live solution mirror + local
> Python test runner). Run/submit-to-LeetCode (Phase 6) is not built yet.

## Building

```sh
go build -o dist/lazyleet ./cmd/lazyleet
# or
make build
```

Requires Go 1.27+. The SQLite driver is pure Go, so no C toolchain is needed.

## Usage (so far)

```sh
lazyleet sync                 # cache the full problem list + study plans locally
lazyleet                      # browse mode: sidebar · fuzzy list · preview
                              #   / filter · tab panes · enter opens workspace
                              #   s sync · z zoom · ? help · q quit

lazyleet solve two-sum        # jump straight into the workspace for one problem
lazyleet solve valid-parentheses --lang python3
lazyleet solve two-sum --refresh   # bypass the local cache

lazyleet auth                 # store LeetCode cookies (only needed for run/submit)
lazyleet auth status
lazyleet auth logout

lazyleet debug list [--remote]     # cached (or live) problem list
lazyleet debug problem <slug>      # fetch + print one problem's detail
lazyleet debug plan leetcode-75    # print an official or bundled study plan
lazyleet debug paths | debug config
lazyleet --version
```

Problem data is read from a bundled fixture, then the local SQLite cache, then
LeetCode (no login required for public problems; results are cached afterwards).

In the workspace: `e` edit in `$EDITOR` · `r` run local tests · `tab` switch
pane · `j`/`k` scroll · `z` zoom the focused pane · `q` quit. Saving the
solution file (from `$EDITOR` or any other editor) re-runs the tests
automatically. The workspace lives at
`$XDG_DATA_HOME/lazyleet/workspace/1-two-sum/`.

## Configuration

`config.yml` lives in `$XDG_CONFIG_HOME/lazyleet/` (default
`~/.config/lazyleet/`). Data — the SQLite cache, auth file, and problem
workspaces — lives in `$XDG_DATA_HOME/lazyleet/` (default
`~/.local/share/lazyleet/`). All keys are optional:

```yaml
region: com                # com | cn
default_language: python3
editor: ""                 # empty -> $VISUAL, $EDITOR, then vi
theme: default
cache_ttl: 24h
keys: {}                   # semantic action -> key override
workspace:
  tier: auto               # auto | mirror | multiplexer | embedded
  run_on_save: true
  run_debounce_ms: 400
```

## License

TBD.
