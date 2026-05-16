// Package detect classifies a project's stack by walking its filesystem
// and asking the local `claude` CLI to produce a structured [Stack]
// payload (see types.go).
//
// The package is intentionally lean: there is one detector — the LLM —
// and one entry point — [Run]. Project-local caching ([RunCached]) means
// repeat invocations on an unchanged tree are free.
//
// Detection never executes the target project's code or its package
// managers. The only subprocess invoked is `claude`, called from a
// neutral cwd with `--tools ""` so it cannot read or write the project.
package detect

import (
	"context"
	"io/fs"
)

// Run walks fsys, builds a [Snapshot], and returns the project's [Stack]
// classification. cachePath is the project-local cache file
// ([DefaultCachePath] for the canonical location); pass "" to disable
// caching.
//
// Errors are returned for every failure mode — missing claude CLI,
// unauthenticated session, subprocess failure, response parse error. The
// caller decides whether to abort or degrade.
func Run(ctx context.Context, fsys fs.FS, cachePath string, opts Options) (*Result, error) {
	snap, err := NewSnapshot(fsys)
	if err != nil {
		return nil, err
	}
	opts.CachePath = cachePath
	return RunCached(ctx, snap, opts)
}
