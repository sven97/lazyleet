package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// ModulePath is the `go install` target for the lazyleet command.
const ModulePath = "github.com/sven97/lazyleet/cmd/lazyleet"

// BrewFormula is the fully qualified Homebrew formula name.
const BrewFormula = "sven97/tap/lazyleet"

// Method is how the running binary was installed, which decides how it
// upgrades itself.
type Method int

const (
	// MethodSource is a local build (`go build`, `go run`, `make build`, or
	// `go install ./cmd/lazyleet` from a checkout). It is never auto-updated.
	MethodSource Method = iota
	// MethodHomebrew is a binary inside a Homebrew Cellar.
	MethodHomebrew
	// MethodGoInstall is `go install github.com/sven97/lazyleet/cmd/lazyleet@vX`.
	MethodGoInstall
	// MethodBinary is a release archive unpacked by hand (goreleaser build).
	MethodBinary
)

func (m Method) String() string {
	switch m {
	case MethodHomebrew:
		return "Homebrew"
	case MethodGoInstall:
		return "go install"
	case MethodBinary:
		return "release binary"
	default:
		return "source build"
	}
}

// Install describes the running binary.
type Install struct {
	Method Method
	// Version is the installed release version ("v0.3.1"), or "" for a
	// source build with no meaningful version.
	Version string
	// Exe is the running binary's resolved path (symlinks followed).
	Exe string
	// Launch is the path to re-exec after an update. For Homebrew it is the
	// bin/ symlink, which moves to the new Cellar version, rather than Exe,
	// which points into the old one.
	Launch string
}

// Detect inspects the running process. ldVersion is main.version: "dev"
// unless goreleaser stamped it via -ldflags.
func Detect(ldVersion string) Install {
	exe, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	info, _ := debug.ReadBuildInfo()
	in := detect(exe, info, ldVersion)
	in.Launch = launchPath(exe)
	return in
}

// detect is Detect's pure core, split out for tests.
func detect(exe string, info *debug.BuildInfo, ldVersion string) Install {
	in := Install{Exe: exe}
	released := ldVersion != "" && ldVersion != "dev"
	if released {
		in.Version = "v" + strings.TrimPrefix(ldVersion, "v")
	}

	if inHomebrewCellar(exe) {
		in.Method = MethodHomebrew
		return in
	}
	if released {
		in.Method = MethodBinary
		return in
	}
	// `go install module@version` records the module version and no VCS
	// info (it builds from the module proxy). A build inside a checkout
	// records vcs.* settings, and since Go 1.24 a VCS-derived Main.Version
	// too, so the VCS check is what tells the two apart.
	if info != nil && IsSemver(info.Main.Version) && !hasVCSInfo(info) {
		in.Method = MethodGoInstall
		in.Version = info.Main.Version
		return in
	}
	in.Method = MethodSource
	return in
}

func inHomebrewCellar(exe string) bool {
	p := filepath.ToSlash(exe)
	return strings.Contains(p, "/Cellar/lazyleet/")
}

func hasVCSInfo(info *debug.BuildInfo) bool {
	for _, s := range info.Settings {
		if strings.HasPrefix(s.Key, "vcs.") {
			return true
		}
	}
	return false
}

// launchPath is the path the user actually ran, before symlinks are followed:
// argv[0] if it names a file, else the PATH entry it resolves to, falling
// back to the resolved exe.
func launchPath(exe string) string {
	arg0 := os.Args[0]
	if strings.ContainsRune(arg0, filepath.Separator) || strings.ContainsRune(arg0, '/') {
		if abs, err := filepath.Abs(arg0); err == nil {
			return abs
		}
	} else if p, err := exec.LookPath(arg0); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	return exe
}

// ManualCommand is the command a user can run themselves to upgrade.
func (in Install) ManualCommand() string {
	switch in.Method {
	case MethodHomebrew:
		return "brew upgrade lazyleet"
	case MethodGoInstall:
		return "go install " + ModulePath + "@latest"
	case MethodBinary:
		return "lazyleet update"
	default:
		return "git pull && go install ./cmd/lazyleet"
	}
}
