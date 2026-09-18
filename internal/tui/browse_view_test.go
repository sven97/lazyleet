package tui

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// wideStatusModel and narrowStatusModel are just past/below lazyleetArtWidth,
// the ASCII-art fallback threshold statusHeaderBlock uses.
func wideStatusWidth() int   { return lazyleetArtWidth + 10 }
func narrowStatusWidth() int { return lazyleetArtWidth - 10 }

func TestStatusDetailBodyOrdersHeaderLinksThenLiveSections(t *testing.T) {
	m := &BrowseModel{
		th:      DefaultTheme(),
		version: "v1.2.3",
		auth:    AuthState{Authed: true, User: "sven97"},
		allRows: []BrowseRow{{FrontendID: 1, Title: "Two Sum", Status: "ac"}},
		daily: DailyInfo{
			Date: time.Now().Format("2006-01-02"), Slug: "two-sum",
			FrontendID: 1, Title: "Two Sum", Difficulty: "Easy",
		},
		dailyLoaded: true,
	}
	got := m.statusDetailBody(wideStatusWidth())

	for _, want := range []string{
		"lazyleet", "v1.2.3",
		"https://github.com/sven97/lazyleet",
		"https://github.com/sven97/lazyleet/issues",
		"https://github.com/sven97/lazyleet/releases",
		"Account", "Catalog", "Progress", "Daily",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	// Structural order: header/version/links block, then the four live
	// sections in their documented order. Search for the art's first glyph
	// row rather than the literal word "lazyleet" — that substring also
	// occurs inside the github.com/sven97/lazyleet URLs further down.
	idx := func(s string) int { return strings.Index(got, s) }
	artFirstLine := strings.SplitN(lazyleetArt, "\n", 2)[0]
	order := []string{artFirstLine, "v1.2.3", "github.com/sven97/lazyleet/releases", "Account", "Catalog", "Progress", "Daily"}
	for i := 1; i < len(order); i++ {
		if idx(order[i-1]) >= idx(order[i]) {
			t.Errorf("expected %q before %q, got positions %d, %d:\n%s", order[i-1], order[i], idx(order[i-1]), idx(order[i]), got)
		}
	}
}

func TestStatusDetailBodyShowsAsciiArtWhenWideFallsBackWhenNarrow(t *testing.T) {
	m := &BrowseModel{th: DefaultTheme(), allRows: nil, dailyErr: errors.New("boom")}

	wide := m.statusDetailBody(wideStatusWidth())
	if !strings.Contains(wide, lazyleetArt) {
		t.Errorf("wide pane should show the full ASCII-art wordmark:\n%s", wide)
	}

	narrow := m.statusDetailBody(narrowStatusWidth())
	if strings.Contains(narrow, lazyleetArt) {
		t.Errorf("narrow pane should fall back, not show the full ASCII-art wordmark:\n%s", narrow)
	}
	if !strings.Contains(narrow, "lazyleet") {
		t.Errorf("narrow pane should still show a plain \"lazyleet\" wordmark line:\n%s", narrow)
	}
}

func TestStatusDetailBodyAnonymousShowsSignInHint(t *testing.T) {
	m := &BrowseModel{
		th:      DefaultTheme(),
		auth:    AuthState{Authed: false},
		allRows: []BrowseRow{{FrontendID: 1, Title: "Two Sum"}},
	}
	got := m.statusDetailBody(wideStatusWidth())
	if !strings.Contains(got, "anonymous") {
		t.Errorf("anonymous user should see the anonymous hint:\n%s", got)
	}
	if !strings.Contains(got, "sign in to track") {
		t.Errorf("anonymous user's Progress section should prompt to sign in:\n%s", got)
	}
	if strings.Contains(got, "solved") && strings.Contains(got, " / ") {
		t.Errorf("anonymous user shouldn't see a solved-count fraction:\n%s", got)
	}
}

func TestStatusDetailBodyStaleSyncStillRendersAge(t *testing.T) {
	m := &BrowseModel{
		th:       DefaultTheme(),
		auth:     AuthState{Authed: true, User: "sven97"},
		allRows:  []BrowseRow{{FrontendID: 1, Title: "Two Sum", Status: "ac"}},
		lastSync: time.Now().Add(-72 * time.Hour),
	}
	got := m.statusDetailBody(wideStatusWidth())
	if !strings.Contains(got, "3d ago") {
		t.Errorf("a 72h-old sync should render as a rough age (\"3d ago\"):\n%s", got)
	}
}

func TestStatusDetailBodyDailyLoadingAndErrorStates(t *testing.T) {
	loading := &BrowseModel{th: DefaultTheme()}
	if got := loading.statusDetailBody(wideStatusWidth()); !strings.Contains(got, "loading…") {
		t.Errorf("unloaded daily should show a loading placeholder:\n%s", got)
	}

	failed := &BrowseModel{th: DefaultTheme(), dailyErr: errors.New("network down")}
	if got := failed.statusDetailBody(wideStatusWidth()); !strings.Contains(got, "unavailable") || !strings.Contains(got, "network down") {
		t.Errorf("a daily load error should render as unavailable + the error text:\n%s", got)
	}
}

func TestStatusDetailBodyOmitsVersionLineWhenUnset(t *testing.T) {
	m := &BrowseModel{th: DefaultTheme()}
	got := m.statusDetailBody(wideStatusWidth())
	// The header/wordmark still renders; just no dedicated version line
	// (nothing to plumb through in tests that don't call SetVersion).
	if !strings.Contains(got, "lazyleet") {
		t.Errorf("header should still render without a version set:\n%s", got)
	}
}

func TestAsciiArtWidthMatchesWidestLine(t *testing.T) {
	art := "ab\nabcd\nabc"
	if got := asciiArtWidth(art); got != 4 {
		t.Errorf("asciiArtWidth(%q) = %d, want 4", art, got)
	}
}
