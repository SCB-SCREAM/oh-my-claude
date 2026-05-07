package detect

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestRun_RealisticTSMonorepo runs the *full registry* against a fixture
// designed to exercise multiple detector categories at once. This is the
// integration test that catches "detector A's signal accidentally
// suppresses B" regressions.
func TestRun_RealisticTSMonorepo(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"package.json": &fstest.MapFile{Data: []byte(`{
  "name": "monorepo",
  "devDependencies": {"typescript": "^5", "vite": "^5"},
  "dependencies": {"next": "^14", "react": "^18", "express": "^4"}
}`)},
		"pnpm-lock.yaml":           &fstest.MapFile{Data: []byte("lockfileVersion: '9.0'\n")},
		"pnpm-workspace.yaml":      &fstest.MapFile{Data: []byte("packages:\n  - apps/*\n")},
		"turbo.json":               &fstest.MapFile{Data: []byte("{}")},
		"tsconfig.json":            &fstest.MapFile{Data: []byte("{}")},
		"next.config.js":           &fstest.MapFile{Data: []byte("module.exports = {}\n")},
		"Dockerfile":               &fstest.MapFile{Data: []byte("FROM node:20\n")},
		".github/workflows/ci.yml": &fstest.MapFile{Data: []byte("on: push\n")},
		"infra/main.tf":            &fstest.MapFile{Data: []byte("terraform {}\n")},
	}
	got, snap, err := Run(fsys)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if snap.Truncated {
		t.Errorf("unexpected truncation")
	}

	want := []string{
		"typescript", "javascript", "next.js", "react", "vite", "express",
		"pnpm", "pnpm-workspace", "turborepo",
		"docker", "terraform", "github-actions",
	}
	for _, w := range want {
		if !hasSignal(got, w) {
			t.Errorf("missing signal %q\nall: %s", w, signalNames(got))
		}
	}
}

// TestRun_PythonDjango — most-common Python web stack.
func TestRun_PythonDjango(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"pyproject.toml": &fstest.MapFile{Data: []byte(`[project]
name = "demo"
dependencies = ["django>=5.0", "psycopg[binary]==3.1"]
`)},
		"manage.py":    &fstest.MapFile{Data: []byte("#!/usr/bin/env python\n")},
		"app/views.py": &fstest.MapFile{Data: []byte("from django.http import HttpResponse\n")},
		"uv.lock":      &fstest.MapFile{Data: []byte("version = 1\n")},
	}
	got, _, err := Run(fsys)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, w := range []string{"python", "django", "uv"} {
		if !hasSignal(got, w) {
			t.Errorf("missing signal %q\nall: %s", w, signalNames(got))
		}
	}
	// pip should NOT show up — uv.lock suppresses it.
	if hasSignal(got, "pip") {
		t.Errorf("pip should not be emitted alongside uv.lock\nall: %s", signalNames(got))
	}
}

// TestRun_GoServiceWithCI — minimal Go service with GH Actions.
func TestRun_GoServiceWithCI(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"go.mod":                     &fstest.MapFile{Data: []byte("module example.com/svc\n\ngo 1.22\n")},
		"go.sum":                     &fstest.MapFile{Data: []byte("\n")},
		"cmd/svc/main.go":            &fstest.MapFile{Data: []byte("package main\n")},
		".github/workflows/test.yml": &fstest.MapFile{Data: []byte("on: pull_request\n")},
	}
	got, _, err := Run(fsys)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, w := range []string{"go", "github-actions"} {
		if !hasSignal(got, w) {
			t.Errorf("missing signal %q\nall: %s", w, signalNames(got))
		}
	}
	// Confidence ordering check: lockfile > manifest > convention.
	for _, s := range got {
		if s.Name == "go" && s.Confidence != ConfLockfile {
			t.Errorf("go confidence: got %v, want ConfLockfile", s.Confidence)
		}
	}
}

// TestRunWith_Deterministic asserts the same inputs produce a stable order.
func TestRunWith_Deterministic(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"go.mod":       &fstest.MapFile{Data: []byte("module x\n")},
		"package.json": &fstest.MapFile{Data: []byte(`{"devDependencies":{"typescript":"^5"}}`)},
		"pyproject.toml": &fstest.MapFile{Data: []byte(`[project]
name = "x"
dependencies = []
`)},
	}
	a, _, _ := Run(fsys)
	b, _, _ := Run(fsys)
	if signalNames(a) != signalNames(b) {
		t.Errorf("non-deterministic ordering:\n a: %s\n b: %s", signalNames(a), signalNames(b))
	}
}

// TestCatalog_ContainsEveryRegisteredDetector ensures every Detector ID is
// documented; the stacks command depends on this.
func TestCatalog_ContainsEveryRegisteredDetector(t *testing.T) {
	t.Parallel()

	have := map[string]bool{}
	for _, e := range Catalog() {
		have[e.ID] = true
	}
	for _, d := range registry {
		if !have[d.ID()] {
			t.Errorf("detector %q has no Catalog entry", d.ID())
		}
	}
}

func hasSignal(sigs []Signal, name string) bool {
	for _, s := range sigs {
		if s.Name == name {
			return true
		}
	}
	return false
}

func signalNames(sigs []Signal) string {
	names := make([]string, len(sigs))
	for i, s := range sigs {
		names[i] = s.Name
	}
	return strings.Join(names, ",")
}
