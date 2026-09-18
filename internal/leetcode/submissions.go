package leetcode

import "context"

// Submission is one row from LeetCode's own submission history for a problem
// (the signed-in account's server-side record) — distinct from lazyleet's
// local attempt log (internal/attempt), which also covers un-submitted local
// runs and survives even when the account has no history for a problem yet.
//
// Verified against live LeetCode 2026-09-17 via `lazyleet debug submissions`
// (account has solved problems across python3/python/java): every field
// below decoded correctly with real data. `lastKey` came back "" on every
// call regardless of `hasNext`, including on a page proven (by exhausting a
// 5-submission history via 3+3 offset paging) to have more rows after it —
// so it is not a working cursor for this query, at least not for this
// account/region. Paging is plain offset/limit instead; HasNext tracked that
// correctly (true at offset 0 of 5, false once offset+limit reached the end).
type Submission struct {
	RemoteID      string
	StatusDisplay string // "Accepted", "Wrong Answer", "Time Limit Exceeded", ...
	Lang          string // langSlug, e.g. "python3"
	LangName      string
	Runtime       string
	Memory        string
	Timestamp     int64 // unix seconds
	URL           string
	IsPending     bool
	Notes         string
}

// Accepted reports whether this submission was judged Accepted.
func (s Submission) Accepted() bool { return s.StatusDisplay == "Accepted" }

// SubmissionPage is one page of a user's submission history for a problem,
// newest first. LastKey is decoded for completeness but has been observed
// empty even when HasNext is true — page with Offset/limit, not LastKey.
type SubmissionPage struct {
	Submissions []Submission
	LastKey     string
	HasNext     bool
}

// flexBool decodes a JSON bool, or a string LeetCode might send instead
// (e.g. "Not Pending"/"Pending"), tolerantly. Same rationale as flexInt in
// submit.go: this API's field types drift across endpoints.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	s := string(b)
	*f = s == "true" || s == `"Pending"`
	return nil
}

const qSubmissionList = `
query submissionList($questionSlug: String!, $offset: Int!, $limit: Int!) {
  questionSubmissionList(questionSlug: $questionSlug, offset: $offset, limit: $limit) {
    lastKey
    hasNext
    submissions {
      id
      statusDisplay
      lang
      langName
      runtime
      memory
      timestamp
      url
      isPending
      notes
    }
  }
}`

// SubmissionList fetches one page (offset/limit) of the signed-in user's
// submission history for slug from LeetCode itself, newest first. Keep
// bumping offset by limit while the returned HasNext is true.
func (c *Client) SubmissionList(ctx context.Context, slug string, limit, offset int) (SubmissionPage, error) {
	if !c.Authenticated() {
		return SubmissionPage{}, &APIError{Op: "questionSubmissionList", Message: "not authenticated (run `lazyleet auth`)"}
	}
	if limit <= 0 {
		limit = 20
	}
	vars := map[string]any{
		"questionSlug": slug,
		"offset":       offset,
		"limit":        limit,
	}
	var resp struct {
		List struct {
			LastKey     string `json:"lastKey"`
			HasNext     bool   `json:"hasNext"`
			Submissions []struct {
				ID            string   `json:"id"`
				StatusDisplay string   `json:"statusDisplay"`
				Lang          string   `json:"lang"`
				LangName      string   `json:"langName"`
				Runtime       string   `json:"runtime"`
				Memory        string   `json:"memory"`
				Timestamp     flexInt  `json:"timestamp"`
				URL           string   `json:"url"`
				IsPending     flexBool `json:"isPending"`
				Notes         string   `json:"notes"`
			} `json:"submissions"`
		} `json:"questionSubmissionList"`
	}
	if err := c.graphql(ctx, "submissionList", qSubmissionList, vars, &resp); err != nil {
		return SubmissionPage{}, err
	}
	page := SubmissionPage{LastKey: resp.List.LastKey, HasNext: resp.List.HasNext}
	for _, s := range resp.List.Submissions {
		page.Submissions = append(page.Submissions, Submission{
			RemoteID:      s.ID,
			StatusDisplay: s.StatusDisplay,
			Lang:          s.Lang,
			LangName:      s.LangName,
			Runtime:       s.Runtime,
			Memory:        s.Memory,
			Timestamp:     int64(s.Timestamp),
			URL:           s.URL,
			IsPending:     bool(s.IsPending),
			Notes:         s.Notes,
		})
	}
	return page, nil
}
