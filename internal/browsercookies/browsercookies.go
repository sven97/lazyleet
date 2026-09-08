// Package browsercookies reads cookies for a given site straight out of the
// local browsers' cookie stores (Chrome, Firefox, Safari, Edge, Brave, Arc,
// Vivaldi, Opera, …) via the kooky library, so lazyleet can pick up a LeetCode
// session without the user copying cookies out of DevTools.
package browsercookies

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/all" // register every browser finder
)

// Cookie is a single cookie found in a browser store.
type Cookie struct {
	Name    string
	Value   string
	Browser string // "chrome", "firefox", "safari", …
	Profile string
	Expires time.Time
	Expired bool
}

// Read collects cookies whose domain ends with hostSuffix from every readable
// browser cookie store. onlyBrowser (case-insensitive), if set, limits the
// search to that browser. names, if given, restricts to those cookie names.
//
// Per-store errors (locked file, denied Keychain, unsupported format) are
// skipped so one unreadable browser does not fail the whole search.
func Read(ctx context.Context, hostSuffix, onlyBrowser string, names ...string) []Cookie {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}

	var out []Cookie
	for c, err := range kooky.TraverseCookies(ctx, kooky.DomainHasSuffix(hostSuffix)) {
		if err != nil || c == nil {
			continue
		}
		if len(want) > 0 && !want[c.Name] {
			continue
		}
		br, prof := "", ""
		if c.Browser != nil {
			br, prof = c.Browser.Browser(), c.Browser.Profile()
		}
		if onlyBrowser != "" && !strings.EqualFold(br, onlyBrowser) {
			continue
		}
		out = append(out, Cookie{
			Name:    c.Name,
			Value:   c.Value,
			Browser: br,
			Profile: prof,
			Expires: c.Expires,
			Expired: !c.Expires.IsZero() && c.Expires.Before(time.Now()),
		})
	}
	return out
}

// Stores lists the browser cookie stores kooky can see on this machine,
// formatted as "browser" or "browser (profile)".
func Stores(ctx context.Context) []string {
	seen := map[string]bool{}
	var out []string
	for s, err := range kooky.TraverseCookieStores(ctx) {
		if err != nil || s == nil {
			continue
		}
		label := s.Browser()
		if label == "" {
			continue
		}
		if p := s.Profile(); p != "" && !strings.EqualFold(p, "default") {
			label += " (" + p + ")"
		}
		if !seen[label] {
			seen[label] = true
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// Pick chooses one browser's values for every wanted cookie name. It groups the
// cookies by browser+profile and returns the first group (preferring one whose
// cookies are all unexpired, then the one with the newest expiry) that has a
// non-empty value for every name in want. source is "browser" or
// "browser (profile)".
func Pick(cookies []Cookie, want ...string) (values map[string]string, source string, ok bool) {
	type group struct {
		browser string
		vals    map[string]Cookie
	}
	groups := map[string]*group{}
	var order []string
	for _, c := range cookies {
		key := c.Browser + "\x00" + c.Profile
		g := groups[key]
		if g == nil {
			label := c.Browser
			if c.Profile != "" && !strings.EqualFold(c.Profile, "default") {
				label += " (" + c.Profile + ")"
			}
			g = &group{browser: label, vals: map[string]Cookie{}}
			groups[key] = g
			order = append(order, key)
		}
		// keep the latest-expiring value for a repeated name
		if prev, dup := g.vals[c.Name]; !dup || c.Expires.After(prev.Expires) {
			g.vals[c.Name] = c
		}
	}

	complete := func(g *group) (map[string]string, bool) {
		vals := make(map[string]string, len(want))
		for _, n := range want {
			c, has := g.vals[n]
			if !has || c.Value == "" {
				return nil, false
			}
			vals[n] = c.Value
		}
		return vals, true
	}
	allFresh := func(g *group) bool {
		for _, n := range want {
			if g.vals[n].Expired {
				return false
			}
		}
		return true
	}

	// pass 1: a group with every wanted cookie present and unexpired
	for _, key := range order {
		g := groups[key]
		if v, done := complete(g); done && allFresh(g) {
			return v, g.browser, true
		}
	}
	// pass 2: any group with every wanted cookie present
	for _, key := range order {
		g := groups[key]
		if v, done := complete(g); done {
			return v, g.browser, true
		}
	}
	return nil, "", false
}
