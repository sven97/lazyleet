package leetcode

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCredentialsRoundTripAndPerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	in := Credentials{Session: "sess", CSRFToken: "tok", Region: "com"}
	if err := in.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Session != "sess" || got.CSRFToken != "tok" || got.Region != "com" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if got.SavedAt == 0 {
		t.Error("SavedAt not stamped")
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("auth.json perm = %o, want 600", perm)
		}
	}
}

func TestLoadMissingIsAnonymous(t *testing.T) {
	got, err := LoadCredentials(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Anonymous() {
		t.Fatalf("expected anonymous, got %+v", got)
	}
}

func TestCredentialsValidate(t *testing.T) {
	if err := (Credentials{}).Validate(); err != nil {
		t.Errorf("empty creds should validate (anonymous): %v", err)
	}
	if err := (Credentials{Session: "x"}).Validate(); err == nil {
		t.Error("session without csrftoken should fail")
	}
	if err := (Credentials{CSRFToken: "x"}).Validate(); err == nil {
		t.Error("csrftoken without session should fail")
	}
}

func TestDeleteCredentialsMissingOK(t *testing.T) {
	if err := DeleteCredentials(filepath.Join(t.TempDir(), "nope.json")); err != nil {
		t.Fatalf("DeleteCredentials on missing file: %v", err)
	}
}
