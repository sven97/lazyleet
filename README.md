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
│· anonymous  ·  com                 ││                                                            │
│4055 problems · synced 5m ago       ││   1. Two Sum                                               │
│○ daily · not done                  ││                                                            │
╰────────────────────────────────────╯│  Given an array of integers  nums  and an integer          │
╭────────────────────────────────────╮│  target , return indices of the two numbers such that      │
│Sources                             ││  they add up to  target .                                  │
│PROBLEMS                            ││                                                            │
│● All Problems                      ││  You may assume that each input would have exactly         │
│  Daily Question                    ││  one solution, and you may not use the same element        │
│                                    ││  twice.                                                    │
│STUDY PLANS                         ││                                                            │
│  lazyleet Starter 10               ││  You can return the answer in any order.                   │
│  LeetCode 75                       ││                                                            │
│  Top Interview 150                 ││  ## Example 1                                              │
│  Top 100 Liked                     ││                                                            │
╰────────────────────────────────────╯│    Input:  nums = [2,7,11,15], target = 9                  │
╭────────────────────────────────────╮│    Output: [0,1]                                           │
│Problems  (4055)                    ││    Explanation: nums[0] + nums[1] == 9, so we return       │
│       # Dif    AC%  Title          ││  [0, 1].                                                   │
│·     1 E    58.2%  Two Sum         ││                                                            │
│·     2 M    49.3%  Add Two Numbers ││  ## Example 2                                              │
│·     3 M    40.0%  Longest Substri…││                                                            │
│·     4 H    47.6%  Median of Two S…││    Input:  nums = [3,2,4], target = 6                      │
│·     5 M    38.7%  Longest Palindr…││    Output: [1,2]                                           │
│·     6 M    55.1%  Zigzag Conversi…││                                                            │
│·     7 M    32.5%  Reverse Integer ││  ## Example 3                                              │
│·     8 M    21.7%  String to Integ…││                                                            │
│·     9 E    61.0%  Palindrome Numb…││    Input:  nums = [3,3], target = 6                        │
╰────────────────────────────────────╯╰────────────────────────────────────────────────────────────╯
↵ open workspace │ / fuzzy filter │ d/f/p filter │ S cycle sort │ tab next pane │ s sync │ ? help │…
```

**Solve in your own editor, tests rerun automatically on save:**

```
╭──────────────────────────────────────────╮╭──────────────────────────────────────────────────────╮
│Two Sum  Easy                             ││solution.py                                           │
│                                          ││1 class Solution:                                     │
│   1. Two Sum                             ││2     def twoSum(self, nums: List[int], target: int) -│
│                                          ││3         seen = {}                                   │
│  Given an array of integers  nums        ││4         for i, n in enumerate(nums):                │
│  and an integer  target , return         ││5             if target - n in seen:                  │
│  indices of the two numbers such         ││6                 return [seen[target - n], i]        │
│  that they add up to  target .           ││7             seen[n] = i                             │
│                                          ││                                                      │
│  You may assume that each input          ││                                                      │
│  would have exactly one solution,        ││                                                      │
│  and you may not use the same            ││                                                      │
│  element twice.                          ││                                                      │
│                                          ││                                                      │
│  You can return the answer in any        ││                                                      │
│  order.                                  ││                                                      │
│                                          │╰──────────────────────────────────────────────────────╯
│  ## Example 1                            │╭──────────────────────────────────────────────────────╮
│                                          ││Results                                               │
│    Input:  nums = [2,7,11,15],           ││✓ 3/3 passed  ·  55ms                                 │
│  target = 9                              ││                                                      │
│    Output: [0,1]                         ││✓ case 1  0.0ms                                       │
│    Explanation: nums[0] + nums[1]        ││  in   [2,7,11,15], 9                                 │
│  == 9, so we return [0, 1].              ││  out  [0, 1]                                         │
│                                          ││                                                      │
│  ## Example 2                            ││✓ case 2  0.0ms                                       │
│                                          ││  in   [3,2,4], 6                                     │
│    Input:  nums = [3,2,4], target =      ││  out  [1, 2]                                         │
│  6                                       ││                                                      │
╰──────────────────────────────────────────╯╰──────────────────────────────────────────────────────╯
e edit │ r run local │ R run @LC │ s submit │ tab next pane │ z zoom │ b back │ q quit
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
lazyleet sync                 # cache the full problem list + study plans locally
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
# auth is only needed for `R` run / `s` submit on LeetCode

lazyleet debug list [--remote]     # cached (or live) problem list
lazyleet debug problem <slug>      # fetch + print one problem's detail
lazyleet debug plan leetcode-75    # print an official or bundled study plan
lazyleet debug paths | debug config
lazyleet --version
```

Problem data is read from a bundled fixture, then the local SQLite cache, then
LeetCode (no login required for public problems; results are cached afterwards).

In the workspace: `e` edit in `$EDITOR` · `r` run local tests · `R` run on
LeetCode · `s` submit · `i` import the last failing case · `tab` switch pane ·
`j`/`k` scroll · `z` zoom · `q` back. Saving the solution file (from `$EDITOR`
or any other editor) re-runs the local tests automatically. `R`/`s` need
`lazyleet auth`. The workspace lives at
`$XDG_DATA_HOME/lazyleet/workspace/<id>-<slug>/`.

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
