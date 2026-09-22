package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// maxArchiveSize bounds a release download; real archives are ~15 MB.
const maxArchiveSize = 200 << 20

// Updater upgrades an Install to a given release.
type Updater struct {
	HTTP *http.Client
	// LookPath and Command are indirections for tests.
	LookPath func(string) (string, error)
	Command  func(ctx context.Context, name string, args ...string) *exec.Cmd
	GOOS     string
	GOARCH   string
}

// NewUpdater returns an Updater for the current platform.
func NewUpdater() *Updater {
	return &Updater{
		HTTP:     &http.Client{Timeout: 5 * time.Minute},
		LookPath: exec.LookPath,
		Command:  exec.CommandContext,
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
	}
}

// CanApply reports whether in can be upgraded automatically, and if not, why.
func (u *Updater) CanApply(in Install) (bool, string) {
	switch in.Method {
	case MethodHomebrew:
		if _, err := u.brewPath(in); err != nil {
			return false, "brew not found on PATH"
		}
	case MethodGoInstall:
		if _, err := u.LookPath("go"); err != nil {
			return false, "go not found on PATH"
		}
	case MethodBinary:
		if err := dirWritable(filepath.Dir(in.Exe)); err != nil {
			return false, "no write access to " + filepath.Dir(in.Exe)
		}
	default:
		return false, "built from source"
	}
	return true, ""
}

// Apply upgrades in to rel, then confirms the binary at in.Launch reports the
// new version. Tool output (brew, go) is captured, not streamed, so it's safe
// to call while the TUI owns the terminal; it is included in any error.
func (u *Updater) Apply(ctx context.Context, in Install, rel Release) error {
	var err error
	switch in.Method {
	case MethodHomebrew:
		err = u.applyHomebrew(ctx, in, rel)
	case MethodGoInstall:
		err = u.applyGoInstall(ctx, in, rel)
	case MethodBinary:
		err = u.applyBinary(ctx, in, rel)
	default:
		return fmt.Errorf("lazyleet was built from source; update it with: %s", in.ManualCommand())
	}
	if err != nil {
		return err
	}
	return u.verify(ctx, in.Launch, rel)
}

func (u *Updater) brewPath(in Install) (string, error) {
	if p, err := u.LookPath("brew"); err == nil {
		return p, nil
	}
	// Not on PATH (e.g. launched from a GUI terminal with a bare PATH): the
	// prefix owning the Cellar has brew in its bin/.
	p := filepath.ToSlash(in.Exe)
	if i := strings.Index(p, "/Cellar/"); i >= 0 {
		brew := filepath.FromSlash(p[:i] + "/bin/brew")
		if _, err := os.Stat(brew); err == nil {
			return brew, nil
		}
	}
	return "", errors.New("brew not found")
}

func (u *Updater) applyHomebrew(ctx context.Context, in Install, rel Release) error {
	brew, err := u.brewPath(in)
	if err != nil {
		return err
	}
	if err := u.run(ctx, nil, brew, "upgrade", BrewFormula); err != nil {
		return err
	}
	// brew only refreshes taps every few hours on its own, so a just-published
	// release can be missed. If so, refresh explicitly and try once more.
	if u.verify(ctx, in.Launch, rel) == nil {
		return nil
	}
	if err := u.run(ctx, nil, brew, "update", "--quiet"); err != nil {
		return err
	}
	return u.run(ctx, nil, brew, "upgrade", BrewFormula)
}

func (u *Updater) applyGoInstall(ctx context.Context, in Install, rel Release) error {
	goBin, err := u.LookPath("go")
	if err != nil {
		return errors.New("go not found on PATH")
	}
	// Install over the running binary rather than wherever GOBIN/GOPATH
	// currently point, so the PATH entry the user runs is what gets updated.
	env := append(os.Environ(), "GOBIN="+filepath.Dir(in.Exe))
	return u.run(ctx, env, goBin, "install", ModulePath+"@"+rel.Tag)
}

func (u *Updater) run(ctx context.Context, env []string, name string, args ...string) error {
	cmd := u.Command(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w%s", filepath.Base(name), strings.Join(args, " "), err, tail(out.String()))
	}
	return nil
}

// verify runs `<launch> --version` and checks it reports rel.
func (u *Updater) verify(ctx context.Context, launch string, rel Release) error {
	out, err := u.Command(ctx, launch, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("check new version: %w%s", err, tail(string(out)))
	}
	want := strings.TrimPrefix(rel.Tag, "v")
	for _, f := range strings.Fields(string(out)) {
		if strings.TrimPrefix(f, "v") == want {
			return nil
		}
	}
	return fmt.Errorf("update finished but %s still reports %q (wanted %s)",
		launch, strings.TrimSpace(string(out)), rel.Tag)
}

// applyBinary replaces a release binary with the one from rel's archive,
// after checking the archive against the release's checksums.txt.
func (u *Updater) applyBinary(ctx context.Context, in Install, rel Release) error {
	ver := strings.TrimPrefix(rel.Tag, "v")
	ext := ".tar.gz"
	if u.GOOS == "windows" {
		ext = ".zip"
	}
	name := fmt.Sprintf("lazyleet_%s_%s_%s%s", ver, u.GOOS, u.GOARCH, ext)
	archiveURL, sumsURL := "", ""
	for _, a := range rel.Assets {
		switch a.Name {
		case name:
			archiveURL = a.URL
		case "checksums.txt":
			sumsURL = a.URL
		}
	}
	if archiveURL == "" {
		return fmt.Errorf("%w (%s)", ErrNoAsset, name)
	}
	if sumsURL == "" {
		return errors.New("release has no checksums.txt; refusing to install an unverified download")
	}

	sums, err := u.download(ctx, sumsURL, 1<<20)
	if err != nil {
		return err
	}
	want, err := checksumFor(sums, name)
	if err != nil {
		return err
	}
	archive, err := u.download(ctx, archiveURL, maxArchiveSize)
	if err != nil {
		return err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s; download discarded", name)
	}

	binName := "lazyleet"
	if u.GOOS == "windows" {
		binName += ".exe"
	}
	var bin []byte
	if ext == ".zip" {
		bin, err = extractZip(archive, binName)
	} else {
		bin, err = extractTarGz(archive, binName)
	}
	if err != nil {
		return err
	}
	return replaceFile(in.Exe, bin, u.GOOS == "windows")
}

func (u *Updater) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "lazyleet-self-update")
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("download %s: larger than %d bytes", url, limit)
	}
	return body, nil
}

// checksumFor finds name's SHA-256 in a goreleaser checksums.txt
// ("<hex>  <filename>" per line).
func checksumFor(sums []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == name && len(f[0]) == sha256.Size*2 {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", name)
}

func extractTarGz(archive []byte, binName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == binName {
			return io.ReadAll(io.LimitReader(tr, maxArchiveSize))
		}
	}
	return nil, fmt.Errorf("archive does not contain %s", binName)
}

func extractZip(archive []byte, binName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	for _, f := range zr.File {
		if f.FileInfo().Mode().IsRegular() && filepath.Base(f.Name) == binName {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, maxArchiveSize))
		}
	}
	return nil, fmt.Errorf("archive does not contain %s", binName)
}

// replaceFile atomically swaps bin in for the file at path: write a sibling
// temp file, then rename over. Windows can't overwrite a running .exe, but it
// can rename one, so the old binary is moved aside first (and cleaned up by
// CleanupOld on the next start).
func replaceFile(path string, bin []byte, windows bool) error {
	mode := os.FileMode(0o755)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lazyleet-update-*")
	if err != nil {
		return fmt.Errorf("stage new binary: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return fmt.Errorf("stage new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("stage new binary: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("stage new binary: %w", err)
	}
	if windows {
		old := path + ".old"
		_ = os.Remove(old)
		if err := os.Rename(path, old); err != nil {
			return fmt.Errorf("move old binary aside: %w", err)
		}
		if err := os.Rename(tmpName, path); err != nil {
			_ = os.Rename(old, path)
			return fmt.Errorf("install new binary: %w", err)
		}
		return nil
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}

// CleanupOld removes a binary left behind by a Windows self-update.
func CleanupOld(exe string) {
	_ = os.Remove(exe + ".old")
}

func dirWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".lazyleet-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// tail returns the last few lines of tool output, formatted for appending to
// an error, or "" when there is none.
func tail(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}
	return ":\n" + strings.Join(lines, "\n")
}
