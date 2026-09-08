package browsercookies

import (
	"testing"
	"time"
)

func TestNeedsKeychain(t *testing.T) {
	for _, b := range []string{"chrome", "Chrome", " brave ", "edge", "arc", "vivaldi"} {
		if !NeedsKeychain(b) {
			t.Errorf("NeedsKeychain(%q) = false, want true", b)
		}
	}
	for _, b := range []string{"firefox", "safari", "librewolf", "", "lynx"} {
		if NeedsKeychain(b) {
			t.Errorf("NeedsKeychain(%q) = true, want false", b)
		}
	}
}

func TestPickPrefersCompleteFreshGroup(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-time.Hour)

	cookies := []Cookie{
		// chrome: session only (incomplete)
		{Name: "LEETCODE_SESSION", Value: "chrome-sess", Browser: "chrome", Expires: future},
		// firefox: both, but session expired
		{Name: "LEETCODE_SESSION", Value: "ff-sess", Browser: "firefox", Expires: past, Expired: true},
		{Name: "csrftoken", Value: "ff-csrf", Browser: "firefox", Expires: future},
		// arc: both, fresh -> should win
		{Name: "LEETCODE_SESSION", Value: "arc-sess", Browser: "arc", Expires: future},
		{Name: "csrftoken", Value: "arc-csrf", Browser: "arc", Expires: future},
	}

	vals, source, ok := Pick(cookies, "LEETCODE_SESSION", "csrftoken")
	if !ok {
		t.Fatal("Pick returned ok=false")
	}
	if source != "arc" {
		t.Errorf("source = %q, want arc", source)
	}
	if vals["LEETCODE_SESSION"] != "arc-sess" || vals["csrftoken"] != "arc-csrf" {
		t.Errorf("values = %v", vals)
	}
}

func TestPickFallsBackToExpiredCompleteGroup(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	cookies := []Cookie{
		{Name: "LEETCODE_SESSION", Value: "s", Browser: "safari", Expires: past, Expired: true},
		{Name: "csrftoken", Value: "c", Browser: "safari", Expires: past, Expired: true},
	}
	vals, source, ok := Pick(cookies, "LEETCODE_SESSION", "csrftoken")
	if !ok || source != "safari" || vals["csrftoken"] != "c" {
		t.Fatalf("expected the expired-but-complete safari group, got ok=%v source=%q vals=%v", ok, source, vals)
	}
}

func TestPickNoCompleteGroup(t *testing.T) {
	cookies := []Cookie{
		{Name: "LEETCODE_SESSION", Value: "s", Browser: "chrome"},
		{Name: "csrftoken", Value: "c", Browser: "firefox"},
	}
	if _, _, ok := Pick(cookies, "LEETCODE_SESSION", "csrftoken"); ok {
		t.Fatal("Pick should not combine cookies across browsers")
	}
}

func TestPickProfileInSource(t *testing.T) {
	cookies := []Cookie{
		{Name: "LEETCODE_SESSION", Value: "s", Browser: "chrome", Profile: "Work"},
		{Name: "csrftoken", Value: "c", Browser: "chrome", Profile: "Work"},
	}
	_, source, ok := Pick(cookies, "LEETCODE_SESSION", "csrftoken")
	if !ok || source != "chrome (Work)" {
		t.Fatalf("source = %q, want 'chrome (Work)'", source)
	}
}

func TestPickKeepsLatestExpiringDuplicate(t *testing.T) {
	early := time.Now().Add(time.Hour)
	late := time.Now().Add(48 * time.Hour)
	cookies := []Cookie{
		{Name: "LEETCODE_SESSION", Value: "old", Browser: "chrome", Expires: early},
		{Name: "LEETCODE_SESSION", Value: "new", Browser: "chrome", Expires: late},
		{Name: "csrftoken", Value: "c", Browser: "chrome", Expires: late},
	}
	vals, _, ok := Pick(cookies, "LEETCODE_SESSION", "csrftoken")
	if !ok || vals["LEETCODE_SESSION"] != "new" {
		t.Fatalf("expected the later-expiring session value, got %v", vals)
	}
}
