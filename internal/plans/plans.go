// Package plans provides study-plan definitions that are not official LeetCode
// study plans, as bundled ordered slug lists compiled into the binary.
//
// Official plans (leetcode-75, top-interview-150, …) are fetched live via
// internal/leetcode instead. Blind 75 and NeetCode 150 slug lists still need to
// be authored and verified here — see PLAN.md.
package plans

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed data/*.json
var dataFS embed.FS

// Bundled is one compiled-in study plan.
type Bundled struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Problems    []string `json:"problems"` // ordered titleSlugs
}

var loaded = mustLoad()

func mustLoad() map[string]Bundled {
	out := map[string]Bundled{}
	entries, err := dataFS.ReadDir("data")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(dataFS, "data/"+e.Name())
		if err != nil {
			panic(err)
		}
		var b Bundled
		if err := json.Unmarshal(raw, &b); err != nil {
			panic(fmt.Errorf("plans: %s: %w", e.Name(), err))
		}
		if b.Slug == "" || len(b.Problems) == 0 {
			panic(fmt.Errorf("plans: %s: missing slug or problems", e.Name()))
		}
		out[b.Slug] = b
	}
	return out
}

// Get returns a bundled plan by slug.
func Get(slug string) (Bundled, bool) {
	b, ok := loaded[slug]
	return b, ok
}

// Slugs lists the bundled plan slugs, sorted.
func Slugs() []string {
	out := make([]string, 0, len(loaded))
	for s := range loaded {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// All returns every bundled plan, sorted by slug.
func All() []Bundled {
	out := make([]Bundled, 0, len(loaded))
	for _, s := range Slugs() {
		out = append(out, loaded[s])
	}
	return out
}
