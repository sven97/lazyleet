package main

import (
	"context"
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
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
			slug := args[0]

			q, src, err := app.resolveQuestion(cmd.Context(), slug, refresh)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "loaded %s (%s)\n", slug, src)

			if err := app.paths.EnsureDirs(); err != nil {
				return err
			}
			if lang == "" {
				lang = app.cfg.DefaultLanguage
			}
			if _, ok := q.CodeSnippets[lang]; !ok {
				return fmt.Errorf("no %s starter for %s; available: %v", lang, slug, sortedKeys(q.CodeSnippets))
			}

			ws, err := workspace.Scaffold(app.paths.WorkspaceRoot, q, lang)
			if err != nil {
				return err
			}

			m, err := tui.NewWorkspaceModel(
				ws, q,
				app.cfg.ResolveEditor(),
				app.cfg.Workspace.RunOnSave,
				app.cfg.Workspace.RunDebounceMs,
			)
			if err != nil {
				return err
			}

			p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
			_, err = p.Run()
			return err
		},
	}

	cmd.Flags().StringVar(&lang, "lang", "", "language slug (default: config default_language)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "bypass the local cache and refetch from LeetCode")
	return cmd
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
			return questionFromCache(d, row), "cache", nil
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
		fmt.Println("warning: could not cache problem detail:", err)
	}
	return q, "leetcode", nil
}

func cacheFromQuestion(q leetcode.Question) store.ProblemDetail {
	snips, _ := json.Marshal(q.CodeSnippets)
	return store.ProblemDetail{
		Slug:             q.Slug,
		QuestionID:       q.QuestionID,
		StatementMD:      q.Statement,
		MetaJSON:         q.MetaData,
		ExampleTestcases: q.ExampleTestcases,
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
	return q
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
