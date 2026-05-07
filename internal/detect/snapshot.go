package detect

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

// fileCap bounds the number of files a [Snapshot] indexes. Hitting it sets
// [Snapshot.Truncated] = true and stops the walk early. Detectors that
// need more files are detectors that should look at fewer files.
const fileCap = 5000

// readCap bounds [Snapshot.Read] per call. A detector should only ever
// read manifests and config files, never user source code; a megabyte is
// more than enough headroom for the largest pyproject.toml seen in the
// wild.
const readCap = 1 << 20 // 1 MiB

// skipDirs are directory basenames that are pruned before descent during
// the walk. Hardcoded because parsing every nested .gitignore is more
// complexity than v0.2.0 needs, and these cover the bulk of "ignore me".
var skipDirs = map[string]struct{}{
	".git":          {},
	"node_modules":  {},
	"vendor":        {},
	"dist":          {},
	"build":         {},
	".next":         {},
	".nuxt":         {},
	".svelte-kit":   {},
	".turbo":        {},
	".cache":        {},
	".venv":         {},
	"venv":          {},
	"env":           {},
	"__pycache__":   {},
	".mypy_cache":   {},
	".pytest_cache": {},
	".ruff_cache":   {},
	".tox":          {},
	"target":        {},
	".gradle":       {},
	".idea":         {},
	".vscode":       {},
	"coverage":      {},
	".terraform":    {},
}

// Snapshot is a single, pre-walked view of the project filesystem that all
// detectors share. Constructing it is the only place we pay the I/O cost
// of walking the tree.
type Snapshot struct {
	// FS is the underlying filesystem. Detectors should not call it
	// directly — go through [Snapshot.Read] / [Snapshot.Has] / etc.
	FS fs.FS

	// Files is the sorted, repo-relative, slash-separated list of every
	// file the walk indexed. Capped at [fileCap].
	Files []string

	// ByName maps a file basename to every path with that basename. The
	// canonical fast path for "is there a package.json anywhere?".
	ByName map[string][]string

	// ByExt maps a lower-cased extension (".tf", ".yaml") to matching
	// paths. Useful for "any *.tf file" style detectors.
	ByExt map[string][]string

	// Truncated is true if the walk hit [fileCap] before exhausting the
	// tree. Surfaced to the user so they understand why a deep monorepo
	// might be missing a signal.
	Truncated bool

	depsOnce sync.Once
	deps     *DepIndex
	depsErr  error
}

// NewSnapshot walks fsys (rooted at the repo root), respecting
// [skipDirs] and the root .gitignore, and returns a populated Snapshot.
// Returns an error only if walking the root itself fails — individual
// per-file errors are skipped silently because detection must remain
// best-effort.
func NewSnapshot(fsys fs.FS) (*Snapshot, error) {
	if fsys == nil {
		return nil, errors.New("detect: nil filesystem")
	}

	s := &Snapshot{
		FS:     fsys,
		ByName: map[string][]string{},
		ByExt:  map[string][]string{},
	}

	ignore := loadRootGitignore(fsys)

	walkErr := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip individual entries we can't read; surface only the
			// root failure (handled by the outer return).
			if p == "." {
				return err
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if p == "." {
			return nil
		}

		// Normalize to slash-separated relative path.
		rel := path.Clean(p)

		if d.IsDir() {
			base := path.Base(rel)
			if _, skip := skipDirs[base]; skip {
				return fs.SkipDir
			}
			if ignore.match(rel, true) {
				return fs.SkipDir
			}
			return nil
		}

		// Files: skip symlinks, devices, sockets — anything not regular.
		if !d.Type().IsRegular() {
			return nil
		}

		if ignore.match(rel, false) {
			return nil
		}

		s.Files = append(s.Files, rel)
		if len(s.Files) >= fileCap {
			s.Truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) {
		return nil, walkErr
	}

	sort.Strings(s.Files)
	for _, f := range s.Files {
		base := path.Base(f)
		s.ByName[base] = append(s.ByName[base], f)
		ext := strings.ToLower(path.Ext(f))
		if ext != "" {
			s.ByExt[ext] = append(s.ByExt[ext], f)
		}
	}
	return s, nil
}

// Has returns the first path with the given basename, if any.
func (s *Snapshot) Has(basename string) (string, bool) {
	paths := s.ByName[basename]
	if len(paths) == 0 {
		return "", false
	}
	return paths[0], true
}

// HasAt returns true if the given exact (slash-separated, repo-relative)
// path exists in the snapshot. O(log n) over [Snapshot.Files].
func (s *Snapshot) HasAt(p string) bool {
	idx := sort.SearchStrings(s.Files, p)
	return idx < len(s.Files) && s.Files[idx] == p
}

// Glob returns the subset of [Snapshot.Files] matching pattern. Supports
// the syntax of [path.Match] plus a leading "**/" wildcard meaning "any
// directory depth". Patterns are matched against the slash-separated
// repo-relative path.
func (s *Snapshot) Glob(pattern string) []string {
	if pattern == "" {
		return nil
	}
	var out []string
	doubleStar := strings.HasPrefix(pattern, "**/")
	tail := strings.TrimPrefix(pattern, "**/")
	for _, f := range s.Files {
		match, err := path.Match(pattern, f)
		if err == nil && match {
			out = append(out, f)
			continue
		}
		if doubleStar {
			// Try matching tail against successive suffixes:
			// "a/b/c.tf" against pattern "**/c.tf" → tail "c.tf"
			// matches the basename and any nested basename.
			rest := f
			for rest != "" {
				if m, err := path.Match(tail, rest); err == nil && m {
					out = append(out, f)
					break
				}
				slash := strings.IndexByte(rest, '/')
				if slash < 0 {
					break
				}
				rest = rest[slash+1:]
			}
		}
	}
	return out
}

// Read returns the contents of repo-relative path p, capped at [readCap].
// Detectors should treat the returned slice as read-only.
func (s *Snapshot) Read(p string) ([]byte, error) {
	f, err := s.FS.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, readCap))
}

// Deps returns the lazily-built [DepIndex]. The first call parses every
// known manifest in the snapshot; subsequent calls reuse the result.
// Detectors normally go through [Snapshot.HasDep] instead.
func (s *Snapshot) Deps() *DepIndex {
	s.depsOnce.Do(func() {
		s.deps, s.depsErr = buildDepIndex(s)
	})
	return s.deps
}

// HasDep reports whether the given dependency was declared in any manifest
// of the given ecosystem ("npm", "python", "go"). The returned version is
// the raw constraint string (e.g. "^14.0.0", ">=4.2,<5", "v1.21.0").
func (s *Snapshot) HasDep(ecosystem, name string) (string, bool) {
	idx := s.Deps()
	if idx == nil {
		return "", false
	}
	return idx.Lookup(ecosystem, name)
}
