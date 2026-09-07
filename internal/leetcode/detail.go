package leetcode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/sven97/lazyleet/internal/testcase"
)

type questionDataResp struct {
	Question *struct {
		QuestionID       json.Number `json:"questionId"`
		FrontendID       string      `json:"frontendId"`
		Title            string      `json:"title"`
		Slug             string      `json:"slug"`
		Difficulty       string      `json:"difficulty"`
		PaidOnly         bool        `json:"paidOnly"`
		Status           *string     `json:"status"`
		Content          string      `json:"content"`
		ExampleTestcases string      `json:"exampleTestcases"`
		SampleTestCase   string      `json:"sampleTestCase"`
		MetaData         string      `json:"metaData"`
		Hints            []string    `json:"hints"`
		SimilarQuestions string      `json:"similarQuestions"`
		TopicTags        []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"topicTags"`
		CodeSnippets []struct {
			LangSlug string `json:"langSlug"`
			Code     string `json:"code"`
		} `json:"codeSnippets"`
	} `json:"question"`
}

// QuestionDetail fetches the full detail for one problem and adapts it into the
// Question type the workspace uses. The HTML statement is converted to Markdown.
func (c *Client) QuestionDetail(ctx context.Context, slug string) (Question, error) {
	var resp questionDataResp
	if err := c.graphql(ctx, "questionData", qQuestionData, map[string]any{"titleSlug": slug}, &resp); err != nil {
		return Question{}, err
	}
	if resp.Question == nil {
		return Question{}, &APIError{Op: "questionData", Message: fmt.Sprintf("no such problem: %q", slug)}
	}
	q := resp.Question

	if q.PaidOnly && strings.TrimSpace(q.Content) == "" {
		return Question{}, &APIError{Op: "questionData", Message: fmt.Sprintf("%q is a paid-only problem; content is not available", slug)}
	}

	meta, metaErr := parseMeta(q.MetaData)

	statement := q.Content
	if md, err := htmltomarkdown.ConvertString(q.Content); err == nil {
		statement = md
	}

	snippets := make(map[string]string, len(q.CodeSnippets))
	for _, s := range q.CodeSnippets {
		snippets[s.LangSlug] = s.Code
	}

	out := Question{
		FrontendID:       parseFrontendID(q.FrontendID),
		QuestionID:       atoiSafe(q.QuestionID.String()),
		Slug:             q.Slug,
		Title:            q.Title,
		Difficulty:       titleCaseDifficulty(q.Difficulty),
		Statement:        strings.TrimSpace(statement) + "\n",
		Meta:             meta,
		CodeSnippets:     snippets,
		MetaData:         q.MetaData,
		ExampleTestcases: q.ExampleTestcases,
	}

	if metaErr == nil && meta.Arity() > 0 {
		if cases, err := testcase.FromLeetCodeExample(q.ExampleTestcases, meta.Arity()); err == nil {
			out.ExampleCases = cases
		}
	}
	return out, nil
}

// rawMeta mirrors LeetCode's metaData JSON string for the function-style form.
type rawMeta struct {
	Name   string `json:"name"`
	Params []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"params"`
	Return struct {
		Type string `json:"type"`
	} `json:"return"`
	// Design problems set these instead; handled in Phase 5.
	ClassName string `json:"classname"`
}

// ParseMetaData parses a LeetCode metaData JSON string into a Meta. A design
// problem (no single entry point) yields a Meta with an empty Name.
func ParseMetaData(s string) (Meta, error) { return parseMeta(s) }

func parseMeta(s string) (Meta, error) {
	if strings.TrimSpace(s) == "" {
		return Meta{}, fmt.Errorf("empty metaData")
	}
	var rm rawMeta
	if err := json.Unmarshal([]byte(s), &rm); err != nil {
		return Meta{}, err
	}
	m := Meta{Name: rm.Name, Return: MetaType{Type: rm.Return.Type}}
	for _, p := range rm.Params {
		m.Params = append(m.Params, MetaParam{Name: p.Name, Type: p.Type})
	}
	if m.Name == "" && rm.ClassName != "" {
		// Design problem: no single entry point. Leave Name empty; the runner
		// will report this is unsupported until Phase 5.
		m.Name = ""
	}
	return m, nil
}
