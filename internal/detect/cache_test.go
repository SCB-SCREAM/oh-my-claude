package detect

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRunCached_FirstRunWritesCache(t *testing.T) {
	t.Parallel()
	snap, _ := NewSnapshot(fstest.MapFS{
		"go.mod":  &fstest.MapFile{Data: []byte("module x\n")},
		"main.go": &fstest.MapFile{Data: []byte("package main")},
	})
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "detected.json")
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"type":"cli","language_primary":"go"}`),
	}

	res, err := RunCached(context.Background(), snap, Options{
		Runner:    runner,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("RunCached: %v", err)
	}
	if res.FromCache || res.Refreshed {
		t.Errorf("first run: FromCache=%v Refreshed=%v, want both false", res.FromCache, res.Refreshed)
	}
	if got, ok := readCacheFile(cachePath); !ok {
		t.Fatalf("cache file not written at %s", cachePath)
	} else if got.Stack.Type != TypeCLI {
		t.Errorf("cached type = %q, want cli", got.Stack.Type)
	}
}

func TestRunCached_SecondRunHitsCacheNoSubprocess(t *testing.T) {
	t.Parallel()
	snap, _ := NewSnapshot(fstest.MapFS{
		"go.mod":  &fstest.MapFile{Data: []byte("module x\n")},
		"main.go": &fstest.MapFile{Data: []byte("package main")},
	})
	cachePath := filepath.Join(t.TempDir(), "detected.json")
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"type":"cli","language_primary":"go"}`),
	}

	// Prime the cache.
	if _, err := RunCached(context.Background(), snap, Options{Runner: runner, CachePath: cachePath}); err != nil {
		t.Fatal(err)
	}
	beforeCalls := len(runner.calls)

	// Second run: same snapshot, same file contents → cache hit, no subprocess.
	res, err := RunCached(context.Background(), snap, Options{Runner: runner, CachePath: cachePath})
	if err != nil {
		t.Fatalf("RunCached (second): %v", err)
	}
	if !res.FromCache {
		t.Errorf("second run: want FromCache=true, got %v", res.FromCache)
	}
	if n := countSubprocessCalls(runner.calls[beforeCalls:]); n != 0 {
		t.Errorf("cache hit triggered %d subprocess calls, want 0", n)
	}
}

func TestRunCached_FileContentChangeRefreshes(t *testing.T) {
	t.Parallel()
	cachePath := filepath.Join(t.TempDir(), "detected.json")
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"type":"cli","language_primary":"go"}`),
	}

	mfs := fstest.MapFS{
		"go.mod":  &fstest.MapFile{Data: []byte("module x\n")},
		"main.go": &fstest.MapFile{Data: []byte("package main")},
	}
	snap, _ := NewSnapshot(mfs)
	if _, err := RunCached(context.Background(), snap, Options{Runner: runner, CachePath: cachePath}); err != nil {
		t.Fatal(err)
	}
	beforeCalls := len(runner.calls)

	// Change go.mod content (same path, different bytes).
	mfs["go.mod"] = &fstest.MapFile{Data: []byte("module x\n\nrequire github.com/foo/bar v1.0.0\n")}
	snap2, _ := NewSnapshot(mfs)

	var logs strings.Builder
	res, err := RunCached(context.Background(), snap2, Options{
		Runner:    runner,
		CachePath: cachePath,
		Verbose:   func(f string, a ...any) { fmt.Fprintf(&logs, f+"\n", a...) },
	})
	if err != nil {
		t.Fatalf("RunCached (after change): %v", err)
	}
	if !res.Refreshed {
		t.Errorf("expected Refreshed=true, got %v", res.Refreshed)
	}
	if !contains(res.Changed, "go.mod") {
		t.Errorf("expected go.mod in Changed, got %v", res.Changed)
	}
	if !strings.Contains(logs.String(), "go.mod") {
		t.Errorf("verbose log should mention go.mod, got %q", logs.String())
	}
	if n := countSubprocessCalls(runner.calls[beforeCalls:]); n == 0 {
		t.Errorf("stale cache should trigger a fresh subprocess call")
	}
}

func TestRunCached_NewFileTriggersRefresh(t *testing.T) {
	t.Parallel()
	cachePath := filepath.Join(t.TempDir(), "detected.json")
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"type":"cli","language_primary":"go"}`),
	}
	mfs := fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x\n")}}
	snap, _ := NewSnapshot(mfs)
	if _, err := RunCached(context.Background(), snap, Options{Runner: runner, CachePath: cachePath}); err != nil {
		t.Fatal(err)
	}

	mfs["package.json"] = &fstest.MapFile{Data: []byte(`{"name":"x"}`)}
	snap2, _ := NewSnapshot(mfs)

	res, _ := RunCached(context.Background(), snap2, Options{Runner: runner, CachePath: cachePath})
	if !res.Refreshed || !contains(res.Changed, "package.json") {
		t.Errorf("expected package.json in Changed, got %+v", res.Changed)
	}
}

func TestRunCached_SkipCacheBypassesRead(t *testing.T) {
	t.Parallel()
	cachePath := filepath.Join(t.TempDir(), "detected.json")
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"type":"cli"}`),
	}
	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x")}})

	// Prime.
	_, _ = RunCached(context.Background(), snap, Options{Runner: runner, CachePath: cachePath})
	beforeCalls := len(runner.calls)

	res, _ := RunCached(context.Background(), snap, Options{Runner: runner, CachePath: cachePath, SkipCache: true})
	if res.FromCache {
		t.Errorf("SkipCache=true should bypass cache, got FromCache=true")
	}
	if n := countSubprocessCalls(runner.calls[beforeCalls:]); n == 0 {
		t.Errorf("SkipCache should force a fresh subprocess call")
	}
}

func TestRunCached_CorruptedCacheFallsBackToFreshRun(t *testing.T) {
	t.Parallel()
	cachePath := filepath.Join(t.TempDir(), "detected.json")
	// Write garbage to the cache path before calling.
	if err := writeCacheFile(cachePath, nil, Stack{}); err != nil {
		t.Fatal(err)
	}
	// Then overwrite with bogus JSON.
	if err := writeRaw(cachePath, []byte("{not json")); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"type":"cli"}`),
	}
	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x")}})
	res, err := RunCached(context.Background(), snap, Options{Runner: runner, CachePath: cachePath})
	if err != nil {
		t.Fatalf("corrupt cache should not bubble up an error: %v", err)
	}
	if res.FromCache {
		t.Errorf("corrupt cache must not be used; got FromCache=true")
	}
}

func TestDiffInputs(t *testing.T) {
	t.Parallel()
	cached := map[string]string{"a": "1", "b": "2", "c": "3"}
	current := map[string]string{"a": "1", "b": "CHANGED", "d": "4"}

	added, removed, changed := diffInputs(cached, current)
	if !equalStrings(added, []string{"d"}) {
		t.Errorf("added = %v, want [d]", added)
	}
	if !equalStrings(removed, []string{"c"}) {
		t.Errorf("removed = %v, want [c]", removed)
	}
	if !equalStrings(changed, []string{"b"}) {
		t.Errorf("changed = %v, want [b]", changed)
	}
}

func TestDefaultCachePath(t *testing.T) {
	t.Parallel()
	if got := DefaultCachePath(""); got != "" {
		t.Errorf("empty root → empty path, got %q", got)
	}
	if got := DefaultCachePath("/repo"); got != filepath.Join("/repo", ".claude", "omc", "detected.json") {
		t.Errorf("unexpected cache path: %q", got)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeRaw(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
