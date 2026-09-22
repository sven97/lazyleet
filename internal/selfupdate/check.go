package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// Repo is the GitHub owner/name releases are published under.
	Repo = "sven97/lazyleet"

	defaultAPIBase = "https://api.github.com"

	// CheckInterval is how long a successful check is reused before GitHub is
	// asked again — once a day, like gh and most CLIs, which also keeps well
	// inside the unauthenticated API rate limit.
	CheckInterval = 24 * time.Hour
)

// Release is the subset of a GitHub release lazyleet needs.
type Release struct {
	Tag    string  `json:"tag_name"` // e.g. "v0.4.0"
	URL    string  `json:"html_url"`
	Assets []Asset `json:"assets"`
}

// Asset is one downloadable file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Checker looks up the latest published release, caching the answer on disk
// so opening the app repeatedly doesn't hit GitHub every time.
type Checker struct {
	HTTP      *http.Client
	APIBase   string // default https://api.github.com (tests override)
	CacheFile string // empty disables caching
	Now       func() time.Time
}

// NewChecker returns a Checker that caches to cacheFile.
func NewChecker(cacheFile string) *Checker {
	return &Checker{
		HTTP:      &http.Client{Timeout: 10 * time.Second},
		APIBase:   defaultAPIBase,
		CacheFile: cacheFile,
		Now:       time.Now,
	}
}

type checkCache struct {
	CheckedAt time.Time `json:"checked_at"`
	Release   Release   `json:"release"`
}

// Latest returns the newest non-draft, non-prerelease release. A cached
// answer younger than CheckInterval is returned without a network call unless
// force is set.
func (c *Checker) Latest(ctx context.Context, force bool) (Release, error) {
	if !force {
		if cached, ok := c.readCache(); ok && c.Now().Sub(cached.CheckedAt) < CheckInterval {
			return cached.Release, nil
		}
	}
	rel, err := c.fetchLatest(ctx)
	if err != nil {
		return Release{}, err
	}
	c.writeCache(checkCache{CheckedAt: c.Now(), Release: rel})
	return rel, nil
}

func (c *Checker) fetchLatest(ctx context.Context) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.APIBase, Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "lazyleet-update-check")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("check for update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("check for update: GitHub returned %s", resp.Status)
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("check for update: decode release: %w", err)
	}
	if !IsSemver(rel.Tag) {
		return Release{}, fmt.Errorf("check for update: unexpected release tag %q", rel.Tag)
	}
	return rel, nil
}

func (c *Checker) readCache() (checkCache, bool) {
	if c.CacheFile == "" {
		return checkCache{}, false
	}
	raw, err := os.ReadFile(c.CacheFile)
	if err != nil {
		return checkCache{}, false
	}
	var cc checkCache
	if json.Unmarshal(raw, &cc) != nil || !IsSemver(cc.Release.Tag) {
		return checkCache{}, false
	}
	return cc, true
}

// writeCache is best effort: a failed write only means the next launch
// checks again.
func (c *Checker) writeCache(cc checkCache) {
	if c.CacheFile == "" {
		return
	}
	raw, err := json.Marshal(cc)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.CacheFile), 0o755); err != nil {
		return
	}
	tmp := c.CacheFile + ".tmp"
	if os.WriteFile(tmp, raw, 0o644) == nil {
		if err := os.Rename(tmp, c.CacheFile); err != nil {
			_ = os.Remove(tmp)
		}
	}
}

// ErrNoAsset means the release has no archive for this OS/architecture.
var ErrNoAsset = errors.New("release has no archive for this platform")
