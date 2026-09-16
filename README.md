# lazyleet

Practice LeetCode without ever leaving the terminal — browse problems and
study plans, solve them in your own editor, and know whether you're right
before you spend a submission on it.

> **Status: early development.** Core flows below already work day to day;
> rough edges are expected.

## Why lazyleet

LeetCode's own UI works, but it means bouncing between a browser tab for the
problem, another for your notes, and an editor for the code — and every "did
that work?" costs a real submission attempt against a shared judge queue.
lazyleet collapses that into one screen:

- **Stay in flow.** Write your solution in whatever editor you already use
  (`$EDITOR`, vim, VS Code, whatever). lazyleet watches the file and reruns
  your tests the instant you hit save — no switching back to check.
- **Know before you submit.** A local test runner catches wrong answers and
  crashes immediately, using the problem's own examples (or a case you import
  from a real failed run). Submit when you're actually confident, not to find
  out.
- **Find what to practice, fast.** A fuzzy-filterable list across LeetCode's
  full catalog, official and bundled study plans with progress tracking, and
  a sidebar that reopens right where you left off — no re-hunting for
  yesterday's problem.
- **One key for everything else.** Run against LeetCode's real judge or
  submit for real, right from the same screen, once you're signed in.

If you've used [lazygit](https://github.com/jesseduffield/lazygit), the shape
will feel familiar: a fast, keyboard-driven terminal UI over something you'd
otherwise do through a lot of clicking.

## Screenshots

**Browse problems, study plans, and the statement side by side:**

```
╭────────────────────────────────────╮╭────────────────────────────────────────────────────────────╮
│Status                              ││1. Two Sum                                                  │
│· anonymous                         ││                                                            │
│4055 problems · synced 4h ago       ││  Given an array of integers  nums  and an integer          │
│○ daily · not done                  ││  target , return indices of the two numbers such that      │
╰────────────────────────────────────╯│  they add up to  target .                                  │
╭────────────────────────────────────╮│                                                            │
│Sources                             ││  You may assume that each input would have exactly         │
│PROBLEMS                            ││  one solution, and you may not use the same element        │
│● All Problems                      ││  twice.                                                    │
│  Daily Question                    ││                                                            │
│                                    ││  You can return the answer in any order.                   │
│STUDY PLANS                         ││                                                            │
│  lazyleet Starter 10               ││  ## Example 1                                              │
│  LeetCode 75                       ││                                                            │
│  Top Interview 150                 ││    Input:  nums = [2,7,11,15], target = 9                  │
│  Top 100 Liked                     ││    Output: [0,1]                                           │
╰────────────────────────────────────╯│    Explanation: nums[0] + nums[1] == 9, so we return       │
╭────────────────────────────────────╮│  [0, 1].                                                   │
│Problems  (4055)                    ││                                                            │
│       # Dif    AC%  Title          ││  ## Example 2                                              │
│·     1 E    58.2%  Two Sum         ││                                                            │
│·     2 M    49.3%  Add Two Numbers ││    Input:  nums = [3,2,4], target = 6                      │
│·     3 M    40.0%  Longest Substri…││    Output: [1,2]                                           │
│·     4 H    47.6%  Median of Two S…││                                                            │
│·     5 M    38.7%  Longest Palindr…││  ## Example 3                                              │
│·     6 M    55.1%  Zigzag Conversi…││                                                            │
│·     7 M    32.5%  Reverse Integer ││    Input:  nums = [3,3], target = 6                        │
│·     8 M    21.7%  String to Integ…││    Output: [0,1]                                           │
│·     9 E    61.0%  Palindrome Numb…││                                                            │
╰────────────────────────────────────╯╰────────────────────────────────────────────────────────────╯
↵ open workspace │ / fuzzy filter │ d/f/p filter │ S cycle sort │ tab next pane │ s sync │ ? help │…
```

**Solve in your own editor, tests rerun automatically on save:**

```
╭──────────────────────────────────────────╮╭──────────────────────────────────────────────────────╮
│Two Sum  Easy                             ││solution.py                                           │
│                                          ││1 class Solution:                                     │
│  Given an array of integers  nums        ││2     def twoSum(self, nums: List[int], target: int) -│
│  and an integer  target , return         ││3         seen = {}                                   │
│  indices of the two numbers such         ││4         for i, n in enumerate(nums):                │
│  that they add up to  target .           ││5             if target - n in seen:                  │
│                                          ││6                 return [seen[target - n], i]        │
│  You may assume that each input          ││7             seen[n] = i                             │
│  would have exactly one solution,        ││                                                      │
│  and you may not use the same            ││                                                      │
│  element twice.                          ││                                                      │
│                                          ││                                                      │
│  You can return the answer in any        ││                                                      │
│  order.                                  ││                                                      │
│                                          ││                                                      │
│  ## Example 1                            │╰──────────────────────────────────────────────────────╯
│                                          │╭──────────────────────────────────────────────────────╮
│    Input:  nums = [2,7,11,15],           ││Results                                               │
│  target = 9                              ││✓ 3/3 passed  ·  88ms                                 │
│    Output: [0,1]                         ││                                                      │
│    Explanation: nums[0] + nums[1]        ││✓ case 1  0.0ms                                       │
│  == 9, so we return [0, 1].              ││  in   [2,7,11,15], 9                                 │
│                                          ││  out  [0, 1]                                         │
│  ## Example 2                            ││                                                      │
│                                          ││✓ case 2  0.0ms                                       │
│    Input:  nums = [3,2,4], target =      ││  in   [3,2,4], 6                                     │
│  6                                       ││  out  [1, 2]                                         │
╰──────────────────────────────────────────╯╰──────────────────────────────────────────────────────╯
e edit │ r run local │ R run @LC │ s submit │ i import │ tab next pane │ z zoom │ ? help │ b back │…
```

*(Real terminal captures, colors flattened to text for the README — run it
yourself to see it in full color.)*

## Install

**Homebrew** (macOS or Linux):

```sh
brew install sven97/tap/lazyleet
```

That taps `sven97/homebrew-tap` and installs `lazyleet` in one step. Later,
`brew upgrade lazyleet` (or plain `brew upgrade`) picks up new releases.

**From source**, if you'd rather not add a tap:

```sh
go install github.com/sven97/lazyleet/cmd/lazyleet@latest
```

or clone and build locally:

```sh
go build -o dist/lazyleet ./cmd/lazyleet
# or
make build
```

Requires Go 1.27+. The SQLite driver is pure Go, so no C toolchain is needed.

## Usage (so far)

```sh
lazyleet sync                 # cache the full problem list + bundled study plans
lazyleet sync --plans         # also prefetch official plans for offline use
lazyleet                      # browse mode: sidebar · list · preview
                              #   / fuzzy filter · d/f/p filter · S sort · c clear
                              #   tab panes · enter opens workspace · s sync · ? help

lazyleet solve two-sum        # jump straight into the workspace for one problem
lazyleet solve valid-parentheses --lang python3
lazyleet solve two-sum --refresh   # bypass the local cache

lazyleet auth                 # open a browser, sign in; lazyleet reads the
                              #   session over DevTools (no keychain prompt)
lazyleet auth import          # or: reuse cookies from a signed-in browser
lazyleet auth import --browser firefox
lazyleet auth paste           # or: enter LEETCODE_SESSION + csrftoken yourself
lazyleet auth browsers        # list detected browser cookie stores
lazyleet auth status | logout
# successful auth automatically refreshes your solve progress
# auth is needed for personal progress and `R` run / `s` submit on LeetCode

lazyleet debug list [--remote]     # cached (or live) problem list
lazyleet debug problem <slug>      # fetch + print one problem's detail
lazyleet debug plan leetcode-75    # print an official or bundled study plan
lazyleet debug paths | debug config
lazyleet --version
```

Problem data is read from a bundled fixture, then the local SQLite cache, then
LeetCode (no login required for public problems; results are cached afterwards).
Official study plans refresh according to `cache_ttl`; if a refresh fails,
browse and `debug plan` can still use the cached plan. `sync --plans` checks
plans even when the problem catalog is fresh; add `--force` to refresh both.
A failed prefetch reports an error while keeping previously cached plans.

In the workspace: `e` edit in `$EDITOR` · `r` run local tests · `R` run on
LeetCode · `s` submit · `i` import the last failing case · `tab` switch pane ·
`j`/`k` scroll · `z` zoom · `t` manage tests · `b`/`q` back to browse. Saving the solution file
(from `$EDITOR` or any other editor) re-runs the local tests automatically.
`R`/`s` need `lazyleet auth`. If prompted, run it in another terminal, then
retry the key in your existing workspace. In an already-open browse screen,
press `s` after signing in to refresh account, progress, and daily indicators.
If the post-login refresh fails, credentials remain saved; retry with
`lazyleet sync --progress`. The workspace lives at
`$XDG_DATA_HOME/lazyleet/workspace/<id>-<slug>/`.

### Filtering by topic

In browse mode, press `t` to open the topic picker. Type to search, use the
arrow keys to move, Space to select or deselect, and Enter to apply. Multiple
selected topics require a problem to match **all** of them. Esc cancels your
changes; Ctrl+R clears the draft selection. Topic counts refer to the cached
catalog, and the picker works offline.

Topics combine with difficulty, solve status, paid-only, and fuzzy title
filters, including inside study plans and the daily challenge. Press `c` in
browse to clear topic and other structured filters.

### Managing test cases

Press `t` in a workspace to open the test-case manager. Use `j`/`k` or arrow
keys to select a case, `a` to add, `e`/Enter to edit, and `d` to delete with
confirmation. The `i` shortcut still imports the last failing remote case.

Enter one JSON input value per line, in the parameter order shown, and an
optional JSON expected result. Use Tab to switch fields, Ctrl+S to save, and
Esc to cancel. Close the manager with Esc and press `r` to run your updated
cases. Saves preserve comments and untouched cases in `testcases.jsonl` and
replace the file atomically. If another editor changed the file, cancel the
edit and press `r` in the manager to reload before trying again.

### Attempt history

Press `a` in a workspace (outside the test-case manager, where `a` instead
adds a case) to see its latest 50 local runs, LeetCode runs, and submissions,
across languages. Use arrow keys or `j`/`k` to select an attempt, Enter for
details, `r` to reload, and Esc to return. History includes verdicts, pass
counts, runtime/memory when available, judge IDs, and diagnostic output.

Attempts, including run-on-save and failed requests, are stored in the local
SQLite database and remain available offline after restarting. A history-write
failure is reported without replacing the judge result. Solution snapshots and auth
credentials are not recorded; diagnostic output can include compiler excerpts.

### Local judge languages

| Language | Local `r` | Notes |
|----------|-----------|--------|
| `python3` / `python` | yes | needs `python3` on PATH |
| `javascript` | yes | needs `node` on PATH |
| `golang` | yes | needs `go` on PATH |
| `java` | yes | needs `javac` + `java` on PATH |
| `cpp` | yes | needs `g++` on PATH |
| `typescript`, `c`, `rust`, … | remote only | scaffold works; use `R` / `s` |

Design problems (LRU Cache-style class/method sequences) are not run locally
yet — the results pane shows a clear unsupported message instead of a false
fail. Array returns compare order-insensitively; floats allow a 1e-5 tolerance.

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
images: auto                # auto | off | blocks | kitty  (LAZYLEET_IMG overrides)
cache_ttl: 24h
keys: {}                   # semantic action -> key override
workspace:
  tier: auto               # auto | mirror | multiplexer | embedded
  run_on_save: true
  run_debounce_ms: 400
```

## License

[MIT](LICENSE)
