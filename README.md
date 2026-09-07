# lazyleet

A lazygit-style terminal UI for practising on LeetCode: browse problems and
study plans, then solve them in a side-by-side workspace with a local test
runner and one-key submit — in your own editor.

> **Status: early development.** See [`PLAN.md`](PLAN.md) for the phase-by-phase
> build plan, progress log, and findings. Phase 0 is done. A first slice of the
> coding workspace ("Tier C": statement + live solution mirror + local test
> results) works against a bundled Two Sum fixture — the LeetCode API (Phase 1)
> and browse mode (Phase 2) are not built yet.

## Building

```sh
go build -o dist/lazyleet ./cmd/lazyleet
# or
make build
```

Requires Go 1.27+. The SQLite driver is pure Go, so no C toolchain is needed.

## Usage (so far)

```sh
lazyleet solve two-sum        # open the Tier C coding workspace for the fixture
lazyleet solve two-sum --lang python3

lazyleet                      # browse mode (Phase 2 — not built yet)
lazyleet debug paths          # show resolved config/data locations
lazyleet debug config         # show the effective configuration
lazyleet --version
```

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
