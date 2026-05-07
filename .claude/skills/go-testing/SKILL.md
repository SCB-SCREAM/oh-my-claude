---
name: go-testing
description: Testing patterns used in this repo — table-driven tests, golden files under testdata/, testing/fstest.MapFS for synthetic project fixtures, t.TempDir() for write-path tests, go test -fuzz for merge logic, and teatest for TUI snapshots. Use this whenever writing or updating *_test.go files, designing test fixtures, adding a new detector that needs a fixture project, or unsure how to structure assertions for a new package. Apply also when reviewing existing tests for a refactor.
---

# Testing patterns in this repo

Tests are first-class. Every package ships with tests in the same directory (`foo.go` ↔ `foo_test.go`). Fixtures and golden outputs live under the package's `testdata/` directory (the Go toolchain ignores `testdata/` during builds — that's the whole point).

## Table-driven tests (default)

The first instinct for any new test should be a table. Each case has a name, inputs, and expected outputs. `t.Run` makes failures easy to locate and lets you `-run TestFoo/case_name`.

```go
func TestResolveProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		signals []detect.Signal
		want    []string // component IDs
		wantErr string
	}{
		{
			name:    "minimal/typescript yields claude.md and settings only",
			profile: "minimal",
			signals: []detect.Signal{{Name: "typescript"}},
			want:    []string{"claude-md", "settings"},
		},
		{
			name:    "unknown profile errors",
			profile: "wat",
			wantErr: "unknown profile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := profile.Resolve(tt.profile, tt.signals)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("components mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
```

Conventions:

- Subtests get descriptive sentence-case names. Spaces are fine — Go replaces them with underscores in `-run` filters.
- `t.Parallel()` in every leaf subtest unless there's a real reason not to.
- Use `github.com/google/go-cmp/cmp` for deep equality with diff output. Avoid `reflect.DeepEqual` + `%v` printf — diffs are unreadable.
- Error checks use a substring match (`strings.Contains`) on a documented sentinel phrase; don't assert exact error strings.

## Synthetic project trees with `fstest.MapFS`

For detector tests we don't want to touch the disk. The standard library ships `testing/fstest.MapFS` — an in-memory `fs.FS`. Use it.

```go
func TestDetectTypeScript(t *testing.T) {
	root := fstest.MapFS{
		"package.json": {Data: []byte(`{"name":"x","dependencies":{"react":"^18"}}`)},
		"tsconfig.json": {Data: []byte(`{}`)},
	}
	got := detect.Run(root)
	if !containsSignal(got, "typescript", 0.9) {
		t.Fatalf("expected typescript signal with confidence ≥ 0.9, got %+v", got)
	}
}
```

`fstest.MapFS` is faster than `t.TempDir()`, hermetic, and trivially diffable in test failures. Reach for it whenever the system under test takes an `fs.FS` (which most of `internal/detect` does).

## `t.TempDir()` for write-path tests

When the code under test *writes* files (e.g. `internal/apply`), use `t.TempDir()`. It's auto-cleaned and safe for parallel tests.

```go
func TestApplyWritesSettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plan := apply.Plan{Writes: []apply.FileWrite{
		{Path: filepath.Join(dir, ".claude/settings.json"), Body: []byte(`{}`)},
	}}
	if err := apply.Execute(plan); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil { t.Fatal(err) }
	if string(got) != `{}` {
		t.Errorf("contents = %q, want %q", got, `{}`)
	}
}
```

## Fixture projects under `testdata/fixtures/`

For larger or repeated synthetic projects (a full TS monorepo, say), put files on disk under `internal/detect/testdata/fixtures/<name>/`. Load with `os.DirFS`. Keeps test files readable and editable.

```
internal/detect/testdata/fixtures/
  ts-next/
    package.json
    next.config.js
    tsconfig.json
  py-fastapi/
    pyproject.toml
    app/main.py
```

```go
got := detect.Run(os.DirFS("testdata/fixtures/ts-next"))
```

Use `fstest.MapFS` for one-off small cases inside the test file; use disk fixtures when the input is large or shared across many tests.

## Detector fixtures specifically

`internal/detect/testdata/fixtures/<scenario>/` projects must look like the real thing — but no bigger than necessary. Rules:

- **Name by scenario, not stack.** Prefer `ts-next-pnpm`, `py-django-uv`, `monorepo-mixed` over generic `typescript`. The name should hint at every signal you expect.
- **Empty-but-present files are fine** for presence-only checks: an empty `pnpm-lock.yaml` or `go.sum` is still a definitive lockfile. Don't generate realistic content if the detector doesn't read it.
- **Real content for content-readers.** `package.json` and `pyproject.toml` must contain valid, parseable structures with the deps the detectors care about. Use minimal but real `dependencies` blocks.
- **Two assertion patterns:**
  - *Per-detector unit tests*: `fstest.MapFS` inline. Faster and the test file shows the exact input.
  - *Aggregate `Run()` tests*: real fixture on disk via `os.DirFS`. Assert on a **set** of expected signal IDs (`mustContainSignals(t, got, "next.js", "pnpm", "typescript")`), not full deep equality — keeps tests robust to confidence tier tweaks.
- **Don't add a fixture per detector.** A fixture covers many detectors at once. ~5 fixtures cover all 38 launch detectors — `ts-next-pnpm`, `py-django-uv`, `go-cli`, `monorepo-mixed`, `infra-only`.
- **Evidence is part of the assertion.** When asserting deep equality on a signal, include `Evidence`. A signal with the right name and the wrong evidence is a regression; the TUI relies on evidence being accurate.

## Golden files

When the system produces a multi-line text output (rendered template, full plan diff, formatted table), don't hard-code expected strings. Use a golden file.

```go
var update = flag.Bool("update", false, "update golden files")

func TestRenderClaudeMD(t *testing.T) {
	tests := []struct {
		name    string
		signals []detect.Signal
	}{
		{"typescript-recommended", typescriptRecommendedSignals()},
		{"python-minimal", pythonMinimalSignals()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := templates.RenderClaudeMD(tt.signals)
			if err != nil { t.Fatal(err) }
			golden := filepath.Join("testdata", "golden", tt.name+".md")
			if *update {
				_ = os.MkdirAll(filepath.Dir(golden), 0o755)
				_ = os.WriteFile(golden, got, 0o644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil { t.Fatal(err) }
			if diff := cmp.Diff(string(want), string(got)); diff != "" {
				t.Errorf("golden %s mismatch (-want +got):\n%s", golden, diff)
			}
		})
	}
}
```

Regenerate with `go test -update ./...` (or `make snapshot`). **Always read the diff before committing updated goldens** — the whole point is to catch unintended template churn.

## Fuzz tests

`internal/apply/settings_merge.go` is the highest-risk piece of write-path code (deep-merging arbitrary user JSON). Add a fuzz test:

```go
func FuzzSettingsMerge(f *testing.F) {
	f.Add(`{}`, `{}`)
	f.Add(`{"permissions":{"allow":["a"]}}`, `{"permissions":{"allow":["b"]}}`)
	f.Fuzz(func(t *testing.T, a, b string) {
		var av, bv map[string]any
		if json.Unmarshal([]byte(a), &av) != nil { t.Skip() }
		if json.Unmarshal([]byte(b), &bv) != nil { t.Skip() }
		out, err := apply.MergeSettings(av, bv)
		if err != nil {
			return // errors are fine; panics are not
		}
		// Round-trip: marshalled output must parse back.
		raw, err := json.Marshal(out)
		if err != nil { t.Fatalf("marshal: %v", err) }
		var rt map[string]any
		if err := json.Unmarshal(raw, &rt); err != nil {
			t.Fatalf("round-trip: %v", err)
		}
	})
}
```

Run locally with `go test -fuzz=FuzzSettingsMerge -fuzztime=30s ./internal/apply`. Don't run fuzz in regular CI (separate scheduled job, longer fuzztime).

## TUI snapshot tests with `teatest`

`teatest` (from `github.com/charmbracelet/x/exp/teatest`) drives a Bubble Tea program with scripted input and snapshots the rendered frames.

```go
func TestWelcomeScreen(t *testing.T) {
	tm := teatest.NewTestModel(t, tui.NewWelcome(),
		teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.RequireEqualOutput(t, tm.FinalOutput(t))
}
```

Run with `-update` to regenerate the golden output. `teatest` lives under `x/exp` so its API is unstable — pin the version, audit on bumps, and isolate it to `internal/tui/*_test.go`. Don't try to use it for non-TUI code.

## Coverage

We track coverage via Codecov but **don't gate PRs on it**. Aim for thoroughness in:

- `internal/detect` — every detector has at least one positive and one negative fixture test.
- `internal/apply` — every `ConflictPolicy` branch is exercised; merge logic has fuzz coverage.
- `internal/profile` — every profile/signal combination is in the table.

Don't chase a coverage number for its own sake. A 70% suite of meaningful tests beats a 95% suite that asserts on getters.

## Don't

- Don't write tests that depend on network, DNS, the user's home dir, or installed binaries.
- Don't use `time.Sleep` to "wait for" something. Use channels, `tea.Cmd`s in tests, or `eventually` patterns.
- Don't share mutable fixtures across subtests. Each subtest builds its own (cheap with `MapFS`).
- Don't compare large strings with `==`; use `cmp.Diff` so the failure message shows the diff.
- Don't put fixtures outside `testdata/`. The Go toolchain only ignores that exact directory name.
- Don't fuzz the detection runner; fuzz `DepIndex` parsers (the JSON/TOML surface that takes user input).
