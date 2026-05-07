// Package version exposes build-time version information.
//
// All three vars are populated via -ldflags by goreleaser; in dev builds they
// keep their default sentinels.
package version

import "fmt"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a single line suitable for `omc --version`.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}
