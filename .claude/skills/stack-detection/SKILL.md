---
name: stack-detection
description: Patterns for writing stack detectors in internal/detect/ — the Detector interface, the shared Snapshot, the registry and init() pattern, confidence tier conventions, evidence rules, FS-walk discipline (5k cap, hard-coded skips, root-.gitignore handling), and when to split into multiple single-purpose detectors versus emitting two related signals from one. Use this whenever adding a detector for a new language/framework/manager/CI/infra/monorepo system, modifying an existing detector, or reviewing detector tests. Apply also when changing internal/detect/walk.go, snapshot.go, or deps.go — these are the seams every detector relies on, and breaking them silently downgrades every detector's accuracy.
---

# Writing stack detectors

`internal/detect/` is omc's eyes. Every signal it emits ends up in the TUI's "why we picked this profile" panel and gates which template components apply. **Detectors are not allowed to be wrong silently** — every signal carries the exact files that triggered it, and confidence is a tiered, documented value, not a vibe.

## The interface and the snapshot

```go
type Detector interface {
    ID() string
    Detect(snap *Snapshot) []Signal
}

type Signal struct {
    Name       string
    Confidence float64  // see "Confidence tiers" below
    Evidence   []string // repo-relative file paths, slash-separated, sorted
    Tags       []string // "language" | "package-manager" | "framework" | "db" | "orm" | "infra" | "ci" | "monorepo"
}
```

Detectors never see `fs.FS` directly. They get a `*Snapshot` — a single, pre-walked view that all detectors share. This matters: the spec caps the walk at 5,000 files. If every detector re-walked, even 30 detectors against a real monorepo would burn the budget on duplicate work and produce non-deterministic truncation.

```go
type Snapshot struct {
    FS        fs.FS                  // gitignore-aware; excluded paths are absent
    Files     []string               // sorted, repo-relative, slash-separated, ≤ 5,000
    ByName    map[string][]string    // basename → matching paths
    ByExt     map[string][]string    // ".tf" → matching paths
    Truncated bool
    Deps      *DepIndex              // lazy
}

func (s *Snapshot) Has(basename string) (path string, ok bool)
func (s *Snapshot) Glob(pattern string) []string  // doublestar over s.Files
func (s *Snapshot) Read(path string) ([]byte, error)
func (s *Snapshot) HasDep(eco, name string) (version string, ok bool)
```

A detector's body should usually be five to ten lines. If it's growing beyond that, you're probably hand-rolling a primitive that belongs on `Snapshot` or `DepIndex`.

## The registry

Each category file (`languages.go`, `frameworks.go`, …) registers its detectors from `init()`:

```go
func init() {
    register(detectorFunc{id: "django", fn: detectDjango})
    register(detectorFunc{id: "fastapi", fn: detectFastAPI})
}

func detectDjango(s *Snapshot) []Signal {
    var ev []string
    if _, ok := s.HasDep("python", "django"); ok {
        ev = append(ev, "pyproject.toml")
    }
    if p, ok := s.Has("manage.py"); ok {
        ev = append(ev, p)
    }
    if len(ev) == 0 {
        return nil
    }
    return []Signal{{
        Name:       "django",
        Confidence: scoreDjango(ev),
        Evidence:   ev,
        Tags:       []string{"framework"},
    }}
}
```

Don't add explicit registration calls in `detect.go`. The `init()` pattern keeps each new detector to a single-file diff: drop a function in the right category file, add one `register(...)` line, write a test. The registry is a private global; tests use `RunWith([]Detector, *Snapshot)` to inject a controlled set when needed.

## Confidence tiers — pick one, don't invent

The conventional tiers live in `confidence.go`:

| Constant            | Value | When to use                                                       |
|---------------------|-------|-------------------------------------------------------------------|
| `ConfLockfile`      | 1.00  | A lockfile or definitive marker is present (pnpm-lock.yaml, go.sum, uv.lock, *.tf) |
| `ConfManifest`      | 0.95  | Canonical manifest present (package.json, pyproject.toml, go.mod, Cargo.toml) |
| `ConfDepDeclared`   | 0.85  | Dependency listed in a manifest with a real constraint            |
| `ConfFileConvention`| 0.70  | Stack-specific file/dir present (next.config.js, manage.py, prisma/schema.prisma) |
| `ConfHeuristic`     | 0.50  | Weaker filename match (e.g. any *.tf file → terraform)            |
| `ConfWeak`          | 0.30  | Last-resort signal; almost never appropriate alone                |

Stacking rule: if a detector has both a manifest dep *and* a config file, combine to `0.95` (cap at `ConfManifest`); never sum to >1.0. If you find yourself reaching for a value that isn't a tier, push back — either the signal is one of the existing tiers in disguise or it isn't strong enough to emit.

## Evidence rules

- Evidence is a slice of **actual repo-relative file paths** (slash-separated, sorted alphabetically), never prose. `"package.json"` is correct; `"package.json with next dep"` is not — the TUI builds the prose itself.
- If a single file triggered the signal, evidence is one entry. If multiple did, list them all up to a sane cap (5). Don't truncate silently — if you cap, sort first so the cap is deterministic.
- Use forward slashes even on Windows. The `Snapshot.Files` slice already normalizes; if you `filepath.Join` anywhere in a detector you've made a mistake.
- Never emit absolute paths. Never emit paths that escape the repo root.

## Walking responsibly

`walk.go` does the walk once, with these invariants:

1. **5,000-file cap.** Hit it, set `snap.Truncated = true`, return `fs.SkipAll`. Don't bump it. Detectors that need more files are detectors that should look at fewer files.
2. **Skip set.** A hardcoded directory blacklist is pruned *before* descent (`.git`, `node_modules`, `vendor`, `dist`, `build`, `.next`, `.venv`, `__pycache__`, `target`, `.cache`, …). This catches 95% of the "ignore me" intent without a gitignore parse.
3. **Root `.gitignore` only.** Parse `<root>/.gitignore` once at walk start; match every relative path against it. We do **not** descend into nested `.gitignore`s, `.git/info/exclude`, or global gitignore — that complexity isn't worth it before v1.0. The hardcoded skip set covers the bases.
4. **No symlinks.** `WalkDir` doesn't follow symlinks by default; keep it that way. A symlinked `node_modules` is the canonical "scan exploded" horror story.
5. **Read-only.** A detector must never write or stat outside the snapshot.

## One signal or many?

Default: **one detector emits one signal.** A `typescript` detector emits `typescript`; a separate `npm` detector emits `npm` based on `package-lock.json`. Don't bundle. Reasons:

- `omc stacks` reads cleaner: each row is a single concern.
- A monorepo with both `pnpm-lock.yaml` and `package-lock.json` (a pnpm migration in flight) needs *two* package-manager signals. A bundled detector would have to pick.
- Tests stay table-shaped — one detector, N input cases, N expected outputs.

The single allowed exception is "causally implied" companion signals: `next.js` may emit a `react` signal as a free by-product, because every Next.js install pulls React. Cap at two signals from one detector. Anything beyond that wants splitting.

## Adding a new detector — checklist

1. Pick the right file: `languages.go`, `pkgmanager.go`, `frameworks.go`, `orm.go`, `infra.go`, `ci.go`, `monorepo.go`, or (for genuinely new categories) a new file.
2. Write `detectFoo(s *Snapshot) []Signal` using `Snapshot.Has` / `Snapshot.Glob` / `Snapshot.HasDep`. Return `nil` when there's no evidence.
3. Pick confidence from the tier table; document the choice in a one-line comment.
4. Register from `init()` in the same file.
5. Add a `CatalogEntry` so `omc stacks` lists it (ID, tags, triggers, confidence range).
6. Add table tests in `<file>_test.go`: one positive, one stronger positive (if confidence stacks), one negative.
7. If the detector needs a new shared primitive on `Snapshot`/`DepIndex`, add it there with its own test — *then* call it from the detector. Don't inline a one-off parser.

## LLM-augmented detection (the optional second path)

omc has a *second* detection path in `internal/detect/llm.go` that shells out to the user's `claude` CLI for stack identification when (a) rule-based detectors return nothing and the user passes `--llm-augment`, or (b) the user explicitly requests it. **This does not change the rules above for rule-based detectors.** Rules are still file-only, sync, no shell-out, no source reading. The LLM path is a separate seam, governed by its own contract:

- It is **not** a `Detector` registered in this package's registry. It runs from the runner *after* rule-based detectors and only fills gaps.
- LLM-emitted signals carry `Tags: [..., "llm-inferred"]` so the TUI and `omc doctor` can surface "we guessed this" vs "we're sure."
- LLM signals **cap at `ConfHeuristic` (0.50)** regardless of how confident the model sounds. A rules-based detector at any tier always sorts above an LLM signal with the same name.
- **Rule wins on collision.** If rules already emitted `Name: "java"` and the LLM also emits `java`, the rules signal stands; the LLM duplicate is discarded entirely (not merged).
- LLM detection is **always opt-in or fallback**. It must never block, fail the parent run, exceed its budget cap, or pop a permission prompt. See the `claude-subprocess` skill for the full graceful-degradation ladder.

When you're adding a new rule-based detector, you do **not** need to coordinate with the LLM path — rules win, period. When you're touching `internal/detect/llm*.go`, read the `claude-subprocess` skill first.

## Common pitfalls

- **Don't shell out from a rule-based detector.** Detection is file-only at the rule layer. No `os/exec`, no `npm`/`pip`/`go list` calls. The user might not have those installed; the user might be on a CI runner with a different lockfile state; running user toolchains is a security boundary we don't cross. The LLM path in `internal/detect/llm.go` is the only place subprocesses live.
- **Don't slurp huge files.** `Snapshot.Read` enforces a per-file size cap (1 MiB). If a detector wants to read more, it's the wrong detector.
- **Don't read user code into memory whole.** A detector inspects manifests and config files, never source. Parsing `*.py` to find `import django` is what `DepIndex` is *for* — the manifest already lists the dep.
- **Don't depend on absolute paths or `os.Getwd()`.** Detectors take an `fs.FS`. They don't know where on disk it lives. Anything that wants `os.Getwd()` belongs in `cmd/omc/`.
- **Don't use `filepath` separators in evidence.** Always forward slashes. This burns Windows users otherwise.
- **Don't catch up on every nested `.gitignore`.** Root-only is the contract for v0.2.0.
- **Don't register the same ID twice.** The registry isn't a multimap. If you need two detectors that both sometimes emit `react`, return them from one detector or pick distinct IDs.

## Don't

- Don't introduce a `utils` file under `internal/detect/`. Helpers live on `Snapshot` or `DepIndex`.
- Don't emit a signal with empty evidence. A signal without a file path attached is unfalsifiable and the TUI has nothing to show the user.
- Don't synthesize confidence values outside the tier constants. If you need a new tier, add it to `confidence.go` and document it here in the same PR.
- Don't return errors from `Detect`. A detector that *can't* decide returns `nil`; the runner is allergic to detector failures bringing down the whole scan.
