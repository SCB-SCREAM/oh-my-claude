//go:build e2e

// End-to-end tests for the headless --no-tui pipeline. Built only with
// `go test -tags=e2e` (so `go test ./...` stays fast). Drives the full
// pipeline against a t.TempDir-populated synthetic fixture and asserts
// on stdout content + the invariant that the preview build writes
// nothing to disk.
//
// Detection is LLM-backed in the new design, so each test:
//   1. Installs a fake `claude` script on PATH (handles --version, auth
//      status, and -p invocations with canned JSON responses).
//   2. Pre-seeds .claude/omc/detected.json so the cache returns the
//      expected Stack and the fake `-p` call is never invoked — keeping
//      tests deterministic across runs.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

type fixtureFile struct {
	path, body string
}

// tsNextPnpm is a minimal TS / Next.js / pnpm fixture.
var tsNextPnpm = []fixtureFile{
	{path: "package.json", body: `{"name":"x","dependencies":{"next":"15.0.0","react":"19.0.0"}}`},
	{path: "tsconfig.json", body: `{"compilerOptions":{}}`},
	{path: "pnpm-lock.yaml", body: "lockfileVersion: 9.0\n"},
	{path: "next.config.js", body: "module.exports = {};\n"},
	{path: "src/app/page.tsx", body: "export default () => null\n"},
}

// goSimple is a minimal Go fixture.
var goSimple = []fixtureFile{
	{path: "go.mod", body: "module example.com/x\n\ngo 1.22\n"},
	{path: "main.go", body: "package main\n\nfunc main() {}\n"},
}

func writeFixture(t *testing.T, dir string, files []fixtureFile) {
	t.Helper()
	for _, f := range files {
		full := filepath.Join(dir, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// installFakeClaude writes a shell script that satisfies the omc init
// preflight (`claude --version` + `claude auth status`) and prepends
// its directory to PATH for the duration of the test. Returns the
// directory containing the fake binary.
func installFakeClaude(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake claude shim relies on POSIX shell")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  --version) echo "2.1.0 (Claude Code)" ; exit 0 ;;
  auth)      exit 0 ;;
  -p)        # printing a stub success envelope keeps it cheap even if invoked
             printf '{"type":"result","subtype":"success","is_error":false,"result":"{\"type\":\"library\"}","stop_reason":"end_turn"}'
             exit 0 ;;
esac
exit 1
`
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //#nosec G306 -- intentional 0755 for fake binary
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// seedCache pre-populates .claude/omc/detected.json with the given
// Stack so detection returns a cache hit without invoking the fake
// `-p`. Cache.Inputs is filled from the actual files on disk so the
// per-file SHA delta check passes.
func seedCache(t *testing.T, repoRoot string, stack detect.Stack, files []fixtureFile) {
	t.Helper()
	cachePath := filepath.Join(repoRoot, ".claude", "omc", "detected.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		t.Fatal(err)
	}

	// Build an inputs map matching the listing the production run would
	// produce. The session walks the FS and the cache verifies
	// per-file hashes — so the seeded inputs must reflect on-disk state.
	inputs := map[string]string{}
	for _, f := range files {
		inputs[f.path] = sha256Hex([]byte(f.body))
	}

	payload := detect.CacheFile{
		Version:    1,
		DetectedAt: time.Now().UTC(),
		Inputs:     inputs,
		Stack:      stack,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// sha256Hex mirrors cache.go: hash up to cacheReadCap (16KiB) of the
// file content. Callers pass file bodies; the cache's per-file SHA
// check then matches.
func sha256Hex(b []byte) string {
	if len(b) > 16<<10 {
		b = b[:16<<10]
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// snapshotTree returns a sorted list of every file under root,
// relative-pathed. Used to assert that nothing was written by the run.
func snapshotTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestE2E_Headless_TSNextPnpm(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, tsNextPnpm)
	installFakeClaude(t)
	seedCache(t, dir, detect.Stack{
		Type:            detect.TypeWebApp,
		LanguagePrimary: "typescript",
		PackageManager:  "pnpm",
		BuildCmd:        "pnpm build",
		TestCmd:         "pnpm test",
		LintCmd:         "pnpm lint",
		Formatter:       "prettier --write .",
		Frameworks:      []string{"next.js", "react"},
	}, tsNextPnpm)

	before := snapshotTree(t, dir)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"init", "--no-tui", "--yes", "--profile", "recommended"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init returned error: %v\nstderr:\n%s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"webapp",
		"typescript",
		"next.js",
		"selected",
		"plan:",
		"preview build",
		"no files were written",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, out)
		}
	}

	after := snapshotTree(t, dir)
	// The cache file we seeded is part of `before`; the run shouldn't
	// touch any other file.
	if !equalStringSlices(before, after) {
		t.Errorf("files changed on disk!\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestE2E_Headless_GoOnly_AllowlistRespected(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, goSimple)
	installFakeClaude(t)
	seedCache(t, dir, detect.Stack{
		Type:            detect.TypeCLI,
		LanguagePrimary: "go",
		PackageManager:  "go-modules",
		BuildCmd:        "go build ./...",
		TestCmd:         "go test -race ./...",
		Formatter:       "gofmt -w .",
	}, goSimple)

	before := snapshotTree(t, dir)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"init", "--no-tui", "--yes", "--profile", "recommended", "--components", "claude-md"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init returned error: %v\nstderr:\n%s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "go") {
		t.Errorf("expected 'go' in stdout; got:\n%s", out)
	}
	if strings.Count(out, "[x]") != 1 {
		t.Errorf("expected exactly 1 selected component; got:\n%s", out)
	}

	if !equalStringSlices(before, snapshotTree(t, dir)) {
		t.Errorf("disk modified by allowlist run")
	}
}

func TestE2E_Headless_EmptyRepoErrors(t *testing.T) {
	dir := t.TempDir()
	installFakeClaude(t)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"init", "--no-tui", "--yes", "--profile", "minimal"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("empty repo should error (nothing to classify)")
	}
	if !strings.Contains(fmt.Sprintf("%v", err), "empty project") {
		t.Errorf("expected 'empty project' in error; got: %v", err)
	}
}

func TestE2E_Preflight_FailsWithoutClaude(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, goSimple)
	// Deliberately scrub PATH of any claude — no installFakeClaude call.
	t.Setenv("PATH", dir) // only the fixture dir, no shims
	t.Chdir(dir)

	var sink bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&sink)
	cmd.SetErr(&sink)
	cmd.SetArgs([]string{"init", "--no-tui", "--yes", "--profile", "minimal"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("missing claude should be a hard error")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("preflight error should mention claude; got: %v", err)
	}
}

func TestE2E_Validate_RejectsUnknownComponent(t *testing.T) {
	dir := t.TempDir()
	installFakeClaude(t)
	t.Chdir(dir)

	cmd := newRootCmd()
	var sink bytes.Buffer
	cmd.SetOut(&sink)
	cmd.SetErr(&sink)
	cmd.SetArgs([]string{"init", "--no-tui", "--yes", "--profile", "recommended", "--components", "no-such-thing"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown-component to error")
	}
	if !strings.Contains(err.Error(), "unknown --components id") {
		t.Errorf("expected 'unknown --components id' error; got: %v", err)
	}
}

func equalStringSlices(a, b []string) bool {
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
