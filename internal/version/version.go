// Package version exposes build-time version information.
//
// All three vars are populated via -ldflags by goreleaser. For builds that
// skip ldflags (notably `go install …@latest`, which strips them), the
// init() below pulls the same data from runtime/debug.BuildInfo so users
// still see a real version, commit, and build date.
package version

import (
	"fmt"
	"runtime/debug"
)

// Build-time identifiers. Populated by goreleaser via -ldflags; otherwise
// hydrated from runtime/debug.ReadBuildInfo at process start.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func init() {
	hydrateFromBuildInfo()
}

// hydrateFromBuildInfo backfills the package vars from BuildInfo when ldflags
// were not applied. A field is filled only if it still holds its sentinel,
// so an explicit ldflag always wins.
func hydrateFromBuildInfo() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	if Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if Commit == "none" && s.Value != "" {
				if len(s.Value) > 7 {
					Commit = s.Value[:7]
				} else {
					Commit = s.Value
				}
			}
		case "vcs.time":
			if Date == "unknown" && s.Value != "" {
				Date = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" && Commit != "none" {
				Commit += "-dirty"
			}
		}
	}
}

// String returns a single line suitable for `omc --version`.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}
