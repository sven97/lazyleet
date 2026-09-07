// Package workspace manages the on-disk directory for solving one problem:
//
//	<root>/<frontendID>-<slug>/
//	  solution.<ext>     starter code (seeded once from the LeetCode snippet)
//	  testcases.jsonl    test cases (seeded from the statement's worked examples)
//	  meta.json          problem + language metadata
//	  notes.md           scratch notes
//
// Scaffolding never clobbers an existing solution or notes file, so reopening a
// problem resumes where you left off.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/testcase"
)

// Workspace is a scaffolded problem directory.
type Workspace struct {
	Dir          string
	Slug         string
	Lang         string
	SolutionPath string
	TestsPath    string
	MetaPath     string
	NotesPath    string
}

// meta is the JSON written to meta.json.
type meta struct {
	Slug       string        `json:"slug"`
	Title      string        `json:"title"`
	FrontendID int           `json:"frontend_id"`
	QuestionID int           `json:"question_id"`
	Difficulty string        `json:"difficulty"`
	Lang       string        `json:"lang"`
	Meta       leetcode.Meta `json:"meta"`
	CreatedAt  int64         `json:"created_at"`
}

// Scaffold creates (or reopens) the workspace directory for q in the given
// language and returns it. lang must be a key of q.CodeSnippets.
func Scaffold(root string, q leetcode.Question, lang string) (*Workspace, error) {
	snippet, ok := q.CodeSnippets[lang]
	if !ok {
		return nil, fmt.Errorf("no %s snippet for %s (have: %v)", lang, q.Slug, keys(q.CodeSnippets))
	}
	ext, ok := extForLang(lang)
	if !ok {
		return nil, fmt.Errorf("unsupported language %q", lang)
	}

	dir := filepath.Join(root, fmt.Sprintf("%d-%s", q.FrontendID, q.Slug))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	w := &Workspace{
		Dir:          dir,
		Slug:         q.Slug,
		Lang:         lang,
		SolutionPath: filepath.Join(dir, "solution."+ext),
		TestsPath:    filepath.Join(dir, "testcases.jsonl"),
		MetaPath:     filepath.Join(dir, "meta.json"),
		NotesPath:    filepath.Join(dir, "notes.md"),
	}

	if err := writeIfAbsent(w.SolutionPath, []byte(snippet)); err != nil {
		return nil, err
	}
	if err := writeIfAbsent(w.NotesPath, []byte("# Notes — "+q.Title+"\n\n")); err != nil {
		return nil, err
	}
	if err := writeIfAbsent(w.TestsPath, renderCases(q.ExampleCases)); err != nil {
		return nil, err
	}

	// meta.json is always refreshed — it is derived, never user-edited.
	m := meta{
		Slug: q.Slug, Title: q.Title, FrontendID: q.FrontendID, QuestionID: q.QuestionID,
		Difficulty: q.Difficulty, Lang: lang, Meta: q.Meta, CreatedAt: time.Now().Unix(),
	}
	blob, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(w.MetaPath, append(blob, '\n'), 0o644); err != nil {
		return nil, err
	}

	return w, nil
}

// ReadSolution returns the current contents of the solution file.
func (w *Workspace) ReadSolution() (string, error) {
	b, err := os.ReadFile(w.SolutionPath)
	return string(b), err
}

// ReadCases parses the test-case file.
func (w *Workspace) ReadCases() ([]testcase.Case, error) {
	f, err := os.Open(w.TestsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return testcase.Parse(f)
}

func renderCases(cs []testcase.Case) []byte {
	var b []byte
	buf := &byteWriter{&b}
	_ = testcase.Write(buf, cs)
	return b
}

type byteWriter struct{ p *[]byte }

func (w *byteWriter) Write(p []byte) (int, error) { *w.p = append(*w.p, p...); return len(p), nil }

func writeIfAbsent(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already there — keep the user's work
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func extForLang(lang string) (string, bool) {
	switch lang {
	case "python3", "python":
		return "py", true
	case "javascript":
		return "js", true
	case "typescript":
		return "ts", true
	case "golang", "go":
		return "go", true
	case "java":
		return "java", true
	case "cpp", "c++":
		return "cpp", true
	case "c":
		return "c", true
	case "rust":
		return "rs", true
	default:
		return "", false
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
