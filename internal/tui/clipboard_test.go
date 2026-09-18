package tui

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestOSC52CopyFormat(t *testing.T) {
	got := osc52Copy("hello world")
	const wantPrefix = "\x1b]52;c;"
	const wantSuffix = "\x07"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("osc52Copy() = %q, want prefix %q", got, wantPrefix)
	}
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("osc52Copy() = %q, want suffix %q (BEL terminator)", got, wantSuffix)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(got, wantPrefix), wantSuffix)
	dec, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("osc52Copy() payload isn't valid base64: %v", err)
	}
	if string(dec) != "hello world" {
		t.Fatalf("osc52Copy() round-trips to %q, want %q", dec, "hello world")
	}
}

func TestOSC52CopyRoundTripsMultiLine(t *testing.T) {
	text := "line one\nline two\nline three"
	got := osc52Copy(text)
	payload := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]52;c;"), "\x07")
	dec, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("invalid base64: %v", err)
	}
	if string(dec) != text {
		t.Fatalf("round-tripped %q, want %q", dec, text)
	}
}

func TestCopiedStatusMsgPluralization(t *testing.T) {
	if got := copiedStatusMsg("x"); got != "copied 1 character to clipboard" {
		t.Errorf("copiedStatusMsg(1 char) = %q", got)
	}
	if got := copiedStatusMsg("xyz"); got != "copied 3 characters to clipboard" {
		t.Errorf("copiedStatusMsg(3 chars) = %q", got)
	}
	if got := copiedStatusMsg(""); got != "copied 0 characters to clipboard" {
		t.Errorf("copiedStatusMsg(empty) = %q", got)
	}
}

func TestPlainViewportTextStripsANSIAndTrailingPadding(t *testing.T) {
	view := "\x1b[1mheading\x1b[0m   \nplain body line   \n\n"
	got := plainViewportText(view)
	want := "heading\nplain body line"
	if got != want {
		t.Fatalf("plainViewportText() =\n%q\nwant\n%q", got, want)
	}
}
