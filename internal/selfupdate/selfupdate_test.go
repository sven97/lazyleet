package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.4.0", "v0.3.1", true},
		{"v0.3.1", "0.3.1", false},
		{"v0.3.10", "v0.3.9", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.3.0", "v0.3.1", false},
		{"v0.4.0", "v0.4.0-rc.1", true},
		{"v0.4.0-rc.1", "v0.4.0", false},
		{"v0.3.2", "v0.3.2-0.20260919044000-419ceef+dirty", true},
		{"v0.4.0", "dev", false},
		{"v0.4.0", "", false},
		{"garbage", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestDetect(t *testing.T) {
	proxyBuild := &debug.BuildInfo{Main: debug.Module{Version: "v0.3.1"}}
	checkoutBuild := &debug.BuildInfo{
		Main:     debug.Module{Version: "v0.3.2-0.20260919044000-419ceef+dirty"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "419ceef0000"}},
	}
	develBuild := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}

	cases := []struct {
		name      string
		exe       string
		info      *debug.BuildInfo
		ldVersion string
		want      Method
		version   string
	}{
		{"homebrew macOS", "/opt/homebrew/Cellar/lazyleet/0.3.1/bin/lazyleet", nil, "0.3.1", MethodHomebrew, "v0.3.1"},
		{"homebrew linux", "/home/linuxbrew/.linuxbrew/Cellar/lazyleet/0.3.1/bin/lazyleet", nil, "0.3.1", MethodHomebrew, "v0.3.1"},
		{"release archive", "/usr/local/bin/lazyleet", nil, "0.3.1", MethodBinary, "v0.3.1"},
		{"go install @version", "/home/u/go/bin/lazyleet", proxyBuild, "dev", MethodGoInstall, "v0.3.1"},
		{"go install from checkout", "/home/u/go/bin/lazyleet", checkoutBuild, "dev", MethodSource, ""},
		{"go run", "/tmp/go-build123/exe/lazyleet", develBuild, "dev", MethodSource, ""},
	}
	for _, c := range cases {
		got := detect(c.exe, c.info, c.ldVersion)
		if got.Method != c.want || got.Version != c.version {
			t.Errorf("%s: got %v %q, want %v %q", c.name, got.Method, got.Version, c.want, c.version)
		}
	}
}

func releaseServer(t *testing.T, tag string, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+Repo+"/releases/latest" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(hits, 1)
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.test/r"}`, tag)
	}))
}

func TestCheckerCachesForADay(t *testing.T) {
	var hits int32
	srv := releaseServer(t, "v0.4.0", &hits)
	defer srv.Close()

	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	c := NewChecker(filepath.Join(t.TempDir(), "sub", "update-check.json"))
	c.APIBase = srv.URL
	c.Now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		rel, err := c.Latest(context.Background(), false)
		if err != nil || rel.Tag != "v0.4.0" {
			t.Fatalf("Latest = %+v, %v", rel, err)
		}
	}
	if hits != 1 {
		t.Fatalf("GitHub hit %d times within the interval, want 1", hits)
	}
	if _, err := c.Latest(context.Background(), true); err != nil || hits != 2 {
		t.Fatalf("force should bypass the cache (hits=%d, err=%v)", hits, err)
	}
	now = now.Add(CheckInterval + time.Minute)
	if _, err := c.Latest(context.Background(), false); err != nil || hits != 3 {
		t.Fatalf("an expired cache should refetch (hits=%d, err=%v)", hits, err)
	}
}

func TestCheckerRejectsBadResponses(t *testing.T) {
	var hits int32
	srv := releaseServer(t, "nightly", &hits)
	defer srv.Close()
	c := NewChecker("")
	c.APIBase = srv.URL
	if _, err := c.Latest(context.Background(), false); err == nil {
		t.Fatal("a non-semver tag should be an error")
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer down.Close()
	c.APIBase = down.URL
	if _, err := c.Latest(context.Background(), false); err == nil {
		t.Fatal("a non-200 response should be an error")
	}
}

func tarGz(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body []byte
	}{{"README.md", []byte("readme")}, {name, body}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.body); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// assetServer serves a goreleaser-shaped release for linux/amd64. corrupt
// makes checksums.txt disagree with the archive.
func assetServer(t *testing.T, bin []byte, corrupt bool) (*httptest.Server, Release) {
	t.Helper()
	const name = "lazyleet_0.4.0_linux_amd64.tar.gz"
	archive := tarGz(t, "lazyleet", bin)
	sum := sha256.Sum256(archive)
	if corrupt {
		sum[0] ^= 0xff
	}
	sums := fmt.Sprintf("%s  %s\n%s  lazyleet_0.4.0_darwin_arm64.tar.gz\n",
		hex.EncodeToString(sum[:]), name, strings.Repeat("0", 64))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + name:
			w.Write(archive)
		case "/checksums.txt":
			w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, Release{Tag: "v0.4.0", Assets: []Asset{
		{Name: name, URL: srv.URL + "/" + name},
		{Name: "checksums.txt", URL: srv.URL + "/checksums.txt"},
	}}
}

// fakeVersionCommand answers `<exe> --version` with whatever the file at exe
// now contains, standing in for running the freshly installed binary.
func fakeVersionCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	if len(args) == 1 && args[0] == "--version" {
		return exec.CommandContext(ctx, "cat", name)
	}
	return exec.CommandContext(ctx, name, args...)
}

func binaryUpdater() *Updater {
	u := NewUpdater()
	u.GOOS, u.GOARCH = "linux", "amd64"
	u.Command = fakeVersionCommand
	return u
}

func TestApplyBinaryReplacesVerifiedDownload(t *testing.T) {
	srv, rel := assetServer(t, []byte("lazyleet version 0.4.0 (abc1234, 2026-09-22)\n"), false)
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "lazyleet")
	if err := os.WriteFile(exe, []byte("lazyleet version 0.3.1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	in := Install{Method: MethodBinary, Version: "v0.3.1", Exe: exe, Launch: exe}
	u := binaryUpdater()
	if ok, why := u.CanApply(in); !ok {
		t.Fatalf("CanApply = false: %s", why)
	}
	if err := u.Apply(context.Background(), in, rel); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(exe)
	if !strings.Contains(string(got), "0.4.0") {
		t.Fatalf("binary not replaced: %q", got)
	}
	if st, _ := os.Stat(exe); st.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755 preserved", st.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Errorf("leftover files next to the binary: %v", entries)
	}
}

func TestApplyBinaryRejectsChecksumMismatch(t *testing.T) {
	srv, rel := assetServer(t, []byte("evil"), true)
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "lazyleet")
	os.WriteFile(exe, []byte("lazyleet version 0.3.1\n"), 0o755)
	in := Install{Method: MethodBinary, Exe: exe, Launch: exe}
	err := binaryUpdater().Apply(context.Background(), in, rel)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Apply err = %v, want checksum mismatch", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "lazyleet version 0.3.1\n" {
		t.Fatalf("binary must be untouched after a failed verification, got %q", got)
	}
}

func TestApplyBinaryNoAssetForPlatform(t *testing.T) {
	srv, rel := assetServer(t, []byte("x"), false)
	defer srv.Close()
	u := binaryUpdater()
	u.GOOS = "plan9"
	exe := filepath.Join(t.TempDir(), "lazyleet")
	os.WriteFile(exe, nil, 0o755)
	err := u.Apply(context.Background(), Install{Method: MethodBinary, Exe: exe, Launch: exe}, rel)
	if err == nil || !strings.Contains(err.Error(), ErrNoAsset.Error()) {
		t.Fatalf("err = %v, want ErrNoAsset", err)
	}
}

func TestApplyGoInstallTargetsRunningBinaryDir(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "lazyleet")
	var install *exec.Cmd
	var gotArgs []string
	u := NewUpdater()
	u.LookPath = func(string) (string, error) { return "/usr/bin/go", nil }
	u.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if len(args) == 1 && args[0] == "--version" {
			return exec.CommandContext(ctx, "echo", "lazyleet version v0.4.0")
		}
		gotArgs = append([]string{name}, args...)
		install = exec.CommandContext(ctx, "true")
		return install
	}
	in := Install{Method: MethodGoInstall, Exe: exe, Launch: exe}
	if err := u.Apply(context.Background(), in, Release{Tag: "v0.4.0"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "/usr/bin/go install " + ModulePath + "@v0.4.0"
	if strings.Join(gotArgs, " ") != want {
		t.Fatalf("ran %q, want %q", strings.Join(gotArgs, " "), want)
	}
	found := false
	for _, e := range install.Env {
		found = found || e == "GOBIN="+dir
	}
	if !found {
		t.Errorf("GOBIN not pointed at %s", dir)
	}
}

func TestApplyReportsVersionMismatch(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "lazyleet")
	u := NewUpdater()
	u.LookPath = func(string) (string, error) { return "/usr/bin/brew", nil }
	var calls []string
	u.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if len(args) == 1 && args[0] == "--version" {
			return exec.CommandContext(ctx, "echo", "lazyleet version 0.3.1")
		}
		calls = append(calls, strings.Join(args, " "))
		return exec.CommandContext(ctx, "true")
	}
	in := Install{Method: MethodHomebrew, Exe: exe, Launch: exe}
	err := u.Apply(context.Background(), in, Release{Tag: "v0.4.0"})
	if err == nil || !strings.Contains(err.Error(), "still reports") {
		t.Fatalf("err = %v, want a version mismatch", err)
	}
	want := []string{"upgrade " + BrewFormula, "update --quiet", "upgrade " + BrewFormula}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("brew calls = %q, want %q (retry after refreshing the tap)", calls, want)
	}
}

func TestSourceBuildIsNeverApplied(t *testing.T) {
	u := NewUpdater()
	in := Install{Method: MethodSource}
	if ok, _ := u.CanApply(in); ok {
		t.Fatal("source builds must not auto-update")
	}
	if err := u.Apply(context.Background(), in, Release{Tag: "v0.4.0"}); err == nil {
		t.Fatal("Apply on a source build should fail")
	}
}
