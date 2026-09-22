// Package selfupdate checks GitHub for newer lazyleet releases and upgrades
// the running binary in place, using whichever mechanism installed it
// (Homebrew, `go install`, or a release archive).
package selfupdate

import (
	"strconv"
	"strings"
)

// semver is a parsed MAJOR.MINOR.PATCH[-PRERELEASE] version. Build metadata
// (+...) is dropped; it never affects precedence.
type semver struct {
	major, minor, patch int
	pre                 string
}

func parseSemver(v string) (semver, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var s semver
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, s.pre = v[:i], v[i+1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	nums := [3]*int{&s.major, &s.minor, &s.patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		*nums[i] = n
	}
	return s, true
}

// IsSemver reports whether v parses as a release or prerelease version
// (with or without a leading "v").
func IsSemver(v string) bool {
	_, ok := parseSemver(v)
	return ok
}

// Newer reports whether latest is a strictly higher version than current.
// Either side failing to parse (e.g. current is "dev") reports false, so an
// unversioned build is never nagged.
func Newer(latest, current string) bool {
	l, ok := parseSemver(latest)
	if !ok {
		return false
	}
	c, ok := parseSemver(current)
	if !ok {
		return false
	}
	return compare(l, c) > 0
}

func compare(a, b semver) int {
	for _, d := range []int{a.major - b.major, a.minor - b.minor, a.patch - b.patch} {
		if d != 0 {
			return sign(d)
		}
	}
	// A prerelease sorts before its release; two prereleases compare
	// lexically, which is good enough for goreleaser-style tags.
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	default:
		return strings.Compare(a.pre, b.pre)
	}
}

func sign(n int) int {
	if n > 0 {
		return 1
	}
	return -1
}
