package detect

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestSnapshot_IndexesAndIgnores(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"go.mod":                          &fstest.MapFile{Data: []byte("module x\n")},
		"cmd/app/main.go":                 &fstest.MapFile{Data: []byte("package main\n")},
		"node_modules/foo/index.js":       &fstest.MapFile{Data: []byte("// junk\n")},
		"vendor/golang.org/x/sys/unix.go": &fstest.MapFile{Data: []byte("package unix\n")},
		".git/config":                     &fstest.MapFile{Data: []byte("[core]\n")},
		"src/app.ts":                      &fstest.MapFile{Data: []byte("export {}\n")},
		"pkg/sub/file.go":                 &fstest.MapFile{Data: []byte("package sub\n")},
	}

	snap, err := NewSnapshot(fsys)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}

	want := []string{
		"cmd/app/main.go",
		"go.mod",
		"pkg/sub/file.go",
		"src/app.ts",
	}
	if got := strings.Join(snap.Files, ","); got != strings.Join(want, ",") {
		t.Errorf("snapshot files mismatch:\n got: %v\nwant: %v", snap.Files, want)
	}

	if _, ok := snap.Has("go.mod"); !ok {
		t.Error("expected Has(go.mod) = true")
	}
	if _, ok := snap.Has("nope.json"); ok {
		t.Error("Has(nope.json) should be false")
	}
	if got := snap.ByExt[".go"]; len(got) != 2 {
		t.Errorf(".go ext index: want 2 entries, got %v", got)
	}
}

func TestSnapshot_HonorsRootGitignore(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		".gitignore":     &fstest.MapFile{Data: []byte("/secret/\n*.log\n!keep.log\n")},
		"secret/key.pem": &fstest.MapFile{Data: []byte("k\n")},
		"app.log":        &fstest.MapFile{Data: []byte("oops\n")},
		"keep.log":       &fstest.MapFile{Data: []byte("ok\n")},
		"src/main.go":    &fstest.MapFile{Data: []byte("pkg\n")},
		"go.mod":         &fstest.MapFile{Data: []byte("module x\n")},
	}

	snap, err := NewSnapshot(fsys)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	for _, want := range []string{"go.mod", "keep.log", "src/main.go"} {
		if !snap.HasAt(want) {
			t.Errorf("expected %q to be indexed (.gitignore) — got %v", want, snap.Files)
		}
	}
	for _, banned := range []string{"secret/key.pem", "app.log"} {
		if snap.HasAt(banned) {
			t.Errorf("expected %q to be gitignored", banned)
		}
	}
}

func TestSnapshot_TruncationFlag(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{}
	for i := 0; i < fileCap+10; i++ {
		fsys[fmtFile(i)] = &fstest.MapFile{Data: []byte("x")}
	}
	snap, err := NewSnapshot(fsys)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}
	if !snap.Truncated {
		t.Errorf("expected Truncated=true once cap is exceeded")
	}
	if len(snap.Files) > fileCap {
		t.Errorf("indexed %d files, want ≤ %d", len(snap.Files), fileCap)
	}
}

func TestSnapshot_GlobDoubleStar(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		".github/workflows/ci.yml":      &fstest.MapFile{Data: []byte{}},
		".github/workflows/release.yml": &fstest.MapFile{Data: []byte{}},
		"docs/readme.md":                &fstest.MapFile{Data: []byte{}},
		"infra/main.tf":                 &fstest.MapFile{Data: []byte{}},
		"infra/modules/db/main.tf":      &fstest.MapFile{Data: []byte{}},
	}
	snap, err := NewSnapshot(fsys)
	if err != nil {
		t.Fatalf("NewSnapshot: %v", err)
	}

	got := snap.Glob(".github/workflows/*.yml")
	if len(got) != 2 {
		t.Errorf("workflows glob: want 2, got %v", got)
	}
	tf := snap.Glob("**/*.tf")
	if len(tf) != 2 {
		t.Errorf("**/*.tf glob: want 2, got %v", tf)
	}
}

func fmtFile(i int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	return string(letters[i%len(letters)]) + "/" +
		string(letters[(i/26)%len(letters)]) + "/" +
		strings.ReplaceAll(itoa(i), " ", "") + ".txt"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
