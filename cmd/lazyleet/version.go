package main

import "runtime/debug"

// These are overridden at release time via -ldflags
// (see .goreleaser.yaml). For `go install` builds they fall back to VCS info.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func versionString() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					return "dev+" + s.Value[:7]
				}
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
