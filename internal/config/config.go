package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// defaultCacheTTL is how long cached problem data is considered fresh.
const defaultCacheTTL = Duration(24 * time.Hour)

// Region selects which LeetCode deployment to talk to.
type Region string

const (
	RegionCom Region = "com" // leetcode.com
	RegionCN  Region = "cn"  // leetcode.cn (Phase 7)
)

// WorkspaceTier chooses how the coding workspace hosts the editor (PLAN.md §5).
type WorkspaceTier string

const (
	TierAuto        WorkspaceTier = "auto"        // pick the best available at runtime
	TierMirror      WorkspaceTier = "mirror"      // Tier C: read-only mirror + file watch
	TierMultiplexer WorkspaceTier = "multiplexer" // Tier B: spawn $EDITOR in an adjacent pane
	TierEmbedded    WorkspaceTier = "embedded"    // Tier A: PTY editor pane inside lazyleet
)

// Config is the user-editable configuration, loaded from config.yml.
type Config struct {
	Region          Region            `yaml:"region"`
	DefaultLanguage string            `yaml:"default_language"`
	Editor          string            `yaml:"editor"` // empty -> $VISUAL, $EDITOR, then vi
	Theme           string            `yaml:"theme"`
	Images          string            `yaml:"images"` // auto | off | blocks | kitty
	CacheTTL        Duration          `yaml:"cache_ttl"`
	Keys            map[string]string `yaml:"keys"` // semantic action -> key override
	Workspace       WorkspaceConfig   `yaml:"workspace"`
	// UpdateCheck looks for a newer release on launch (at most once a day)
	// and offers it in the Status pane.
	UpdateCheck bool `yaml:"update_check"`
}

// WorkspaceConfig tunes coding-workspace behaviour.
type WorkspaceConfig struct {
	Tier          WorkspaceTier `yaml:"tier"`
	RunOnSave     bool          `yaml:"run_on_save"`
	RunDebounceMs int           `yaml:"run_debounce_ms"`
}

// Default returns the built-in configuration used when config.yml is absent or
// a field is left unset.
func Default() Config {
	return Config{
		Region:          RegionCom,
		DefaultLanguage: "python3",
		Editor:          "",
		Theme:           "default",
		Images:          "auto",
		CacheTTL:        defaultCacheTTL,
		Keys:            map[string]string{},
		UpdateCheck:     true,
		Workspace: WorkspaceConfig{
			Tier:          TierAuto,
			RunOnSave:     true,
			RunDebounceMs: 400,
		},
	}
}

// Load reads config.yml at the given path, filling any unset field from
// Default. A missing file is not an error: it returns Default with no error.
func Load(path string) (Config, error) {
	cfg := Default()

	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	// Decode over the defaults so omitted keys keep their default value.
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Default(), fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.applyFallbacks()
	if err := cfg.Validate(); err != nil {
		return Default(), err
	}
	return cfg, nil
}

// Save writes the configuration to path as YAML, creating parent dirs.
func (c Config) Save(path string) error {
	if err := os.MkdirAll(dir(path), 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// applyFallbacks repairs zero values that YAML decoding may have introduced for
// keys present-but-empty in the file.
func (c *Config) applyFallbacks() {
	d := Default()
	if c.Region == "" {
		c.Region = d.Region
	}
	if c.DefaultLanguage == "" {
		c.DefaultLanguage = d.DefaultLanguage
	}
	if c.Theme == "" {
		c.Theme = d.Theme
	}
	if c.Images == "" {
		c.Images = d.Images
	}
	if c.CacheTTL == 0 {
		c.CacheTTL = d.CacheTTL
	}
	if c.Keys == nil {
		c.Keys = map[string]string{}
	}
	if c.Workspace.Tier == "" {
		c.Workspace.Tier = d.Workspace.Tier
	}
	if c.Workspace.RunDebounceMs == 0 {
		c.Workspace.RunDebounceMs = d.Workspace.RunDebounceMs
	}
}

// Validate rejects configurations that cannot work.
func (c Config) Validate() error {
	switch c.Region {
	case RegionCom, RegionCN:
	default:
		return fmt.Errorf("config: unknown region %q (want %q or %q)", c.Region, RegionCom, RegionCN)
	}
	switch c.Workspace.Tier {
	case TierAuto, TierMirror, TierMultiplexer, TierEmbedded:
	default:
		return fmt.Errorf("config: unknown workspace.tier %q", c.Workspace.Tier)
	}
	if c.Workspace.RunDebounceMs < 0 {
		return fmt.Errorf("config: workspace.run_debounce_ms must be >= 0")
	}
	return nil
}

// ResolveEditor returns the editor command to launch: the configured value, or
// $VISUAL, or $EDITOR, or "vi".
func (c Config) ResolveEditor() string {
	if c.Editor != "" {
		return c.Editor
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	return "vi"
}

func dir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
