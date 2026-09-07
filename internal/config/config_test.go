package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DefaultLanguage != Default().DefaultLanguage || got.CacheTTL != Default().CacheTTL {
		t.Fatalf("expected defaults, got %+v", got)
	}
}

func TestLoadOverridesAndFallbacks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	body := "" +
		"default_language: cpp\n" +
		"cache_ttl: 90m\n" +
		"region: cn\n" +
		"workspace:\n" +
		"  run_on_save: false\n"
	if err := writeFile(path, body); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DefaultLanguage != "cpp" {
		t.Errorf("DefaultLanguage = %q, want cpp", got.DefaultLanguage)
	}
	if got.CacheTTL.D() != 90*time.Minute {
		t.Errorf("CacheTTL = %s, want 1h30m0s", got.CacheTTL)
	}
	if got.Region != RegionCN {
		t.Errorf("Region = %q, want cn", got.Region)
	}
	if got.Workspace.RunOnSave {
		t.Errorf("Workspace.RunOnSave = true, want false")
	}
	// Unset fields fall back to defaults.
	if got.Theme != Default().Theme {
		t.Errorf("Theme = %q, want default fallback %q", got.Theme, Default().Theme)
	}
	if got.Workspace.Tier != TierAuto {
		t.Errorf("Workspace.Tier = %q, want auto fallback", got.Workspace.Tier)
	}
	if got.Workspace.RunDebounceMs != Default().Workspace.RunDebounceMs {
		t.Errorf("RunDebounceMs = %d, want default fallback", got.Workspace.RunDebounceMs)
	}
}

func TestLoadRejectsBadRegion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := writeFile(path, "region: mars\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown region")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yml")
	in := Default()
	in.DefaultLanguage = "javascript"
	in.CacheTTL = Duration(6 * time.Hour)
	if err := in.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.DefaultLanguage != "javascript" || out.CacheTTL.D() != 6*time.Hour {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestResolveEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if got := Default().ResolveEditor(); got != "vi" {
		t.Errorf("ResolveEditor with no env = %q, want vi", got)
	}

	t.Setenv("EDITOR", "nano")
	if got := Default().ResolveEditor(); got != "nano" {
		t.Errorf("ResolveEditor with $EDITOR = %q, want nano", got)
	}

	t.Setenv("VISUAL", "hx")
	if got := Default().ResolveEditor(); got != "hx" {
		t.Errorf("ResolveEditor with $VISUAL = %q, want hx", got)
	}

	c := Default()
	c.Editor = "code -w"
	if got := c.ResolveEditor(); got != "code -w" {
		t.Errorf("ResolveEditor with explicit = %q, want 'code -w'", got)
	}
}
