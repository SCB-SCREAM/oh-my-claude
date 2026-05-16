package detect

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseStackResponse_Happy(t *testing.T) {
	t.Parallel()
	stack, err := parseStackResponse(`{
	  "type": "cli",
	  "language_primary": "go",
	  "package_manager": "go-modules",
	  "build_cmd": "go build ./...",
	  "test_cmd": "go test -race ./...",
	  "lint_cmd": "golangci-lint run",
	  "formatter": "gofmt -w .",
	  "frameworks": ["cobra", "bubbletea"],
	  "notes": "Go-based TUI CLI using cobra commands and bubbletea screens."
	}`)
	if err != nil {
		t.Fatalf("parseStackResponse: %v", err)
	}
	if stack.Type != TypeCLI {
		t.Errorf("type = %q, want cli", stack.Type)
	}
	if stack.Formatter != "gofmt -w ." {
		t.Errorf("formatter not preserved: %q", stack.Formatter)
	}
	if len(stack.Frameworks) != 2 {
		t.Errorf("frameworks = %v, want 2", stack.Frameworks)
	}
}

func TestParseStackResponse_RejectsInvalidType(t *testing.T) {
	t.Parallel()
	_, err := parseStackResponse(`{"type":"unknown"}`)
	if err == nil {
		t.Fatal("expected error on invalid type")
	}
	if !strings.Contains(err.Error(), "invalid type") {
		t.Errorf("expected 'invalid type' in %q", err.Error())
	}
}

func TestParseStackResponse_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	if _, err := parseStackResponse(`not json`); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestSanitizeFrameworks_DedupAndCap(t *testing.T) {
	t.Parallel()
	in := []string{"React", "react", " NEXT.JS ", "next.js", "vue", "svelte", "solid", "preact", "alpine", "lit", "ember"}
	out := sanitizeFrameworks(in)
	if len(out) > 8 {
		t.Errorf("cap not enforced: %d entries: %v", len(out), out)
	}
	seen := map[string]bool{}
	for _, f := range out {
		if seen[f] {
			t.Errorf("duplicate %q in %v", f, out)
		}
		seen[f] = true
		if f != strings.ToLower(f) {
			t.Errorf("not lowercased: %q", f)
		}
	}
}

func TestSanitizeCommand_StripsControlChars(t *testing.T) {
	t.Parallel()
	if got := sanitizeCommand("go\x07test"); got != "" {
		t.Errorf("control-char string should be rejected, got %q", got)
	}
	if got := sanitizeCommand("  go test -race  "); got != "go test -race" {
		t.Errorf("trim failed: %q", got)
	}
}

func TestSanitizeNotes_CollapsesNewlines(t *testing.T) {
	t.Parallel()
	got := sanitizeNotes("line one\nline two\rline three")
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("notes still contains line breaks: %q", got)
	}
}

func TestRunStack_HardFailWhenClaudeMissing(t *testing.T) {
	t.Parallel()
	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x")}})
	runner := &fakeRunner{} // versionOK=false
	_, err := RunStack(context.Background(), snap, Options{Runner: runner})
	if !errors.Is(err, ErrClaudeMissing) {
		t.Errorf("want ErrClaudeMissing, got %v", err)
	}
}

func TestRunStack_HardFailWhenUnauthenticated(t *testing.T) {
	t.Parallel()
	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x")}})
	runner := &fakeRunner{versionOK: true} // authOK=false
	_, err := RunStack(context.Background(), snap, Options{Runner: runner})
	if !errors.Is(err, ErrClaudeUnauthenticated) {
		t.Errorf("want ErrClaudeUnauthenticated, got %v", err)
	}
}

func TestRunStack_Success(t *testing.T) {
	t.Parallel()
	snap, _ := NewSnapshot(fstest.MapFS{
		"go.mod":  &fstest.MapFile{Data: []byte("module x\n")},
		"main.go": &fstest.MapFile{Data: []byte("package main")},
	})
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response: makeFakeSuccess(`{
		  "type": "cli",
		  "language_primary": "go",
		  "build_cmd": "go build ./...",
		  "test_cmd": "go test -race ./..."
		}`),
	}
	got, err := RunStack(context.Background(), snap, Options{Runner: runner})
	if err != nil {
		t.Fatalf("RunStack: %v", err)
	}
	if got.Stack.Type != TypeCLI {
		t.Errorf("type = %q, want cli", got.Stack.Type)
	}
	if got.Stack.TestCmd != "go test -race ./..." {
		t.Errorf("test_cmd not roundtripped: %q", got.Stack.TestCmd)
	}
}
