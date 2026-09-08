package browserlogin

import (
	"context"
	"testing"
	"time"
)

func TestHaveAll(t *testing.T) {
	want := []string{"LEETCODE_SESSION", "csrftoken"}
	if HaveAll(map[string]string{"LEETCODE_SESSION": "x"}, want) {
		t.Error("HaveAll should be false with a missing cookie")
	}
	if HaveAll(map[string]string{"LEETCODE_SESSION": "x", "csrftoken": ""}, want) {
		t.Error("HaveAll should be false with an empty value")
	}
	if !HaveAll(map[string]string{"LEETCODE_SESSION": "x", "csrftoken": "y"}, want) {
		t.Error("HaveAll should be true when every cookie has a value")
	}
	if HaveAll(map[string]string{}, nil) {
		t.Error("HaveAll with no wanted names should be false")
	}
}

func TestCaptureRejectsBadOptions(t *testing.T) {
	_, err := Capture(context.Background(), Options{Timeout: time.Second})
	if err == nil {
		t.Fatal("expected an error when CookieDomain / WaitFor are missing")
	}
}
