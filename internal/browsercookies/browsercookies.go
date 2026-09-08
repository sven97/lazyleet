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

// chromiumFamily browsers store cookies encrypted with a key held in the OS
// keychain/secret service, so reading them triggers a permission prompt on
// macOS. lazyleet only touches these when the user asks for one explicitly or
// when no keychain-free browser has the cookies.
var chromiumFamily = map[string]bool{
	"chrome": true, "chromium": true, "brave": true, "edge": true,
	"vivaldi": true, "opera": true, "opera-gx": true, "arc": true,
	"yandex": true, "epic": true, "chrome-beta": true, "chrome-canary": true,
}

// NeedsKeychain reports whether importing from this browser prompts for the OS
// keychain / secret service.
func NeedsKeychain(browser string) bool {
	return chromiumFamily[strings.ToLower(strings.TrimSpace(browser))]
}

// Read collects cookies whose domain ends with hostSuffix from browser cookie
// stores.
//
//   - onlyBrowser (case-insensitive), if set, limits the search to that browser
//     and always reads it (even if it needs the keychain).
//   - When onlyBrowser is empty and allowKeychain is false, Chromium-family
//     stores are skipped *before* they are opened, so no keychain prompt fires.
//   - names, if given, restricts the result to those cookie names.
//
// Per-store errors (locked file, denied keychain, unsupported format) are
// skipped so one unreadable browser does not fail the whole search.
func Read(ctx context.Context, hostSuffix, onlyBrowser string, allowKeychain bool, names ...string) []Cookie {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}

	var out []Cookie
	for store, err := range kooky.TraverseCookieStores(ctx) {
		if err != nil || store == nil {
			continue
		}
		br := safeBrowser(store)
		switch {
		case onlyBrowser != "":
			if !strings.EqualFold(br, onlyBrowser) {
				store.Close()
				continue
			}
		case !allowKeychain && NeedsKeychain(br):
			store.Close()
			continue
		}

		prof := safeProfile(store)
		for c, cerr := range store.TraverseCookies(kooky.DomainHasSuffix(hostSuffix)) {
			if cerr != nil || c == nil {
				continue
			}
			if len(want) > 0 && !want[c.Name] {
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
		store.Close()
	}
	return out
}

func safeBrowser(s kooky.CookieStore) (name string) {
	defer func() { _ = recover() }()
	return s.Browser()
}

func safeProfile(s kooky.CookieStore) (p string) {
	defer func() { _ = recover() }()
	return s.Profile()
}

// Stores lists the browser cookie stores kooky can see on this machine,
// formatted as "browser" or "browser (profile)", with " · needs keychain"
// appended for Chromium-family browsers. Listing does not decrypt anything.
func Stores(ctx context.Context) []string {
	seen := map[string]bool{}
	var out []string
	for s, err := range kooky.TraverseCookieStores(ctx) {
		if err != nil || s == nil {
			continue
		}
		br := safeBrowser(s)
		if br == "" {
			s.Close()
			continue
		}
		label := br
		if p := safeProfile(s); p != "" && !strings.EqualFold(p, "default") {
			label += " (" + p + ")"
		}
		if NeedsKeychain(br) {
			label += "  · needs keychain"
		}
		if !seen[label] {
			seen[label] = true
			out = append(out, label)
		}
		s.Close()
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
