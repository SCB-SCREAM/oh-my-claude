package detect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// cacheVersion is bumped whenever the on-disk cache shape changes
// incompatibly. Cache files with a different version are treated as
// invalid (forces a fresh detection run) rather than mis-parsed.
const cacheVersion = 1

// cacheReadCap bounds how many bytes per file are folded into the SHA
// fingerprint. Manifests and lockfiles are tiny; source files we hash
// only to detect "did anything meaningful change". 16 KiB is enough to
// catch any realistic manifest edit (package.json, go.mod, Cargo.toml,
// pyproject.toml) and keeps per-run I/O bounded — 500 files × 16 KiB ≈
// 8 MiB worst case, well under a second of cold-page reads.
const cacheReadCap = 16 << 10

// CacheFile is the on-disk JSON payload. Field tags are stable: bumping
// the cache layout requires incrementing [cacheVersion].
type CacheFile struct {
	Version    int               `json:"version"`
	DetectedAt time.Time         `json:"detected_at"`
	Inputs     map[string]string `json:"inputs"` // repo-relative path → hex sha256
	Stack      Stack             `json:"stack"`
}

// RunCached is the cache-aware wrapper around [RunStack]. It hashes the
// project file listing into Result.inputs, compares against the cache
// file (if any), and either:
//   - returns the cached Stack untouched (FromCache=true), or
//   - logs the delta and re-runs the LLM (Refreshed=true), or
//   - runs fresh on a missing cache.
//
// The cache file path is taken from opts.CachePath. An empty CachePath
// disables cache entirely — every call hits the LLM. SkipCache forces a
// fresh run regardless of cache freshness, but the new result is still
// written back.
//
// All cache I/O errors (read/parse/write) are non-fatal: the function
// falls through to a fresh LLM call rather than failing the run.
func RunCached(ctx context.Context, snap *Snapshot, opts Options) (*Result, error) {
	if snap == nil || len(snap.Files) == 0 {
		return nil, errors.New("detect: empty project (no files indexed)")
	}
	opts = opts.withDefaults()

	listing := buildListing(snap.Files)
	currentInputs := hashListingFiles(snap, listing)

	if opts.CachePath != "" && !opts.SkipCache {
		if cached, ok := readCacheFile(opts.CachePath); ok {
			added, removed, changed := diffInputs(cached.Inputs, currentInputs)
			if len(added)+len(removed)+len(changed) == 0 {
				opts.Verbose("detection: using cached result from %s", cached.DetectedAt.Format(time.RFC3339))
				return &Result{Stack: cached.Stack, Snapshot: snap, FromCache: true}, nil
			}
			// Stale: log the delta and fall through to the LLM call.
			opts.Verbose("detection: cache stale, re-running (%s)", deltaSummary(added, removed, changed))
			res, err := RunStack(ctx, snap, opts)
			if err != nil {
				return nil, err
			}
			res.Refreshed = true
			res.Changed = mergeChanged(added, removed, changed)
			_ = writeCacheFile(opts.CachePath, currentInputs, res.Stack)
			return res, nil
		}
	}

	// No cache (missing file, SkipCache, or empty CachePath): fresh run.
	res, err := RunStack(ctx, snap, opts)
	if err != nil {
		return nil, err
	}
	if opts.CachePath != "" {
		_ = writeCacheFile(opts.CachePath, currentInputs, res.Stack)
	}
	return res, nil
}

// hashListingFiles produces the input map from the snapshot. Each entry
// is hex(sha256(first 16 KiB)). Files that fail to open are skipped
// (their absence shows up as a "removed" entry on the next run, which is
// the right invalidation signal).
func hashListingFiles(snap *Snapshot, listing []string) map[string]string {
	out := make(map[string]string, len(listing))
	for _, p := range listing {
		h, ok := hashFile(snap.FS, p)
		if !ok {
			continue
		}
		out[p] = h
	}
	return out
}

func hashFile(fsys fs.FS, p string) (string, bool) {
	f, err := fsys.Open(p)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.CopyN(h, f, cacheReadCap); err != nil && !errors.Is(err, io.EOF) {
		// Short reads (file < cap) are fine; bail only on real I/O errors.
		if _, ok := err.(*fs.PathError); ok {
			return "", false
		}
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

// diffInputs returns three sorted lists describing what's different
// between the cached inputs map and the current one.
//
//	added   = paths in current that were not cached (new files)
//	removed = paths cached that no longer exist
//	changed = paths in both whose hashes differ
func diffInputs(cached, current map[string]string) (added, removed, changed []string) {
	for p, h := range current {
		old, ok := cached[p]
		switch {
		case !ok:
			added = append(added, p)
		case old != h:
			changed = append(changed, p)
		}
	}
	for p := range cached {
		if _, ok := current[p]; !ok {
			removed = append(removed, p)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)
	return added, removed, changed
}

// deltaSummary turns the three diff lists into a single human-readable
// string suitable for Verbose output. Caps at 3 files per category so
// the line stays terminal-friendly on big refactors.
func deltaSummary(added, removed, changed []string) string {
	const showLimit = 3
	parts := make([]string, 0, 3)
	if n := len(added); n > 0 {
		parts = append(parts, fmt.Sprintf("added %d (%s)", n, joinHead(added, showLimit)))
	}
	if n := len(removed); n > 0 {
		parts = append(parts, fmt.Sprintf("removed %d (%s)", n, joinHead(removed, showLimit)))
	}
	if n := len(changed); n > 0 {
		parts = append(parts, fmt.Sprintf("changed %d (%s)", n, joinHead(changed, showLimit)))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "; " + p
	}
	return out
}

func joinHead(items []string, limit int) string {
	if len(items) <= limit {
		return joinComma(items)
	}
	return joinComma(items[:limit]) + ", …"
}

func joinComma(items []string) string {
	if len(items) == 0 {
		return ""
	}
	out := items[0]
	for _, s := range items[1:] {
		out += ", " + s
	}
	return out
}

// mergeChanged flattens the three diff lists into one sorted slice for
// Result.Changed. Used by callers that want to display "what triggered
// the re-run" without re-running the diff themselves.
func mergeChanged(added, removed, changed []string) []string {
	out := make([]string, 0, len(added)+len(removed)+len(changed))
	out = append(out, added...)
	out = append(out, removed...)
	out = append(out, changed...)
	sort.Strings(out)
	return out
}

// ── on-disk read/write ─────────────────────────────────────────────────

func readCacheFile(path string) (*CacheFile, bool) {
	data, err := os.ReadFile(path) //#nosec G304 -- path is supplied by session layer (repo-rooted)
	if err != nil {
		return nil, false
	}
	var f CacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, false
	}
	if f.Version != cacheVersion {
		return nil, false
	}
	if f.Inputs == nil {
		return nil, false
	}
	return &f, true
}

func writeCacheFile(path string, inputs map[string]string, stack Stack) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cache mkdir: %w", err)
	}
	payload := CacheFile{
		Version:    cacheVersion,
		DetectedAt: time.Now().UTC(),
		Inputs:     inputs,
		Stack:      stack,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// DefaultCachePath returns the canonical project-local cache file path
// for the given repository root. Wrapping it in a helper keeps the path
// definition in one place — change it here and every caller follows.
func DefaultCachePath(repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	return filepath.Join(repoRoot, ".claude", "omc", "detected.json")
}
