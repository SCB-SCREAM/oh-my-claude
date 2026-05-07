// Package templates exposes the per-stack Claude Code templates that omc
// renders into a target project. Templates live under stacks/<stack>/ and are
// compiled into the binary via go:embed.
package templates

import "embed"

// FS is the read-only filesystem of every stack template embedded in the
// binary at build time. Callers walk it with fs.WalkDir / fs.ReadFile.
//
//go:embed all:stacks
var FS embed.FS
