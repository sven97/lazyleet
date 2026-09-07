package leetcode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Credentials are the two browser cookies LeetCode uses for authenticated
// requests. Both are copied from a logged-in browser session. They are optional:
// the problem list, problem detail, and study plans all work anonymously.
type Credentials struct {
	Session   string `json:"session"`   // LEETCODE_SESSION cookie
	CSRFToken string `json:"csrftoken"` // csrftoken cookie
	Region    string `json:"region"`    // "com" | "cn"
	SavedAt   int64  `json:"saved_at"`  // unix seconds
}

// Anonymous reports whether these credentials are empty.
func (c Credentials) Anonymous() bool { return c.Session == "" }

// Validate checks that a non-empty credential set has both cookies.
func (c Credentials) Validate() error {
	if c.Session == "" && c.CSRFToken == "" {
		return nil // fully anonymous is fine
	}
	if c.Session == "" {
		return errors.New("missing LEETCODE_SESSION")
	}
	if c.CSRFToken == "" {
		return errors.New("missing csrftoken")
	}
	return nil
}

// cookieHeader renders the Cookie request header value.
func (c Credentials) cookieHeader() string {
	if c.Anonymous() {
		return ""
	}
	return fmt.Sprintf("LEETCODE_SESSION=%s; csrftoken=%s", c.Session, c.CSRFToken)
}

// LoadCredentials reads auth.json. A missing file yields anonymous credentials
// and no error.
func LoadCredentials(path string) (Credentials, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Credentials{}, nil
	}
	if err != nil {
		return Credentials{}, err
	}
	var c Credentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return Credentials{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, c.Validate()
}

// Save writes auth.json with 0600 permissions, creating parent dirs.
func (c Credentials) Save(path string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	c.SavedAt = time.Now().Unix()
	blob, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// Write via a temp file so a crashed write can't leave a half-file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(blob, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DeleteCredentials removes auth.json. A missing file is not an error.
func DeleteCredentials(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
