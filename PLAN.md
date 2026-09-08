# lazyleet — Build Plan

A lazygit-style terminal UI for a better LeetCode practice experience: intuitive
problem browsing/search/study-plans, and a side-by-side coding workspace with
local test running and one-key submit — in the user's own editor.

> **This is a living document.** Update it as implementation moves forward:
> tick task checkboxes, set phase status, append dated entries to the
> **Progress Log** and **Findings Log**, and record any plan changes in
> **Plan Changes**. Keep companion research in `lazygit-ui-research.md` and
> `leetcode-product-analysis.md`.

Last updated: 2026-09-08 (auth = browser-driven login via chromedp)

---

## 1. Vision & scope

Two halves of the experience:

1. **Browse mode** — view the full problem list and curated study plans
   (LeetCode 75, Blind 75, NeetCode 150). See status, ID, title, acceptance,
   difficulty in a list; read the full problem description for the selected
   item; fuzzy find / filter.
2. **Workspace mode** — for a selected problem: code, run tests locally,
   iterate, view the problem side-by-side, and submit to LeetCode. Quit back to
   browse mode when done.

Guiding product priorities (from `leetcode-product-analysis.md`): a dependable
solving workspace, a clear next problem, help when stuck, saved progress. The
highest-frequency loop is **edit → run → inspect → edit**; the workspace must
make that loop frictionless.

---

## 2. Decision log

| # | Decision | Choice | Status | Notes |
|---|----------|--------|--------|-------|
| D1 | Tool language | Go (1.23+) | Assumed | Matches lazygit ecosystem; single static binary. |
| D2 | TUI toolkit | Bubble Tea + Lip Gloss + Bubbles (+ glamour for the statement, chroma for the code mirror) | **Decided 2026-09-07** | Chosen because Tier C/B need no embedded terminal and Bubble Tea has the ecosystem. Tier A (embedded editor pane) will need `charmbracelet/x/vt` + `creack/pty`; revisit only then. |
| D3 | LeetCode access | Unofficial GraphQL (`leetcode.com/graphql`) + REST run/submit | Assumed | No official API exists. Isolate entirely behind `internal/leetcode`. |
| D4 | Auth | `lazyleet auth login` (chromedp drives a visible browser, reads the session over CDP — no keychain) is the recommended path; `lazyleet auth` (kooky cookie scrape, keychain-free browsers first) and `--manual` / `--stdin` remain. | **Changed 2026-09-08 (×2)** | Was manual paste → cookie scrape → browser-driven. Stored 0600 in `auth.json`. `browserlogin` is a reusable browser layer for later (Cloudflare fallback, editorial scraping). |
| D5 | First judged language | Python, then JS/TS, then compiled langs | Assumed | Python driver is the reference implementation. |
| D6 | Local store | SQLite via `modernc.org/sqlite` (pure Go) | Assumed | Problem cache, detail cache, submission history, workspace state. |
| D7 | Dirs | XDG: config `~/.config/lazyleet/`, state/cache `~/.local/share/lazyleet/` | Assumed | |
| D8 | Workspace editing model | Decouple viewing from editing; persistent statement+results surface; editor is a sibling pane, not a modal takeover. Ship Tier C first, then B, then A. | Assumed | See §5. |
| D9 | Region | `leetcode.com` first; `leetcode.cn` support in Phase 7 | Assumed | Different endpoints/auth. |

Change a decision by editing the row, flipping **Status** to `Changed
YYYY-MM-DD`, and adding a **Plan Changes** entry explaining why.

---

## 3. Architecture

```
cmd/lazyleet/            entrypoint, CLI subcommands (auth, sync, debug, ...)
internal/leetcode/       GraphQL + REST client, auth, DTOs, poller
internal/store/          SQLite cache, config load/save, submission history, workspace state
internal/tui/            layout solver, panes, context/focus stack, keymap, theme, shortcut bar
internal/workspace/      on-disk problem dirs, scaffolding, file watch
internal/runner/         local judge: per-language drivers, sandbox, comparison
internal/plans/          bundled study-plan JSON (blind-75, neetcode-150, ...)
```

Cross-cutting principles:

- **Pure layout function**: `(termW, termH, focusedPane, screenMode) -> named rects`.
  Geometry never depends on selected problem, scroll position, editor cursor, or
  active tab. Port boxlayout's fixed/weight children, `ROW`/`COLUMN` semantics,
  responsive collapse thresholds, portrait mode, explicit screen modes.
- **Context/focus stack** with semantic actions (`NextItem`, `NextPane`,
  `NextTab`, `CycleScreenMode`, `Run`, `Submit`, `Edit`). Keys and mouse both
  map to the same actions.
- **Shortcut bar generated** from the same binding metadata used for dispatch.
- **The solution file on disk is the single source of truth** — embedded editor,
  external editor, and lazyleet all read/write the same file.
- **Never block the render loop** on network or subprocess work; everything async
  with spinners.
- All LeetCode quirks (headers, Cloudflare, schema drift) stay behind
  `internal/leetcode`.

---

## 4. Known risks & open questions

- **No official API.** GraphQL schema shifts; Cloudflare challenges happen.
  Mitigation: one client package, recorded fixtures, graceful "LeetCode changed"
  errors.
- **Local judge parity** is the hardest problem: per-type argument parsing,
  output comparison (order-insensitive cases, float tolerance, multiple valid
  answers), and "design" problems (LRU Cache-style op replay). Start exact,
  refine per problem family.
- **Cookie auth expires** — need a clean detect + re-auth path.
- **Paid-only problems** — detect and degrade gracefully (no content, no snippet).
- **Terminal/editor contention** — the reason §5 exists.
- Open: which TUI toolkit after the Tier A spike (D2).
- Open: bundle vs fetch study-plan definitions that aren't official LC plans.

---

## 5. Workspace mode design — seamless side-by-side

Stop treating "view problem / edit / run" as three modes. Keep the **statement +
run results as a permanent surface**; make editing a sibling pane, not a modal
takeover; add a file-watch + run-on-save loop so "jump back to run" is not a step.

### The loop (all tiers)

1. Solution file on disk = single source of truth.
2. `fsnotify` watch → 300–500 ms debounce → run local cases → Results/diff pane
   updates in place. No keypress, no focus change.
3. `R` / `s` for remote run / submit. On a remote Wrong Answer, one key imports
   the failing case as a local test.
4. Responsive: wide terminal → 3 columns; narrow → statement/results collapse to
   tabs behind the editor; zoom/focus key (screen modes) expands editor or
   results full-width.

### Tier C — read-only mirror + file-watch (build first, works everywhere)

lazyleet owns the terminal. Panes: Statement | `solution.*` mirror
(read-only, syntax-highlighted via chroma) | Results. `e` opens `$EDITOR`
(suspend/resume like `git commit`). The `fsnotify` watcher means saving in *any*
window (a tmux split, GUI VS Code, another terminal tab) refreshes the mirror and
auto-runs. Small to build; the fallback the other tiers degrade to.

### Tier B — multiplexer orchestration (real editor, adjacent pane)

If running inside tmux / zellij / wezterm / kitty, offer a "workspace layout":
spawn `$EDITOR` in an adjacent pane and arrange panes via the multiplexer CLI
(`tmux split-window`, `wezterm cli split-pane`, ...). lazyleet keeps its own pane
with statement + results, driven by the same watch/run loop. Pane navigation is
the multiplexer's. Moderate effort; genuinely simultaneous; needs a multiplexer.

### Tier A — embedded editor pane (flagship, single process)

lazyleet hosts `$EDITOR` inside one of its own panes via PTY + terminal-emulator
widget. One process, real editor, no multiplexer. Precedent: **aerc** embeds
`$EDITOR` this way. Cheap with vaxis (`term` widget) or gocui; in Bubble Tea,
wire `creack/pty` + `charmbracelet/x/vt`. Focus model: one key focuses the editor
pane (keys pass through), one key pops back to lazyleet bindings. This is the
main reason to reconsider D2.

**Recommended path:** Tier C in Phase 4 → Tier B detection right after → Tier A
as the flagship (also settles D2).

---

## 6. Phases

Status values: `Not started` · `In progress` · `Blocked` · `Done`.
Tick tasks with `[x]` as they land.

### Phase 0 — Scaffolding & foundations
**Status: Done** — D2 resolved to Bubble Tea (see decision log; the Tier C
build served as the spike).

- [x] Repo layout per §3; `go mod init github.com/sven97/lazyleet`; `git init`.
- [x] Config file `~/.config/lazyleet/config.yml` (keys, theme, default language,
      editor cmd, region, cache TTL) with load/save + defaults +
      validation. `internal/config` — `config.go`, `duration.go`, `paths.go`.
- [x] XDG dir resolution; state/cache under `~/.local/share/lazyleet/`
      (honours `XDG_CONFIG_HOME` / `XDG_DATA_HOME`, falls back to `~/.config`
      and `~/.local/share`). `Paths` + `EnsureDirs`.
- [x] SQLite schema + migrations (`internal/store`): `problems`,
      `problem_detail`, `study_plans`, `submissions`, `workspace_state` +
      `schema_migrations`. Append-only numbered migration runner, pure-Go
      `modernc.org/sqlite`, WAL.
- [x] CLI skeleton (cobra): root launches TUI (stub), `auth` / `sync` stubs,
      hidden `debug paths` / `debug config`, `--version`, `--config`,
      `--verbose`.
- [x] CI (`.github/workflows/ci.yml`): gofmt, `go vet`, `staticcheck`,
      `go build`, `go test -race`, goreleaser `check` + snapshot build.
      `.goreleaser.yaml` (v2), `Makefile`, `README.md`.
- [x] Decide D2 — resolved to Bubble Tea; the Tier C build was the spike.

### Phase 1 — LeetCode API client (headless)
**Status: In progress** — read paths done and verified against live LeetCode;
run/submit are client-only (Phase 6 wires them). `internal/leetcode`.

- [x] GraphQL client (`client.go`): `X-Csrftoken` + `Referer` + `Origin` +
      `Cookie` headers, `rate.Limiter` (2 req/s), retry on 429/5xx with linear
      backoff, `APIError` type, per-region base URLs, functional options.
- [x] `lazyleet auth login` — **browser-driven** (`internal/browserlogin` over
      `chromedp`): opens a visible Chromium window at the LeetCode login page,
      polls `network.GetCookies` over CDP until `LEETCODE_SESSION` + `csrftoken`
      appear, verifies via `userStatus`, saves. Dedicated profile at
      `<data-dir>/browser` (`--fresh` wipes it), `--browser-path` overrides
      discovery. No OS keychain prompt, no cookie-file decryption. `browserlogin`
      is intentionally a small reusable browser layer.
- [x] `lazyleet auth` — **cookie scrape** (`internal/browsercookies` over
      `browserutils/kooky`): reads the two cookies from an already-logged-in
      browser, keychain-free ones (Firefox/Safari) first, `Pick` chooses one
      browser+profile. `--browser <name>` restricts it (and allows keychain);
      `auth browsers` lists detected stores (marks "· needs keychain");
      `--manual` / `--stdin` are the paste fallbacks. `auth status` /
      `auth logout` unchanged.
- [x] Problem list (`problemsetQuestionList`), paginated
      (`ListProblems` / `ListAllProblems` with progress cb): frontend id, slug,
      title, difficulty, acRate, paidOnly, status, topic tags. `questionId` is
      **not** in this response — filled later from detail; store upsert
      preserves it across list re-syncs.
- [x] Problem detail (`questionData`): `content` HTML → Markdown
      (`html-to-markdown/v2`), `metaData` string → `Meta`, `exampleTestcases` →
      `[]testcase.Case` via arity, `codeSnippets` → map, raw fields kept for
      caching.
- [x] Study plans: `studyPlanV2Detail` for official plans (`StudyPlanDetail`,
      verified with `leetcode-75`). Bundled: `internal/plans` embeds JSON;
      ships `lazyleet-starter` (10 problems). **Blind 75 / NeetCode 150 slug
      lists still need authoring/verifying.**
- [x] `lazyleet sync` — fetch full list → SQLite (`--force`, freshness check
      against `cache_ttl`); syncs bundled plans too. `lazyleet debug
      list [--remote] | problem <slug> | plan <slug>`.
- [x] Cache layer: `store` methods `UpsertProblems` / `ListProblems` (filters:
      difficulty, status, tag, search, paid) / `GetProblem` / `ProblemsFresh`,
      `PutStudyPlan` / `GetStudyPlan`, `PutProblemDetail` / `GetProblemDetail`
      (TTL). `solve <slug>` now resolves fixture → cache → API (+persist), so it
      works for any public problem; `--refresh` bypasses the cache.
- [x] Run: `Interpret` → `POST /problems/{slug}/interpret_solution/`; poll via
      `CheckResult` / `PollResult` on `/submissions/detail/{id}/check/`.
- [x] Submit: `Submit` → `POST /problems/{slug}/submit/`; `JudgeResult` carries
      verdict, runtime/memory percentile, last failed case, compile/runtime err.
- [x] Tests: httptest fake GraphQL server (list / detail / plan / gql-error /
      retry / auth-headers), auth round-trip + 0600, metaData parse, plans
      embed, store cache methods. 40+ tests, `-race` clean.
- [ ] `submissionList` history — deferred to Phase 6.
- [ ] Detect expired session mid-run and prompt re-auth (only `auth status`
      checks today).
- [ ] gzip request/response (Go's transport handles response gzip transparently;
      explicit `Accept-Encoding` not set).

### Phase 2 — Browse mode (product part 1)
**Status: In progress** — usable read-only browser wired to the cache;
`lazyleet` (no args) launches it. `internal/tui/browse_*.go`,
`cmd/lazyleet/browse*.go`.

- [x] Layout engine: pure `ComputeBrowse(termW, termH, focused, zoom)` — sidebar
      (weighted, 18–30 cols) · list · preview (drops preview then sidebar as
      width shrinks), 1-row status bar, `z` zoom to focused pane.
- [x] Panes: sources sidebar (All Problems + bundled + official study plans) ·
      problem list · statement preview · generated shortcut bar.
- [x] Problem list: status glyph (✓/~/·), frontend id, difficulty letter
      (color-coded), acceptance %, title (🔒 for paid). Cursor index separate
      from scroll `top`; `g`/`G`, `ctrl+u`/`ctrl+d`.
- [x] Study plan view: selecting a plan reorders the list to plan order and the
      title shows `N/M solved`. Official plans fetched via `studyPlanV2Detail`
      and cached; bundled from `internal/plans`.
- [x] Fuzzy find: `/` opens a `textinput`; `sahilm/fuzzy` over `"id title"`;
      `esc` clears. (Difficulty/status/tag filter *chips* not done — the store
      supports the filters, no UI yet.)
- [x] Preview: statement Markdown via glamour, lazy-loaded on cursor change
      (120 ms debounce), cache→API. `]` toggles Statement / Topics tabs.
      (Hints / Similar tabs not done — need those fields fetched.)
- [x] Keymap + generated shortcut bar (`BrowseKeyMap`); `?` help overlay.
      (Config key overrides not wired yet — `config.Keys` still unused.)
- [x] Async loading with a spinner; empty cache → "press s to sync" + inline
      sync.
- [x] `Enter` on a problem sets `Chosen` and quits; `cmd/lazyleet` loops
      browse → `openWorkspace` → browse. Sync core shared with `lazyleet sync`
      (`synccore.go`).
- [x] Tests: fake `BrowseData`, 6 model tests (render / cursor+open / fuzzy /
      plan reorder+counts / empty-cache sync). `-race` + staticcheck clean.
- [ ] Column sort, filter chips, Hints/Similar preview tabs, config-driven keys.
- [ ] **Needs manual check in a real terminal** (pty capture unavailable in this
      env) — same as Tier C.

### Phase 3 — Workspace scaffolding & editor integration
**Status: In progress** — built ahead of Phase 1/2 to support the Tier C slice,
driven by `leetcode.Fixture` instead of the API. `internal/workspace`.

- [x] Scaffold `<workspaceRoot>/<frontendID>-<slug>/`:
  - [x] `solution.<ext>` seeded from the snippet for the chosen language
  - [x] `meta.json` (slug, ids, difficulty, lang, `Meta`, created_at) — always
        refreshed (derived, not user-edited)
  - [x] `testcases.jsonl` (seeded from `Question.ExampleCases`) — chose JSONL
        over `testcases.txt` + `.local.txt`; format in `internal/testcase`
  - [x] `notes.md`
  - [x] Idempotent: never clobbers an existing solution/notes/tests file
- [ ] Language picker limited to languages LC offers (currently: `--lang` flag +
      config `default_language`; `extForLang` covers py/js/ts/go/java/cpp/c/rust)
- [x] Editor launch: `e` → `tea.ExecProcess` suspends the TUI, runs the resolved
      editor via `sh -c`, resumes + reloads + re-runs on exit
- [x] `fsnotify` watch (`workspace.Watch`): watches the parent dir, filters to
      the solution file, coalesces, non-blocking signal channel
- [ ] Persist workspace state to SQLite (`workspace_state` table exists, unused);
      "Recent" section (needs browse mode)

### Phase 4 — Workspace mode TUI (product part 2)
**Status: In progress** — Tier C vertical slice works against the fixture via
`lazyleet solve two-sum`. `internal/tui`.

- [x] Tier C layout: Statement (glamour) · Code mirror (chroma, read-only,
      line-gutter) · Results, side by side. Pure layout solver
      (`tui.Compute`) with weights, min-width fallback to tabbed, and a
      `ModeZoom` that fills the focused pane. Bordered panes, focus highlight.
- [x] Live file watch → re-read on save → debounced (`run_debounce_ms`) local
      run; `run_on_save` config gate; generation counter cancels stale debounces.
- [x] Keys: `e` edit · `r` run local · `z`/`+` zoom · `tab`/`⇧tab` focus ·
      `j/k` `pgup/pgdn` scroll · `b`/`q` quit. `R`/`s`/`t` are wired to
      "coming in Phase 6/4" status messages.
- [x] Results pane: aggregate line, per-case pass/fail/error/timeout/unknown,
      `exp`/`got` diff on failure, stderr + captured stdout.
- [x] Generated shortcut bar from the keymap (`KeyMap.shortcutHints`).
- [ ] Panels/tabs for Tests · Notes (only Statement/Code/Results so far).
- [ ] `dirty` indicator; return to **browse mode** on `b` (currently quits).
- [ ] Test-case manager UI (edit `testcases.jsonl` by hand for now).
- [ ] Tier B: detect tmux/zellij/wezterm/kitty; spawn `$EDITOR` in an adjacent
      pane; manage layout via multiplexer CLI.
- [ ] **Needs manual check in a real terminal:** interactive render + the
      save→debounce→run loop firing on an external editor save (automated pty
      smoke test via macOS `script` couldn't deliver stdin; unit tests cover
      layout, rendering, key handling, and the runner).

### Phase 5 — Local test runner / judge
**Status: In progress** — Python driver landed with exact-match comparison.
`internal/runner`.

- [x] Driver contract: `Runner` interface (`Available`, `Run(ctx, Spec)`),
      `Spec{Lang, SolutionPath, Meta, Cases, Timeout}`, `Result` +
      `CaseResult` with statuses pass/fail/error/timeout/unknown; `runner.For`
      registry. Build failures surface as `Result.BuildErr`, not `error`.
- [x] **Python driver** (reference): a `-c` harness (no temp file) reads a JSON
      payload on stdin, `exec`s the solution in a namespace preloaded with
      LeetCode-style imports (`typing`, `collections`, …), invokes
      `Solution().<entry>(*args)` per case with stdout captured, prints JSON
      results. Subprocess is `python3 -I` with a context timeout. 5 tests
      (all-pass / wrong-answer / runtime-error / syntax-error / no-expected).
- [ ] Comparison: currently **exact** (JSON-normalised equality). Still need
      order-insensitive, float tolerance, and multiple-valid-answer handling —
      per problem family. (Two Sum's canonical solution returns ascending
      indices, so exact match is fine for the fixture.)
- [ ] Sandbox hardening: have timeout + `-I`; still need a memory cap and
      network isolation.
- [ ] JS/TS driver.
- [ ] Compiled-language drivers (Go, Java, C++) with compile step + template.
- [ ] "Design" problems: `[methodNames]` + `[args]` op replay.
- [ ] Sandbox: subprocess, wall-clock timeout, memory cap (`ulimit`/cgroups where
      available), no network.
- [ ] Fallback: no local driver → `r` transparently uses the LeetCode Run API.
- [ ] Results panel: per-case table, diff view for failures, aggregate + timing.

### Phase 6 — Run & Submit against LeetCode
**Status: In progress** — run/submit + verdict + import-failing-case + mark-solved
done; submission history panel deferred. `internal/tui` + `cmd/lazyleet`.

- [x] `tui.RemoteJudge` interface (`Available`/`Run`/`Submit`) + `RemoteOutcome`,
      implemented by `cmd/lazyleet/remotejudge.go` over `*leetcode.Client`
      (`Interpret`/`Submit` → `PollResult`). Nil / unauthenticated → `R`/`s`
      show a `lazyleet auth` hint.
- [x] Workspace `R` = Run Code (local test-case inputs as `data_input`),
      `s` = Submit. Results pane swaps to a remote view: verdict (Accepted /
      Wrong Answer / Compile Error / Runtime Error / sample pass), `N/M` cases,
      runtime + memory with percentiles, failing input, exp/got. Spinner while
      polling (90 s timeout).
- [x] `i` imports the remote failing case into `testcases.jsonl`
      (`workspace.AppendCases`, dedup by input) and kicks off a local run.
- [x] On Accepted submit: `store.SetProblemStatus(slug, "ac")` → browse list and
      study-plan `N/M solved` pick it up on next open.
- [x] Tests: fake `RemoteJudge` — unavailable hint, submit-accepted verdict
      render, import-failing-case appends + re-runs. `-race` + staticcheck clean.
- [ ] Submission history panel; diff current code vs last accepted
      (needs `submissionList` — deferred with Phase 1's history item).
- [ ] Single-key "quit whole app" from the workspace (today `q`/`b` → browse).

### Phase 7 — Polish & distribution
**Status: Not started**

- [ ] Themes / color schemes; ASCII border + light-terminal fallbacks.
- [ ] Mouse: click-to-select, wheel scroll, drag range — coordinates converted
      through scroll origin.
- [ ] First-run wizard (auth, default language, editor).
- [ ] Auth-expired and offline banners.
- [ ] `leetcode.cn` region support.
- [ ] Tests: layout solver, API client fixtures, runner drivers; integration
      tests behind a build tag needing real cookies.
- [ ] Packaging: static binary, `go install`, goreleaser + GitHub releases,
      Homebrew tap, shell completions.
- [ ] Docs: README, keybinding reference, "how local judging works", "adding a
      language driver".

---

## 7. Sequencing & MVP

- Strict order: Phase 0 → 1 → 2. Phase 3 depends on 1. Phases 4–6 overlap once 3
  lands. Phase 5 is independent of 6 and can slip.
- **Usable MVP:** Phases 1 + 2 + 3 + 5 (Python only) + 6 → browse / search /
  study-plans, open a workspace, edit in your editor, run cases locally, submit
  to LeetCode.

---

## 8. Progress Log

Append newest entries at the top. One entry per working session or milestone.

- **2026-09-08 (c)** — `lazyleet auth login`: browser-driven auth via
  `internal/browserlogin` (chromedp). Launches a visible Chromium window at the
  login page, polls CDP `network.GetCookies` until both cookies appear, saves +
  verifies. Persistent profile at `<data-dir>/browser`, `--fresh`,
  `--browser-path`. `browserlogin` kept small/reusable for a future
  Cloudflare-challenge fallback. chromedp/cdproto only added **~1.6 MB** to the
  binary (32.6 MB) — dead-code elimination keeps the used surface tiny. 2 pure
  tests (browser path can't run in CI). `-race` + staticcheck clean.
- **2026-09-08 (b)** — Auth: avoid the macOS keychain prompt. `Read` now
  iterates `TraverseCookieStores` and skips Chromium-family stores before
  opening them (no prompt) unless `--browser` names one; `importFromBrowser`
  does a keychain-free pass (Firefox/Safari) first, then a Chromium pass with a
  "click Always Allow" note. `auth browsers` marks "· needs keychain".
  +`NeedsKeychain` test.
- **2026-09-08 (a)** — Auth reworked to browser cookie auto-import
  (`lazyleet auth` with no flags). New `internal/browsercookies` wraps
  `browserutils/kooky` (`Read` + pure `Pick` group-by-browser chooser, tests);
  `cmd/lazyleet/auth.go` adds `--browser`, `auth browsers`, keeps
  `--manual`/`--stdin`. `-race` + staticcheck clean.
- **2026-09-07 (g)** — Phase 6 run/submit. `tui.RemoteJudge` interface +
  `RemoteOutcome` keep `internal/tui` transport-free; `cmd/lazyleet/
  remotejudge.go` implements it over the LeetCode client (Interpret/Submit →
  PollResult, maps `JudgeResult`). Workspace: `R` Run Code / `s` Submit with a
  swappable remote Results view (verdict, N/M, runtime+memory percentiles,
  failing input, exp/got), 90 s poll w/ spinner; `i` imports the failing case
  into `testcases.jsonl` + re-runs locally; Accepted submit →
  `store.SetProblemStatus(slug,"ac")`. New: `leetcode.JudgeResult.CorrectAnswer`
  /`RunSuccess`, `workspace.AppendCases`, `store.SetProblemStatus`. 4 new tui
  tests via a fake `RemoteJudge`. `-race` + staticcheck clean. Submission
  history panel deferred. Not yet committed.
- **2026-09-07 (f)** — Phase 2 browse mode. `internal/tui`: `ComputeBrowse`
  layout solver, `BrowseKeyMap`, `BrowseModel` (sidebar sources · fuzzy list ·
  lazy statement preview) + `browse_view.go`. `BrowseData` interface keeps the
  model decoupled from store/leetcode; `cmd/lazyleet/browsedata.go` implements
  it, `browse.go` loops browse→workspace→browse, `synccore.go` shares the sync
  logic with `lazyleet sync`. `solve.go` refactored to expose
  `appContext.openWorkspace`. `lazyleet` (no args) now launches browse. Deps:
  `sahilm/fuzzy`. 6 model tests via a fake `BrowseData`; `-race` + staticcheck
  clean. Interactive render still needs a real-terminal eyeball (pty capture
  doesn't work here). Not yet committed.
- **2026-09-07 (e)** — Phase 1 read paths. `internal/leetcode`: GraphQL client
  (rate limit, retry, auth headers, `APIError`), `auth.go` (0600 creds),
  `ListProblems`/`ListAllProblems`, `QuestionDetail` (HTML→MD), `StudyPlanDetail`,
  `Interpret`/`Submit`/`PollResult` (Phase 6 wiring later), `WhoAmI`. New
  `internal/plans` (embedded JSON, `lazyleet-starter`). `store`: problem +
  study-plan + detail caching with TTL/filters. CLI: real `auth`
  (+status/logout), real `sync`, `debug list|problem|plan`. `solve <slug>` now
  fixture→cache→API for any public problem. Deps: `x/time/rate`,
  `html-to-markdown/v2`, `x/term`. **Verified against live LeetCode**: `debug
  list --remote` (4046 problems), `debug problem two-sum` (clean MD + parsed
  metaData + 19 langs), `sync` (full list → SQLite in ~22s), `debug plan
  leetcode-75` (75 problems w/ groups). 40+ tests `-race` clean, staticcheck
  clean, goreleaser snapshot ok. Not yet committed.
- **2026-09-07 (d)** — Committed and pushed. Repo:
  `github.com/sven97/lazyleet` (private). Initial commit `de76f28` +
  `a534b97` (staticcheck fix). CI green on GitHub Actions — `test` (gofmt,
  vet, staticcheck, build, `test -race`) and `release-dryrun` (goreleaser
  check + snapshot) both pass. Only annotation is GitHub's own
  "Node 20 deprecated" notice on checkout/setup-go actions — cosmetic.
- **2026-09-07 (c)** — Tier C workspace vertical slice. D2 resolved to Bubble
  Tea. New packages: `internal/testcase` (JSONL cases, parse/write, LC-example
  splitter), `internal/leetcode` (`Question`/`Meta` DTOs + a bundled `two-sum`
  `Fixture`), `internal/workspace` (`Scaffold` + fsnotify `Watch`),
  `internal/runner` (`Runner` iface + Python `-c` harness driver),
  `internal/tui` (pure `Compute` layout solver, theme, keymap+shortcut bar,
  `WorkspaceModel` — statement/code/results panes, glamour + chroma, file-watch
  → debounce → run loop, `e` opens `$EDITOR` via `tea.ExecProcess`). New CLI:
  `lazyleet solve <slug>`. Deps: bubbletea, bubbles, lipgloss, glamour,
  chroma/v2, fsnotify. 25 tests pass under `-race`; Python driver tests really
  shell out to `python3`. Interactive terminal render + external-save loop
  still need a manual eyeball (pty smoke test via macOS `script` couldn't feed
  stdin). Still uncommitted.
- **2026-09-07 (b)** — Phase 0 scaffolding landed. Module
  `github.com/sven97/lazyleet`, Go 1.27.1. Packages: `internal/config`
  (config + XDG paths + custom YAML `Duration`, 9 tests), `internal/store`
  (SQLite via `modernc.org/sqlite`, numbered migration runner, 5-table schema,
  3 tests), `cmd/lazyleet` (cobra CLI: TUI stub + `auth`/`sync` stubs + hidden
  `debug paths|config`). CI workflow, `.goreleaser.yaml` (v2), `Makefile`,
  `README.md`. `go build/vet/test -race` all green; `goreleaser check` +
  snapshot build pass locally. Remaining in Phase 0: the D2 embedded-editor
  spike. Nothing committed yet (no git remote; awaiting user).
- **2026-09-07 (a)** — Plan written. Research docs (`lazygit-ui-research.md`,
  `leetcode-product-analysis.md`) reviewed. Workspace side-by-side design settled
  on the three-tier approach (C → B → A). No code yet.

---

## 9. Findings Log

Technical discoveries, gotchas, and things that changed our understanding.
Append newest at the top; reference the phase/task.

- **2026-09-08** (auth) — `chromedp` + `cdproto` sound heavy (cdproto is huge
  generated code) but added only ~1.6 MB to the binary — the linker drops every
  unused CDP domain. So the "browser layer" is cheap to keep. Its browser-facing
  code can't be exercised in CI (no Chrome); test the pure helpers, drive the
  rest manually.
- **2026-09-08** (auth) — the macOS "Chrome Safe Storage" keychain prompt fires
  whenever kooky opens a Chromium-family store, and the "Always Allow" grant is
  bound to the exact (unsigned) binary — so every `make build` / `go run`
  re-prompts. Mitigations shipped: `Read` skips Chromium stores *before opening
  them* unless `--browser` names one or a keychain-free pass found nothing, so
  users logged in via Firefox/Safari never see the prompt; `auth browsers`
  marks stores that "· needs keychain". User guidance: `go install` once to a
  stable path and click "Always Allow".
- **2026-09-08** (auth) — `browserutils/kooky` (the maintained fork of
  `zellyn/kooky`; `zellyn/kooky` fails `go get` — module path mismatch) adds
  ~10 indirect deps (its own pure-Go sqlite3/ese readers, keychain libs, lz4)
  and roughly doubles the binary to ~31 MB. Still `CGO_ENABLED=0`; goreleaser
  snapshot fine. Iterate cookies with `kooky.TraverseCookies` (range-over-func)
  so one unreadable store doesn't fail the whole scan.
- **2026-09-07** (Phase 6) — `interpret_solution` ("Run Code") returns
  `correct_answer` / `run_success`, not a `status_msg` like submit does. The
  outcome mapper special-cases `kind == "run"`: compile err → runtime err →
  `correct_answer` → "Wrong Answer (sample)". `data_input` for the run is the
  local test cases with every argument literal on its own line.
- **2026-09-07** (Phase 2) — Browse → workspace transition is a
  loop-in-the-command, not a wrapper model: the browse `tea.Program` exits with
  `BrowseModel.Chosen` set, `cmd/lazyleet` then runs the workspace program, and
  loops back to a fresh browse program on exit. Simpler and robust; costs a
  screen repaint between screens. From the workspace, `q`/`b` both return to
  browse — there's no single-key "quit the whole app" from inside the workspace
  yet.
- **2026-09-07** (Phase 1) — LeetCode's `problemsetQuestionList` response does
  **not** include `questionId` (internal id) — only the frontend id. The
  internal id (needed for run/submit) comes from `questionData`. `UpsertProblems`
  therefore keeps an existing non-zero `question_id` when a list sync passes 0.
- **2026-09-07** (Phase 1) — Live schema confirmed working as of today:
  `problemsetQuestionList` (categorySlug/skip/limit/filters), `question`
  (questionData), `studyPlanV2Detail` (planSubGroups), `userStatus`. Public
  reads need no auth. Full list is ~4046 problems / 41 pages; at 2 req/s a full
  `sync` is ~22s. Consider a `--rate` flag if that annoys.
- **2026-09-07** (Phase 1) — `html-to-markdown/v2` (`htmltomarkdown.ConvertString`)
  turns LeetCode's `content` HTML into clean Markdown that glamour renders well
  (code fences, `**bold**`, inline `code`, lists all survive). No pre/post
  scrubbing needed so far.
- **2026-09-07** (CI) — `bubbles/viewport` v1.0.0 deprecated
  `LineUp/LineDown/ViewUp/ViewDown` in favour of
  `ScrollUp/ScrollDown/PageUp/PageDown` (staticcheck SA1019). Local dev must
  run `staticcheck ./...` (or `make lint`) before pushing — `go vet` does not
  catch this.
- **2026-09-07** (Phase 4/5) — Running the local judge as `python3 -c <harness>`
  (harness reads a JSON payload on stdin, `exec`s the user source) avoids
  writing a `_lazyleet_runner.py` into the workspace dir and keeps `import`
  semantics simple. A syntax error in the solution → harness catches it at
  `compile()` → reported as `BuildErr`. Preloading `typing`/`collections`/etc.
  into the exec namespace means LeetCode snippets that assume those imports
  just work.
- **2026-09-07** (Phase 4) — Could not drive the Bubble Tea program from an
  automated pty here: macOS `script -q` does not forward piped stdin to the
  child, so `r`/`q` never arrived and the process had to be killed. For real
  interactive verification use `teatest` (`charmbracelet/x/exp/teatest`) or a
  manual terminal run. Model logic is unit-tested by feeding `tea.Msg`s
  directly.
- **2026-09-07** (Phase 3) — Chose `testcases.jsonl` (one `{"in":[...],"out":""}`
  per line) over the plan's `testcases.txt` + `testcases.local.txt`. One file,
  trivial round-trip, expected-output slot per case. The nicer editor is the
  Phase 4 test-case manager.
- **2026-09-07** (Phase 0) — `gopkg.in/yaml.v3` decodes `time.Duration` as raw
  int64 nanoseconds, not `"24h"` strings. Added a `config.Duration` wrapper type
  with `Marshal/UnmarshalYAML` (accepts a duration string or bare seconds). Any
  future duration config field must use this type.
- **2026-09-07** (Phase 0) — `goreleaser check` fails locally with "scm
  releases: no remote configured" until a git remote exists. Pinned an explicit
  `release.github` owner/repo in `.goreleaser.yaml` — check now passes offline.
- **2026-09-07** (Phase 0) — `modernc.org/sqlite` pulls a large transitive tree
  (libc, cc/v4, ccgo) into `go.sum` but stays pure Go (`CGO_ENABLED=0` builds
  fine). Acceptable per D6; revisit only if build times become painful.

---

## 10. Plan Changes

Record every material deviation from this plan: what changed, why, and the date.

- **2026-09-07** — Added a cobra CLI skeleton as an explicit Phase 0 task (was
  implied under "repo layout"). Deps introduced now: `spf13/cobra`,
  `gopkg.in/yaml.v3`, `modernc.org/sqlite`. Module path is
  `github.com/sven97/lazyleet`.
- **2026-09-07** — Built Tier C (Phases 3+4+5 slice) **before** Phases 1–2, at
  the user's direction, against `leetcode.Fixture` sample data. The API client
  (Phase 1) then slots in behind the same `leetcode.Question` type. Sequencing
  in §7 is otherwise unchanged.
- **2026-09-07** — D2 resolved to Bubble Tea (was "revisit in Phase 0"). Tier A
  will need `charmbracelet/x/vt` + `creack/pty`; not blocking anything now.
- **2026-09-07** — Test-case file is `testcases.jsonl`, not the two-file
  `testcases.txt`/`.local.txt` split originally planned (see Findings).
- **2026-09-07** — Phase 2 shipped the core browser (sidebar + fuzzy list +
  lazy preview + study-plan reorder/progress + inline sync) but deferred filter
  *chips*, column sort, Hints/Similar preview tabs, and config-driven keybinds
  to a later pass. `config.Keys` is still unused.
- **2026-09-07** — Phase 1 built read paths + client-only run/submit, skipping
  `submissionList` history (moved to Phase 6). `solve <slug>` gained a
  fixture→cache→API resolver so it works for any public problem now, ahead of
  browse mode. Bundled study plans: shipped a small `lazyleet-starter` to prove
  the path; Blind 75 / NeetCode 150 slug lists still TODO.
