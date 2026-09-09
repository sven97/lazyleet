package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
	"github.com/sven97/lazyleet/internal/termimg"
	"github.com/sven97/lazyleet/internal/testcase"
	"github.com/sven97/lazyleet/internal/tui"
	"github.com/sven97/lazyleet/internal/workspace"
)

func newSolveCmd(app *appContext) *cobra.Command {
	var lang string
	var refresh bool

	cmd := &cobra.Command{
		Use:   "solve <slug>",
		Short: "Open the coding workspace for a problem (Tier C)",
		Long: "Scaffold a workspace directory for the problem and open the side-by-side\n" +
			"coding view: statement, a live mirror of your solution file, and local\n" +
			"test results. Edit with `e` (your $EDITOR); saves auto-run.\n\n" +
			"Problem data comes from a bundled fixture, then the local cache, then\n" +
			"LeetCode (cached afterwards). No login needed for public problems.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.openWorkspace(cmd.Context(), args[0], lang, refresh)
		},
	}

	cmd.Flags().StringVar(&lang, "lang", "", "language slug (default: config default_language)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "bypass the local cache and refetch from LeetCode")
	return cmd
}

// openWorkspace resolves a problem, scaffolds its workspace directory, and runs
// the workspace TUI until the user exits it.
func (a *appContext) openWorkspace(ctx context.Context, slug, lang string, refresh bool) error {
	q, src, err := a.resolveQuestion(ctx, slug, refresh)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "loaded %s (%s)\n", slug, src)

	if err := a.paths.EnsureDirs(); err != nil {
		return err
	}
	if lang == "" {
		lang = a.cfg.DefaultLanguage
	}
	if _, ok := q.CodeSnippets[lang]; !ok {
		picked := pickLanguage(q.CodeSnippets, lang)
		if picked == "" {
			return fmt.Errorf("no starter code available for %s", slug)
		}
		fmt.Fprintf(os.Stderr, "no %s starter; using %s\n", lang, picked)
		lang = picked
	}

	ws, err := workspace.Scaffold(a.paths.WorkspaceRoot, q, lang)
	if err != nil {
		return err
	}

	m, err := tui.NewWorkspaceModel(
		ws, q,
		a.cfg.ResolveEditor(),
		a.cfg.Workspace.RunOnSave,
		a.cfg.Workspace.RunDebounceMs,
		newRemoteJudge(a, q, lang),
	)
	if err != nil {
		return err
	}
	iw := tui.NewImageWriter(os.Stdout)
	m.EnableImages(termimg.Detect(a.cfg.Images), a.paths.ImageCacheDir, iw)

	_, err = tea.NewProgram(m,
		tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithOutput(iw),
	).Run()
	return err
}

// resolveQuestion returns problem detail from the first available source:
// bundled fixture, local cache, then LeetCode (persisting the result).
func (a *appContext) resolveQuestion(ctx context.Context, slug string, refresh bool) (leetcode.Question, string, error) {
	if q, ok := leetcode.Fixture(slug); ok && !refresh {
		return q, "fixture", nil
	}

	db, err := a.openStore()
	if err != nil {
		return leetcode.Question{}, "", err
	}
	defer db.Close()

	if !refresh {
		if d, err := db.GetProblemDetail(ctx, slug, a.cfg.CacheTTL.D()); err == nil {
			row, _ := db.GetProblem(ctx, slug)
			q := questionFromCache(d, row)
			// A detail cached before we scraped example outputs has judge-able
			// examples but no expected values — refetch once to fill them in.
			// The recency check stops a problem that genuinely has no scrapable
			// outputs from refetching on every open.
			if !exampleOutputsMissing(q) || time.Since(d.FetchedAt) < time.Hour {
				return q, "cache", nil
			}
		}
	}

	client, err := a.newClient()
	if err != nil {
		return leetcode.Question{}, "", err
	}
	q, err := client.QuestionDetail(ctx, slug)
	if err != nil {
		return leetcode.Question{}, "", err
	}
	if err := db.PutProblemDetail(ctx, cacheFromQuestion(q)); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not cache problem detail:", err)
	}
	return q, "leetcode", nil
}

func cacheFromQuestion(q leetcode.Question) store.ProblemDetail {
	snips, _ := json.Marshal(q.CodeSnippets)
	var exCases string
	if len(q.ExampleCases) > 0 {
		if b, err := json.Marshal(q.ExampleCases); err == nil {
			exCases = string(b)
		}
	}
	return store.ProblemDetail{
		Slug:             q.Slug,
		QuestionID:       q.QuestionID,
		StatementMD:      q.Statement,
		MetaJSON:         q.MetaData,
		ExampleTestcases: q.ExampleTestcases,
		ExampleCasesJSON: exCases,
		CodeSnippetsJSON: string(snips),
	}
}

func questionFromCache(d store.ProblemDetail, row store.Problem) leetcode.Question {
	var snips map[string]string
	_ = json.Unmarshal([]byte(d.CodeSnippetsJSON), &snips)
	meta, _ := leetcode.ParseMetaData(d.MetaJSON)

	q := leetcode.Question{
		FrontendID:       row.FrontendID,
		QuestionID:       d.QuestionID,
		Slug:             d.Slug,
		Title:            firstNonEmpty(row.Title, d.Slug),
		Difficulty:       row.Difficulty,
		Statement:        d.StatementMD,
		Meta:             meta,
		CodeSnippets:     snips,
		MetaData:         d.MetaJSON,
		ExampleTestcases: d.ExampleTestcases,
	}
	if meta.Arity() > 0 {
		if cases, err := testcase.FromLeetCodeExample(d.ExampleTestcases, meta.Arity()); err == nil {
			q.ExampleCases = cases
		}
	}
	// Prefer the cached cases with scraped expected outputs, if we have them.
	var stored []testcase.Case
	if json.Unmarshal([]byte(orDefault(d.ExampleCasesJSON, "[]")), &stored) == nil && len(stored) > 0 {
		q.ExampleCases = stored
	}
	return q
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// exampleOutputsMissing reports whether q has function-style examples but none
// of them carry an expected output (so the local judge can't verify anything).
func exampleOutputsMissing(q leetcode.Question) bool {
	if q.Meta.Arity() == 0 || len(q.ExampleCases) == 0 {
		return false
	}
	for _, c := range q.ExampleCases {
		if c.Out != "" {
			return false
		}
	}
	return true
}

// pickLanguage chooses a fallback language when the requested one has no
// snippet: prefer the request, then python3, then the first alphabetically.
func pickLanguage(snippets map[string]string, want string) string {
	if _, ok := snippets[want]; ok {
		return want
	}
	if _, ok := snippets["python3"]; ok {
		return "python3"
	}
	keys := make([]string, 0, len(snippets))
	for k := range snippets {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return keys[0]
}
