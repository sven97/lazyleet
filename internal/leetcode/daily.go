package leetcode

import "context"

// DailyChallenge is LeetCode's "Question of Today".
type DailyChallenge struct {
	Date       string // YYYY-MM-DD, LeetCode's UTC-ish challenge date
	Slug       string
	Question   ProblemSummary
	UserStatus string // "Complete" once the signed-in user has solved it today
}

// Done reports whether the signed-in user has completed today's challenge.
func (d DailyChallenge) Done() bool { return d.UserStatus == "Complete" }

// Streak is the signed-in user's daily-challenge streak.
type Streak struct {
	Count          int
	DaysSkipped    int
	CurrentDayDone bool
}

const qDailyQuestion = `
query questionOfToday {
  activeDailyCodingChallengeQuestion {
    date
    userStatus
    question {
      frontendId: questionFrontendId
      title
      slug: titleSlug
      difficulty
      acRate
      paidOnly: isPaidOnly
      status
      topicTags { slug }
    }
  }
}`

const qStreakCounter = `
query getStreakCounter {
  streakCounter {
    streakCount
    daysSkipped
    currentDayCompleted
  }
}`

type dailyResp struct {
	Active struct {
		Date       string `json:"date"`
		UserStatus string `json:"userStatus"`
		Question   struct {
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
		} `json:"question"`
	} `json:"activeDailyCodingChallengeQuestion"`
}

// DailyQuestion returns today's daily coding challenge. userStatus is populated
// only when the client is authenticated.
func (c *Client) DailyQuestion(ctx context.Context) (DailyChallenge, error) {
	var resp dailyResp
	if err := c.graphql(ctx, "questionOfToday", qDailyQuestion, nil, &resp); err != nil {
		return DailyChallenge{}, err
	}
	q := resp.Active.Question
	sum := ProblemSummary{
		FrontendID:  parseFrontendID(q.FrontendID),
		FrontendRaw: q.FrontendID,
		Slug:        q.Slug,
		Title:       q.Title,
		Difficulty:  titleCaseDifficulty(q.Difficulty),
		ACRate:      q.ACRate,
		PaidOnly:    q.PaidOnly,
	}
	if q.Status != nil {
		sum.Status = *q.Status
	}
	for _, t := range q.TopicTags {
		sum.TopicTags = append(sum.TopicTags, t.Slug)
	}
	return DailyChallenge{
		Date:       resp.Active.Date,
		Slug:       q.Slug,
		Question:   sum,
		UserStatus: resp.Active.UserStatus,
	}, nil
}

// DailyStreak returns the signed-in user's daily-challenge streak. It returns a
// zero Streak (no error) when the client is anonymous or the account has no
// streak record yet.
func (c *Client) DailyStreak(ctx context.Context) (Streak, error) {
	if !c.Authenticated() {
		return Streak{}, nil
	}
	var resp struct {
		SC *struct {
			StreakCount         int  `json:"streakCount"`
			DaysSkipped         int  `json:"daysSkipped"`
			CurrentDayCompleted bool `json:"currentDayCompleted"`
		} `json:"streakCounter"`
	}
	if err := c.graphql(ctx, "getStreakCounter", qStreakCounter, nil, &resp); err != nil {
		return Streak{}, err
	}
	if resp.SC == nil {
		return Streak{}, nil
	}
	return Streak{
		Count:          resp.SC.StreakCount,
		DaysSkipped:    resp.SC.DaysSkipped,
		CurrentDayDone: resp.SC.CurrentDayCompleted,
	}, nil
}
