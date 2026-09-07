package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a tiny helper shared by the config tests.
func writeFile(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func TestResolvePathsHonoursXDG(t *testing.T) {
	cfgHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	p := ResolvePaths()

	if want := filepath.Join(cfgHome, "lazyleet", "config.yml"); p.ConfigFile != want {
		t.Errorf("ConfigFile = %q, want %q", p.ConfigFile, want)
	}
	if want := filepath.Join(dataHome, "lazyleet", "lazyleet.db"); p.DatabaseFile != want {
		t.Errorf("DatabaseFile = %q, want %q", p.DatabaseFile, want)
	}
	if want := filepath.Join(dataHome, "lazyleet", "workspace"); p.WorkspaceRoot != want {
		t.Errorf("WorkspaceRoot = %q, want %q", p.WorkspaceRoot, want)
	}
}

func TestResolvePathsFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	p := ResolvePaths()

	if want := filepath.Join(home, ".config", "lazyleet"); p.ConfigDir != want {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, want)
	}
	if want := filepath.Join(home, ".local", "share", "lazyleet"); p.DataDir != want {
		t.Errorf("DataDir = %q, want %q", p.DataDir, want)
	}
}

func TestEnsureDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	p := ResolvePaths()
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	for _, d := range []string{p.ConfigDir, p.DataDir, p.WorkspaceRoot} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Errorf("expected dir %s: err=%v", d, err)
		}
	}
}
