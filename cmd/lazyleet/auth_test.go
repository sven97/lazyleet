package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sven97/lazyleet/internal/config"
	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/store"
)

func authTestApp(t *testing.T) *appContext {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	return &appContext{paths: config.ResolvePaths(), cfg: config.Default()}
}

func TestRefreshAuthProgress(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		seeded, expired, fail bool
	}{
		{name: "first sign-in"}, {name: "cached catalog", seeded: true},
		{name: "expired session preserves progress", seeded: true, expired: true},
		{name: "failed refresh preserves progress", seeded: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := authTestApp(t)
			ctx := context.Background()
			db, err := app.openStore()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if tc.seeded {
				if err := db.UpsertProblems(ctx, []store.Problem{{FrontendID: 1, Slug: "two-sum", Title: "Two Sum", Status: "ac"}}); err != nil {
					t.Fatal(err)
				}
			}
			catalogCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Operation string `json:"operationName"`
					Variables struct {
						Filters struct {
							Status string `json:"status"`
						} `json:"filters"`
					} `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				if req.Operation == "globalData" {
					fmt.Fprintf(w, `{"data":{"userStatus":{"isSignedIn":%t,"username":"tester"}}}`, !tc.expired)
					return
				}
				status := req.Variables.Filters.Status
				if status == "" {
					catalogCalls++
				}
				if tc.fail && status == "TRIED" {
					fmt.Fprint(w, `{"errors":[{"message":"progress unavailable"}]}`)
					return
				}
				if status == "TRIED" {
					fmt.Fprint(w, `{"data":{"problemsetQuestionList":{"total":0,"questions":[]}}}`)
					return
				}
				fmt.Fprint(w, `{"data":{"problemsetQuestionList":{"total":1,"questions":[{"frontendId":"1","slug":"two-sum","title":"Two Sum","difficulty":"Easy"}]}}}`)
			}))
			defer server.Close()
			client := leetcode.New("com", leetcode.WithBaseURL(server.URL), leetcode.WithRateLimit(10000, 100), leetcode.WithCredentials(leetcode.Credentials{Session: "test", CSRFToken: "test"}))
			solved, err := refreshAuthProgress(ctx, app, client)
			if tc.expired || tc.fail {
				if err == nil {
					t.Fatal("expected refresh failure")
				}
			} else if err != nil || solved != 1 {
				t.Fatalf("solved=%d err=%v", solved, err)
			}
			rows, err := db.ListProblems(ctx, store.ProblemFilter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Status != "ac" {
				t.Fatalf("lost progress: %+v", rows)
			}
			_, synced, err := db.GetMetaTime(ctx, store.MetaProgressSyncedAt)
			if err != nil {
				t.Fatal(err)
			}
			if synced == (tc.expired || tc.fail) {
				t.Fatalf("unexpected sync timestamp: %v", synced)
			}
			if tc.seeded && catalogCalls != 0 {
				t.Fatalf("unnecessary catalog fetch: %d", catalogCalls)
			}
			if !tc.seeded && catalogCalls != 1 {
				t.Fatalf("catalog was not seeded: %d", catalogCalls)
			}
		})
	}
}

func TestOpenWorkspaceReloadsCredentials(t *testing.T) {
	app := authTestApp(t)
	q := leetcode.Question{Slug: "two-sum", QuestionID: 1}
	judge := newRemoteJudge(app, q, "python3")
	if judge.Available() {
		t.Fatal("anonymous judge available")
	}
	if err := (leetcode.Credentials{Session: "new-session", CSRFToken: "csrf"}).Save(app.paths.AuthFile); err != nil {
		t.Fatal(err)
	}
	if !judge.Available() {
		t.Fatal("open workspace did not pick up sign-in")
	}
	if err := os.Remove(app.paths.AuthFile); err != nil {
		t.Fatal(err)
	}
	if judge.Available() {
		t.Fatal("open workspace did not pick up logout")
	}
	_, err := judge.Run(context.Background(), "", "")
	if err == nil || !strings.Contains(err.Error(), "lazyleet auth") {
		t.Fatalf("missing sign-in guidance: %v", err)
	}
}
