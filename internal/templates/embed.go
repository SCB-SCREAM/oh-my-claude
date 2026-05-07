// Package templates exposes the per-stack Claude Code templates that omc
// renders into a target project. Templates live under stacks/<stack>/ and are
// compiled into the binary via go:embed.
package templates

import "embed"

//go:embed all:stacks
var FS embed.FS
