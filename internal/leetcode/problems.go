package leetcode

import "context"

// ProblemSummary is one row of the problem list.
type ProblemSummary struct {
	FrontendID  int    // parsed from the (usually numeric) frontend id string
	FrontendRaw string // original, e.g. "1" or "LCP 01"
	Slug        string
	Title       string
	Difficulty  string  // Easy | Medium | Hard
	ACRate      float64 // acceptance percentage, 0..100
	PaidOnly    bool
	Status      string   // "" | ac | notac  (empty when anonymous)
	TopicTags   []string // tag slugs
}

// ProblemFilter narrows the problem list. Zero value means "everything".
type ProblemFilter struct {
	Difficulty string   // "EASY" | "MEDIUM" | "HARD"
	Status     string   // "AC" | "NOT_STARTED" | "TRIED"
	Tags       []string // topic tag slugs (AND-ed by LeetCode)
	Search     string   // free-text (matches id or title)
	OrderBy    string   // optional: "FRONTEND_ID" | "AC_RATE" | "DIFFICULTY" | "FREQUENCY"
	SortOrder  string   // optional: "ASCENDING" | "DESCENDING"
}

func (f ProblemFilter) toVars() map[string]any {
	m := map[string]any{}
	if f.Difficulty != "" {
		m["difficulty"] = f.Difficulty
	}
	if f.Status != "" {
		m["status"] = f.Status
	}
	if len(f.Tags) > 0 {
		m["tags"] = f.Tags
	}
	if f.Search != "" {
		m["searchKeywords"] = f.Search
	}
	if f.OrderBy != "" {
		m["orderBy"] = f.OrderBy
	}
	if f.SortOrder != "" {
		m["sortOrder"] = f.SortOrder
	}
	return m
}

type problemListResp struct {
	Wrap struct {
		Total     int `json:"total"`
		Questions []struct {
			FrontendID string  `json:"frontendId"`
			Title      string  `json:"title"`
			Slug       string  `json:"slug"`
			Difficulty string  `json:"difficulty"`
			ACRate     float64 `json:"acRate"`
			PaidOnly   bool    `json:"paidOnly"`
			Status     *string `json:"status"`
			TopicTags  []struct {
				Slug string `json:"slug"`
			} `json:"topicTags"`
		} `json:"questions"`
	} `json:"problemsetQuestionList"`
}

// ListProblems fetches one page of the problem list. skip/limit paginate;
// LeetCode caps limit near 100. The returned total is the full filtered count.
func (c *Client) ListProblems(ctx context.Context, filter ProblemFilter, skip, limit int) (problems []ProblemSummary, total int, err error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	vars := map[string]any{
		"categorySlug": "",
		"skip":         skip,
		"limit":        limit,
		"filters":      filter.toVars(),
	}

	var resp problemListResp
	if err := c.graphql(ctx, "problemsetQuestionList", qProblemList, vars, &resp); err != nil {
		return nil, 0, err
	}

	out := make([]ProblemSummary, 0, len(resp.Wrap.Questions))
	for _, q := range resp.Wrap.Questions {
		p := ProblemSummary{
			FrontendID:  parseFrontendID(q.FrontendID),
			FrontendRaw: q.FrontendID,
			Slug:        q.Slug,
			Title:       q.Title,
			Difficulty:  titleCaseDifficulty(q.Difficulty),
			ACRate:      q.ACRate,
			PaidOnly:    q.PaidOnly,
		}
		if q.Status != nil {
			p.Status = *q.Status
		}
		for _, t := range q.TopicTags {
			p.TopicTags = append(p.TopicTags, t.Slug)
		}
		out = append(out, p)
	}
	return out, resp.Wrap.Total, nil
}

// ListAllProblems pages through the entire filtered list, calling onPage after
// each page with the running total fetched so far (for progress reporting).
func (c *Client) ListAllProblems(ctx context.Context, filter ProblemFilter, onPage func(fetched, total int)) ([]ProblemSummary, error) {
	const page = 100
	var all []ProblemSummary
	for skip := 0; ; skip += page {
		batch, total, err := c.ListProblems(ctx, filter, skip, page)
		if err != nil {
			return all, err
		}
		all = append(all, batch...)
		if onPage != nil {
			onPage(len(all), total)
		}
		if len(batch) < page || len(all) >= total {
			return all, nil
		}
	}
}

// CatalogHead snapshots the catalog's total problem count and its highest
// frontend id in a single request by listing one problem ordered by
// FRONTEND_ID descending. Comparing both against the local cache detects
// newly added problems even when the total is unchanged (an add plus a
// removal), which a count-only check misses. maxFrontendID is 0 when the
// catalog is empty.
func (c *Client) CatalogHead(ctx context.Context) (total, maxFrontendID int, err error) {
	ps, total, err := c.ListProblems(ctx, ProblemFilter{OrderBy: "FRONTEND_ID", SortOrder: "DESCENDING"}, 0, 1)
	if err != nil {
		return 0, 0, err
	}
	if len(ps) > 0 {
		maxFrontendID = ps[0].FrontendID
	}
	return total, maxFrontendID, nil
}
