package leetcode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmissionList(t *testing.T) {
	const payload = `{"data":{"questionSubmissionList":{"lastKey":"","hasNext":true,"submissions":[
		{"id":"1234","statusDisplay":"Accepted","lang":"python3","langName":"Python3","runtime":"52 ms","memory":"14.2 MB","timestamp":"1700000000","url":"/submissions/detail/1234/","isPending":"Not Pending","notes":""},
		{"id":"1200","statusDisplay":"Wrong Answer","lang":"python3","langName":"Python3","runtime":"N/A","memory":"N/A","timestamp":"1699999000","url":"/submissions/detail/1200/","isPending":"Not Pending","notes":""}
	]}}}`
	srv := gqlServer(t, map[string]string{"submissionList": payload})
	c := testClient(t, srv, WithCredentials(Credentials{Session: "sess", CSRFToken: "tok"}))

	page, err := c.SubmissionList(context.Background(), "two-sum", 20, 0)
	if err != nil {
		t.Fatalf("SubmissionList: %v", err)
	}
	if !page.HasNext {
		t.Fatalf("page paging fields wrong: %+v", page)
	}
	if len(page.Submissions) != 2 {
		t.Fatalf("got %d submissions", len(page.Submissions))
	}
	first := page.Submissions[0]
	if first.RemoteID != "1234" || !first.Accepted() || first.Lang != "python3" || first.Timestamp != 1700000000 {
		t.Errorf("first submission wrong: %+v", first)
	}
	if first.IsPending {
		t.Errorf("expected IsPending=false for %q", "Not Pending")
	}
	if page.Submissions[1].Accepted() {
		t.Errorf("second submission should not be Accepted: %+v", page.Submissions[1])
	}
}

func TestSubmissionListRequiresAuth(t *testing.T) {
	srv := gqlServer(t, map[string]string{"submissionList": `{"data":{}}`})
	c := testClient(t, srv)

	_, err := c.SubmissionList(context.Background(), "two-sum", 20, 0)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Message, "not authenticated") {
		t.Fatalf("want not-authenticated APIError, got %v", err)
	}
}

func TestSubmissionListIsPendingTrue(t *testing.T) {
	const payload = `{"data":{"questionSubmissionList":{"lastKey":"","hasNext":false,"submissions":[
		{"id":"1","statusDisplay":"","lang":"go","langName":"Go","runtime":"","memory":"","timestamp":"1700000001","url":"","isPending":"Pending","notes":""}
	]}}}`
	srv := gqlServer(t, map[string]string{"submissionList": payload})
	c := testClient(t, srv, WithCredentials(Credentials{Session: "sess", CSRFToken: "tok"}))

	page, err := c.SubmissionList(context.Background(), "two-sum", 1, 0)
	if err != nil {
		t.Fatalf("SubmissionList: %v", err)
	}
	if len(page.Submissions) != 1 || !page.Submissions[0].IsPending {
		t.Fatalf("expected IsPending=true, got %+v", page.Submissions)
	}
}

// TestSubmissionListPagesByOffset locks in the live-verified paging
// mechanism: the server ignores/echoes an empty lastKey regardless of
// hasNext, so callers must page by bumping offset, not by round-tripping
// lastKey. This asserts the client actually forwards a nonzero offset.
func TestSubmissionListPagesByOffset(t *testing.T) {
	var gotOffset float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables struct {
				Offset float64 `json:"offset"`
			} `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotOffset = req.Variables.Offset
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"questionSubmissionList":{"lastKey":"","hasNext":false,"submissions":[]}}}`))
	}))
	t.Cleanup(srv.Close)
	c := testClient(t, srv, WithCredentials(Credentials{Session: "sess", CSRFToken: "tok"}))

	if _, err := c.SubmissionList(context.Background(), "two-sum", 3, 6); err != nil {
		t.Fatalf("SubmissionList: %v", err)
	}
	if gotOffset != 6 {
		t.Fatalf("offset not forwarded: got %v", gotOffset)
	}
}
