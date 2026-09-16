package leetcode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// gqlHandler builds a test server that dispatches on the GraphQL operationName.
func gqlServer(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql/" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req gqlRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, ok := responses[req.OperationName]
		if !ok {
			http.Error(w, "no canned response for "+req.OperationName, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testClient(t *testing.T, srv *httptest.Server, opts ...Option) *Client {
	all := append([]Option{
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRateLimit(1e6, 1),
		WithRetryBackoff(time.Millisecond),
	}, opts...)
	return New("com", all...)
}

func TestListProblems(t *testing.T) {
	const payload = `{"data":{"problemsetQuestionList":{"total":2,"questions":[
		{"frontendId":"1","title":"Two Sum","slug":"two-sum","difficulty":"Easy","acRate":52.3,"paidOnly":false,"status":null,"topicTags":[{"slug":"array"},{"slug":"hash-table"}]},
		{"frontendId":"4","title":"Median of Two Sorted Arrays","slug":"median-of-two-sorted-arrays","difficulty":"HARD","acRate":41.1,"paidOnly":false,"status":"ac","topicTags":[]}
	]}}}`
	srv := gqlServer(t, map[string]string{"problemsetQuestionList": payload})
	c := testClient(t, srv)

	ps, total, err := c.ListProblems(context.Background(), ProblemFilter{}, 0, 100)
	if err != nil {
		t.Fatalf("ListProblems: %v", err)
	}
	if total != 2 || len(ps) != 2 {
		t.Fatalf("got %d/%d", len(ps), total)
	}
	if ps[0].FrontendID != 1 || ps[0].Difficulty != "Easy" || ps[0].Status != "" {
		t.Errorf("row0 = %+v", ps[0])
	}
	if len(ps[0].TopicTags) != 2 || ps[0].TopicTags[1] != "hash-table" {
		t.Errorf("row0 tags = %v", ps[0].TopicTags)
	}
	if ps[1].Difficulty != "Hard" || ps[1].Status != "ac" {
		t.Errorf("row1 = %+v", ps[1])
	}
}

func TestQuestionDetail(t *testing.T) {
	const payload = `{"data":{"question":{
		"questionId":"1","frontendId":"1","title":"Two Sum","slug":"two-sum","difficulty":"Easy","paidOnly":false,"status":null,
		"content":"<p>Given an array <code>nums</code>.</p><ul><li>one</li><li>two</li></ul>",
		"exampleTestcases":"[2,7,11,15]\n9\n[3,2,4]\n6",
		"sampleTestCase":"[2,7,11,15]\n9",
		"metaData":"{\"name\":\"twoSum\",\"params\":[{\"name\":\"nums\",\"type\":\"integer[]\"},{\"name\":\"target\",\"type\":\"integer\"}],\"return\":{\"type\":\"integer[]\"}}",
		"hints":["<p>use a <code>map</code></p>", " ", "Try O(n<sup>2</sup>) first."],
		"similarQuestions":"[]",
		"topicTags":[{"slug":"array","name":"Array"}],
		"codeSnippets":[{"langSlug":"python3","code":"class Solution:\n    pass"},{"langSlug":"go","code":"func twoSum() {}"}]
	}}}`
	srv := gqlServer(t, map[string]string{"questionData": payload})
	c := testClient(t, srv)

	q, err := c.QuestionDetail(context.Background(), "two-sum")
	if err != nil {
		t.Fatalf("QuestionDetail: %v", err)
	}
	if len(q.Hints) != 2 || !strings.Contains(q.Hints[0], "map") || strings.Contains(q.Hints[0], "<code>") || !strings.Contains(q.Hints[1], "n²") {
		t.Fatalf("hints missing or not converted: %q", q.Hints)
	}
	if q.QuestionID != 1 || q.FrontendID != 1 || q.Meta.Name != "twoSum" || q.Meta.Arity() != 2 {
		t.Fatalf("meta wrong: %+v", q)
	}
	if strings.Contains(q.Statement, "<p>") || strings.Contains(q.Statement, "<code>") {
		t.Errorf("statement not converted from HTML: %q", q.Statement)
	}
	if _, ok := q.CodeSnippets["python3"]; !ok {
		t.Errorf("missing python3 snippet: %v", q.CodeSnippets)
	}
	if len(q.ExampleCases) != 2 || q.ExampleCases[0].In[0] != "[2,7,11,15]" || q.ExampleCases[0].In[1] != "9" {
		t.Errorf("example cases wrong: %+v", q.ExampleCases)
	}
	if q.ExampleCases[0].Out != "" {
		t.Errorf("example cases should have no expected output, got %q", q.ExampleCases[0].Out)
	}
}

func TestStudyPlanDetail(t *testing.T) {
	const payload = `{"data":{"studyPlanV2Detail":{
		"slug":"leetcode-75","name":"LeetCode 75",
		"planSubGroups":[
			{"slug":"array-string","name":"Array / String","questions":[
				{"slug":"merge-strings-alternately","frontendId":"1768","title":"Merge Strings Alternately","difficulty":"Easy"}
			]},
			{"slug":"two-pointers","name":"Two Pointers","questions":[
				{"slug":"move-zeroes","frontendId":"283","title":"Move Zeroes","difficulty":"Easy"}
			]}
		]
	}}}`
	srv := gqlServer(t, map[string]string{"studyPlanV2Detail": payload})
	c := testClient(t, srv)

	plan, err := c.StudyPlanDetail(context.Background(), "leetcode-75")
	if err != nil {
		t.Fatalf("StudyPlanDetail: %v", err)
	}
	if plan.Name != "LeetCode 75" || len(plan.Questions) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Questions[0].Group != "Array / String" || plan.Questions[1].Slug != "move-zeroes" {
		t.Errorf("questions = %+v", plan.Questions)
	}
}

func TestGraphQLErrorSurfacesAsAPIError(t *testing.T) {
	srv := gqlServer(t, map[string]string{"problemsetQuestionList": `{"errors":[{"message":"boom"}]}`})
	c := testClient(t, srv)

	_, _, err := c.ListProblems(context.Background(), ProblemFilter{}, 0, 10)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Message, "boom") {
		t.Fatalf("want APIError containing boom, got %v", err)
	}
}

func TestRetryThenSuccess(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			http.Error(w, "try later", http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, `{"data":{"userStatus":{"isSignedIn":true,"username":"sven"}}}`)
	}))
	t.Cleanup(srv.Close)
	c := testClient(t, srv)

	name, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI after retries: %v", err)
	}
	if name != "sven" || hits != 3 {
		t.Fatalf("name=%q hits=%d", name, hits)
	}
}

func TestAuthHeadersSentWhenCredentialed(t *testing.T) {
	var gotCookie, gotCSRF, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotCSRF = r.Header.Get("X-Csrftoken")
		gotReferer = r.Header.Get("Referer")
		io.WriteString(w, `{"data":{"userStatus":{"isSignedIn":true,"username":"sven"}}}`)
	}))
	t.Cleanup(srv.Close)
	c := testClient(t, srv, WithCredentials(Credentials{Session: "sess", CSRFToken: "tok"}))

	if _, err := c.WhoAmI(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotCookie, "LEETCODE_SESSION=sess") || gotCSRF != "tok" {
		t.Errorf("auth headers missing: cookie=%q csrf=%q", gotCookie, gotCSRF)
	}
	if gotReferer == "" {
		t.Errorf("Referer not set")
	}
}
