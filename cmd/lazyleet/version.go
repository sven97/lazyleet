package main

import "runtime/debug"

// These are overridden at release time via -ldflags
// (see .goreleaser.yaml). For `go install` builds they fall back to build info.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func versionString() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			// `go install …@v0.3.1` records the module version but no VCS
			// info; a build inside a checkout records the revision.
			var rev string
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					rev = s.Value[:7]
				}
			}
			if rev != "" {
				return "dev+" + rev
			}
			if mv := info.Main.Version; mv != "" && mv != "(devel)" {
				return mv
			}
		}
	}
	if commit != "" {
		v += " (" + commit
		if date != "" {
			v += ", " + date
		}
		v += ")"
	}
	return v
}
