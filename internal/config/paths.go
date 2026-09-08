// Package config resolves lazyleet's on-disk locations and loads/saves the
// user configuration file.
//
// Directory layout (XDG-style, honoured on every platform per PLAN.md D7):
//
//	$XDG_CONFIG_HOME/lazyleet/config.yml   (default ~/.config/lazyleet)
//	$XDG_DATA_HOME/lazyleet/lazyleet.db    (default ~/.local/share/lazyleet)
//	$XDG_DATA_HOME/lazyleet/auth.json      (0600)
//	$XDG_DATA_HOME/lazyleet/workspace/<frontendID>-<slug>/
package config

import (
	"os"
	"path/filepath"
)

// Paths holds every resolved filesystem location lazyleet uses.
type Paths struct {
	ConfigDir     string // directory containing config.yml
	DataDir       string // directory containing the DB, auth file, workspaces
	ConfigFile    string // <ConfigDir>/config.yml
	DatabaseFile  string // <DataDir>/lazyleet.db
	AuthFile      string // <DataDir>/auth.json
	WorkspaceRoot string // <DataDir>/workspace
	ImageCacheDir string // <DataDir>/imgcache
}

// ResolvePaths computes every path from the environment. It does not touch the
// filesystem; call EnsureDirs to create the directories.
func ResolvePaths() Paths {
	cfg := xdgDir("XDG_CONFIG_HOME", ".config")
	data := xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share"))

	configDir := filepath.Join(cfg, "lazyleet")
	dataDir := filepath.Join(data, "lazyleet")

	return Paths{
		ConfigDir:     configDir,
		DataDir:       dataDir,
		ConfigFile:    filepath.Join(configDir, "config.yml"),
		DatabaseFile:  filepath.Join(dataDir, "lazyleet.db"),
		AuthFile:      filepath.Join(dataDir, "auth.json"),
		WorkspaceRoot: filepath.Join(dataDir, "workspace"),
		ImageCacheDir: filepath.Join(dataDir, "imgcache"),
	}
}

// EnsureDirs creates the config, data, and workspace directories if missing.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.ConfigDir, p.DataDir, p.WorkspaceRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// xdgDir returns $envVar if set to an absolute path, otherwise
// <home>/<fallback>. If the home directory cannot be determined it falls back
// to the current working directory so lazyleet still runs in odd environments.
func xdgDir(envVar, fallback string) string {
	if v := os.Getenv(envVar); filepath.IsAbs(v) {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, fallback)
}
