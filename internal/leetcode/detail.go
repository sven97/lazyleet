package leetcode

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/sven97/lazyleet/internal/testcase"
)

var (
	supRe = regexp.MustCompile(`(?is)<sup>\s*(.*?)\s*</sup>`)
	subRe = regexp.MustCompile(`(?is)<sub>\s*(.*?)\s*</sub>`)
)

// Unicode super/subscript forms for the characters that actually turn up in
// LeetCode statements (digits and a few operators). Anything outside these maps
// falls back to caret / underscore notation.
var (
	superscriptRunes = map[rune]rune{
		'0': '⁰', '1': '¹', '2': '²', '3': '³', '4': '⁴', '5': '⁵', '6': '⁶', '7': '⁷', '8': '⁸', '9': '⁹',
		'+': '⁺', '-': '⁻', '=': '⁼', '(': '⁽', ')': '⁾', 'n': 'ⁿ', 'i': 'ⁱ',
	}
	subscriptRunes = map[rune]rune{
		'0': '₀', '1': '₁', '2': '₂', '3': '₃', '4': '₄', '5': '₅', '6': '₆', '7': '₇', '8': '₈', '9': '₉',
		'+': '₊', '-': '₋', '=': '₌', '(': '₍', ')': '₎',
	}
)

func mapRunes(s string, table map[rune]rune) (string, bool) {
	if s == "" {
		return "", false
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		m, ok := table[r]
		if !ok {
			return "", false
		}
		out = append(out, m)
	}
	return string(out), true
}

// preprocessStatementHTML rewrites tags the HTML→Markdown converter would
// otherwise flatten lossily. LeetCode writes exponents as <sup> (e.g.
// "10<sup>15</sup>"), which the converter drops, turning it into "1015". We
// render true Unicode superscript when every character maps (the common
// digit-only case → "10¹⁵"), and fall back to "^15" / "_2" otherwise.
func preprocessStatementHTML(h string) string {
	repl := func(re *regexp.Regexp, table map[rune]rune, prefix string) {
		h = re.ReplaceAllStringFunc(h, func(match string) string {
			inner := strings.TrimSpace(re.FindStringSubmatch(match)[1])
			if u, ok := mapRunes(inner, table); ok {
				return u
			}
			return prefix + inner
		})
	}
	for i := 0; i < 5; i++ { // a few passes to unwrap the rare nested tag
		before := h
		repl(supRe, superscriptRunes, "^")
		repl(subRe, subscriptRunes, "_")
		if h == before {
			break
		}
	}
	return h
}

type questionDataResp struct {
	Question *struct {
		QuestionID       json.Number `json:"questionId"`
		FrontendID       string      `json:"frontendId"`
		Title            string      `json:"title"`
		Slug             string      `json:"slug"`
		Difficulty       string      `json:"difficulty"`
		ACRate           float64     `json:"acRate"`
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
	if md, err := htmltomarkdown.ConvertString(preprocessStatementHTML(q.Content)); err == nil {
		statement = md
	}

	snippets := make(map[string]string, len(q.CodeSnippets))
	for _, s := range q.CodeSnippets {
		snippets[s.LangSlug] = s.Code
	}

	tags := make([]string, 0, len(q.TopicTags))
	for _, t := range q.TopicTags {
		tags = append(tags, t.Name)
	}

	out := Question{
		FrontendID:       parseFrontendID(q.FrontendID),
		QuestionID:       atoiSafe(q.QuestionID.String()),
		Slug:             q.Slug,
		Title:            q.Title,
		Difficulty:       titleCaseDifficulty(q.Difficulty),
		ACRate:           q.ACRate,
		PaidOnly:         q.PaidOnly,
		Tags:             tags,
		Statement:        strings.TrimSpace(statement) + "\n",
		Meta:             meta,
		CodeSnippets:     snippets,
		MetaData:         q.MetaData,
		ExampleTestcases: q.ExampleTestcases,
	}

	if metaErr == nil && meta.Arity() > 0 {
		if cases, err := testcase.FromLeetCodeExample(q.ExampleTestcases, meta.Arity()); err == nil {
			// exampleTestcases is inputs only; the expected outputs live in the
			// statement prose ("Output: 3"). Attach them when the count lines up
			// exactly, so the local judge can actually verify the examples.
			if outs := parseExampleOutputs(q.Content); len(outs) == len(cases) {
				for i := range cases {
					cases[i].Out = outs[i]
				}
			}
			out.ExampleCases = cases
		}
	}
	return out, nil
}

// exampleOutputRe pulls the literal after an "Output:" label out of a LeetCode
// statement, tolerating both the old <pre> markup ("<strong>Output:</strong> 3")
// and the newer example-block markup ("...</strong> <span ...>3</span>").
var exampleOutputRe = regexp.MustCompile(`(?i)Output:\s*(?:</strong>)?\s*(?:<[^>]+>\s*)?([^<\n]+)`)

// parseExampleOutputs returns the expected-output literals from a statement, in
// document order.
func parseExampleOutputs(statementHTML string) []string {
	ms := exampleOutputRe.FindAllStringSubmatch(statementHTML, -1)
	outs := make([]string, 0, len(ms))
	for _, m := range ms {
		v := strings.TrimSpace(html.UnescapeString(m[1]))
		if v == "" {
			return nil // ambiguous — better to attach nothing
		}
		outs = append(outs, v)
	}
	return outs
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
