package leetcode

import (
	"context"
	"fmt"
)

// StudyPlan is an ordered list of problems under a named plan.
type StudyPlan struct {
	Slug      string
	Name      string
	Source    string // "leetcode" | "bundled"
	Questions []StudyPlanQuestion
}

// StudyPlanQuestion is one entry in a plan.
type StudyPlanQuestion struct {
	Slug       string
	FrontendID int
	Title      string
	Difficulty string
	Group      string // sub-group name, e.g. "Array / String"
}

type studyPlanResp struct {
	Detail *struct {
		Slug          string `json:"slug"`
		Name          string `json:"name"`
		PlanSubGroups []struct {
			Slug      string `json:"slug"`
			Name      string `json:"name"`
			Questions []struct {
				Slug       string `json:"slug"`
				FrontendID string `json:"frontendId"`
				Title      string `json:"title"`
				Difficulty string `json:"difficulty"`
			} `json:"questions"`
		} `json:"planSubGroups"`
	} `json:"studyPlanV2Detail"`
}

// StudyPlanDetail fetches an official LeetCode study plan by slug, e.g.
// "leetcode-75" or "top-interview-150".
func (c *Client) StudyPlanDetail(ctx context.Context, slug string) (StudyPlan, error) {
	var resp studyPlanResp
	if err := c.graphql(ctx, "studyPlanV2Detail", qStudyPlanDetail, map[string]any{"planSlug": slug}, &resp); err != nil {
		return StudyPlan{}, err
	}
	if resp.Detail == nil {
		return StudyPlan{}, &APIError{Op: "studyPlanV2Detail", Message: fmt.Sprintf("no such study plan: %q", slug)}
	}

	plan := StudyPlan{Slug: resp.Detail.Slug, Name: resp.Detail.Name, Source: "leetcode"}
	for _, g := range resp.Detail.PlanSubGroups {
		for _, q := range g.Questions {
			plan.Questions = append(plan.Questions, StudyPlanQuestion{
				Slug:       q.Slug,
				FrontendID: parseFrontendID(q.FrontendID),
				Title:      q.Title,
				Difficulty: titleCaseDifficulty(q.Difficulty),
				Group:      g.Name,
			})
		}
	}
	return plan, nil
}
