# oh-my-claude — Stack-aware Claude Code project setup CLI

## Context

`oh-my-claude` (binary: `omc`) is a greenfield TUI CLI that bootstraps a Claude Code project setup tailored to a project's stack. The repo currently contains only `LICENSE`, `README.md`, `.gitignore` — everything below is to be built from scratch.

**Why this exists.** Setting up Claude Code well per-stack is the difference between "magic" and "frustrating": a Python project benefits from different permission allowlists, hooks, slash commands, and MCP servers than a Go monorepo. Today, every team rediscovers these conventions. `omc` is "oh-my-zsh for Claude Code": opinionated, batteries-included starting points that detect the stack, let the user pick a profile, and generate `CLAUDE.md`, `.claude/settings.json`, hooks, slash commands, MCP recommendations, and subagent stubs. The intended outcome: a developer runs `omc` in a fresh repo and gets a working, sensible Claude Code setup in under a minute, with everything still hand-editable afterwards.

**Decisions already made (from clarifying Qs):**
- **Implementation language:** Go, with **Bubble Tea v2** + **Bubbles v2** + **Lip Gloss** for the TUI (v2 is the current major as of 2026; ~30% faster rendering and improved resize handling vs v1).
- **v1 scope:** Generator **plus** curated per-stack templates (hooks, slash commands, MCP recs, agent stubs) embedded in the binary.
- **Opinion level:** Ship `minimal | recommended | full` profiles, but every component is a checkbox the user can toggle before applying.

---

## Goals (v1)

1. Detect a project's language(s), package manager, frameworks, key libs, and CI/infra signals from files only — never executing user code.
2. Walk the user through a TUI flow: welcome → scan → profile → component checklist → diff preview → apply → next-steps.
3. Generate, into the project:
   - `CLAUDE.md` (stack-aware)
   - `.claude/settings.json` (permissions, hooks, model defaults)
   - `.claude/commands/*.md` (slash commands)
   - `.claude/agents/*.md` (subagent stubs)
   - `.mcp.json` snippet recommendations
   - `.gitignore` additions (e.g. `.claude/settings.local.json`)
4. Be **safe and idempotent**: never silently clobber; show diffs; back up; merge `settings.json` rather than overwrite.
5. Ship a `--yes`/non-interactive mode for CI / dotfile bootstraps.

## Non-goals (v1, defer to later)

- Updating/migrating existing `omc`-managed setups across template versions.
- Plugin system for community-contributed stack templates.
- Cloud sync, team-shared profiles.
- Internationalization.

---

## High-level architecture

```
oh-my-claude/
├── cmd/omc/main.go              // Cobra root, subcommands
├── internal/
│   ├── detect/                  // file-only stack detection
│   │   ├── detector.go          // Detector interface, registry, runner
│   │   ├── typescript.go
│   │   ├── python.go
│   │   ├── golang.go
│   │   ├── rust.go
│   │   ├── frameworks.go        // Next/Vite/Django/FastAPI/Rails…
│   │   ├── infra.go             // Docker, k8s, Terraform, CI
│   │   └── monorepo.go          // pnpm-workspace, turbo, nx, go.work
│   ├── templates/
│   │   ├── embed.go             // //go:embed all:templates
│   │   ├── typescript/…         // CLAUDE.md.tmpl, settings.json.tmpl, hooks/, commands/, agents/, mcp.json
│   │   ├── python/…
│   │   └── golang/…             // (3 launch stacks)
│   ├── profile/profile.go       // minimal | recommended | full → component lists
│   ├── component/               // Component = one renderable unit (file + metadata)
│   ├── apply/
│   │   ├── writer.go            // backup, dry-run, atomic write
│   │   ├── differ.go            // unified diff for preview pane
│   │   └── settings_merge.go    // deep-merge .claude/settings.json
│   ├── tui/
│   │   ├── app.go               // top-level Bubble Tea program
│   │   ├── welcome.go
│   │   ├── scan.go
│   │   ├── profile.go
│   │   ├── components.go
│   │   ├── preview.go
│   │   ├── apply.go
│   │   └── theme.go             // Lip Gloss styles, NO_COLOR support
│   └── version/version.go
├── testdata/fixtures/…          // synthetic projects per stack for tests
├── .goreleaser.yaml
├── Makefile
├── go.mod
└── README.md
```

### Key types (shape, not final)

```go
// internal/detect
type Signal struct {
    Name       string   // "typescript", "next.js", "postgres", "github-actions"
    Confidence float64  // 0..1
    Evidence   []string // file paths / matched lines that triggered it
    Tags       []string // "language", "framework", "db", "ci"
}
type Detector interface { Detect(fs.FS) []Signal }

// internal/component
type Component struct {
    ID          string                // "claude-md", "settings", "hook.format-on-save", "cmd.test"
    Title       string
    Description string
    Files       []TargetFile          // path + rendered content
    AppliesTo   func([]Signal) bool   // stack gating
    Profiles    []string              // "minimal", "recommended", "full"
    Conflicts   ConflictPolicy        // merge | overwrite-with-prompt | skip-if-exists
}

// internal/apply
type Plan struct {
    Writes  []FileWrite
    Backups []string
    Skips   []SkipReason
}
```

### Data flow

`detect.Run(fs)` → `[]Signal` → `profile.Resolve(profile, signals)` → `[]Component` → user toggles in TUI → `apply.BuildPlan(components)` → diff preview → `apply.Execute(plan)`.

---

## TUI flow (Bubble Tea)

1. **Welcome** — ASCII logo, version, one-line pitch, `[enter]` to start.
2. **Scan** — spinner, then a tree of detected signals grouped by category (`Language`, `Framework`, `Infra`, `CI`). Each row is editable: confirm, remove, or add (fuzzy-search of known signals). Confidence shown as `●●●○`.
3. **Profile** — three-pane split: profile list on the left, preview of what each profile installs on the right (`[Recommended]` is default).
4. **Components** — checklist grouped by category (`CLAUDE.md`, `Permissions`, `Hooks`, `Slash commands`, `Subagents`, `MCP servers`, `gitignore`). Pre-checked from profile. Space toggles, `i` shows description.
5. **Preview** — file tree of changes; right pane shows unified diff or full content for new files. `s` skips a file.
6. **Apply** — progress list (`✓ wrote .claude/settings.json`, `✓ backed up CLAUDE.md → .claude/.omc-backup-…/`).
7. **Done** — summary, link to docs, reminder to add `settings.local.json` to `.gitignore` (already done if user accepted that component), suggested next commands.

Headless fallback: `--no-tui` prints structured plan + applies. Required for CI.

---

## Detection strategy

- **File-only.** Walk the repo (respecting `.gitignore`), match against detectors. Bound to first ~5k files for huge monorepos; show a warning if truncated.
- **Layered detectors:** language → package manager → frameworks → key libs → DB → infra → CI → monorepo. Multiple matches expected and welcome.
- **Evidence-based.** Every signal carries the file(s) that triggered it so the TUI can show *why* something was detected.
- **Launch detector set (v1):**
  - Languages: TypeScript/JavaScript (`package.json`), Python (`pyproject.toml`, `requirements*.txt`, `setup.py`), Go (`go.mod`).
  - Stretch if cheap: Rust (`Cargo.toml`), Ruby (`Gemfile`), Java (`pom.xml` / `build.gradle*`).
  - Package managers: npm/yarn/pnpm/bun (lockfile presence); pip/poetry/uv (`uv.lock`, `poetry.lock`); go modules.
  - Frameworks: Next.js, Vite, Express, NestJS, Django, FastAPI, Flask.
  - DB/ORM: Prisma, Drizzle, SQLAlchemy, pg/mysql clients (via deps).
  - Infra: `Dockerfile`, `docker-compose*.yml`, `k8s/`/`kustomization.yaml`, `terraform/*.tf`.
  - CI: `.github/workflows/`, `.gitlab-ci.yml`, `circle.yml`.
  - Monorepo: `pnpm-workspace.yaml`, `turbo.json`, `nx.json`, `go.work`, `lerna.json`.

---

## Curated templates (the v1 "wow")

Three launch stacks fully fleshed out: **TypeScript/Node**, **Python**, **Go**. Each ships:

- **CLAUDE.md** — sections for project commands (test/build/lint/format/dev), conventions, stack-specific gotchas (e.g. Python: "use `uv`, never `pip install`"; TS: "respect tsconfig paths"; Go: "always run `go mod tidy` before commits").
- **settings.json** — pre-approved Bash allowlist tuned to the stack (e.g. `npm test`, `pnpm build`, `pytest -q`, `go test ./…`), conservative deny list for destructive ops, model defaults left blank for the user.
- **Hooks** — opt-in starters: `PostToolUse` formatter (prettier/ruff/gofmt), `PreToolUse` guard for `rm -rf`, optional `Stop` hook with a "did you run tests?" reminder.
- **Slash commands** — `/test`, `/lint`, `/format`, `/typecheck`, `/migrate` (when ORM detected), `/deploy` (when CI detected) as markdown stubs the user fills in.
- **Subagents** — `test-runner`, `migration-reviewer` (gated on DB signal).
- **MCP recs** — `.mcp.json` *suggestions* (commented out, user must opt-in): GitHub MCP if `.github/` present, Postgres MCP if Postgres detected, etc.

Each template lives in `internal/templates/<stack>/` and is loaded via `//go:embed`. Each component file declares its `AppliesTo` gate in a sibling `manifest.yaml`, so adding a new stack is "drop folder + add manifest" — no Go code changes.

---

## Safety / idempotency

- **Existing files.** For each target, if the file exists: show diff, offer `overwrite | merge | skip | back-up + write`. Default: `skip` for `CLAUDE.md`, `merge` for `settings.json`/`.gitignore`.
- **`settings.json` deep merge.** `permissions.allow` and `.deny` are arrays — dedupe and union. `hooks` keyed by event — append, don't replace. Anything ambiguous → prompt.
- **Backups.** Anything we modify gets copied to `.claude/.omc-backup-<unix-ts>/<original-path>`.
- **Dry-run.** `omc init --dry-run` prints the plan and exits 0 without touching disk.
- **Repo guard.** Refuse to run outside a git repo unless `--force`. Refuse to write paths that escape the repo root.
- **`.gitignore` nudge.** Always offer to add `.claude/settings.local.json` and `.claude/.omc-backup-*/`.

---

## CLI surface (v1)

- `omc init` — interactive TUI (default).
- `omc init --profile {minimal|recommended|full} --yes` — non-interactive.
- `omc init --dry-run` — show plan, don't write.
- `omc doctor` — checks: `claude` CLI on PATH, `.claude/settings.local.json` in `.gitignore`, no broken hook scripts, template version drift.
- `omc stacks` — list supported stacks and what each profile installs.
- `omc --version`.
- Deferred (v2+): `omc update`, `omc add <component>`, `omc remove <component>`.

---

## Distribution & release

**Local installation is the headline feature of M2.** The plan ships install paths *before* product features (detection, TUI, apply pipeline, templates) so every subsequent milestone is just `omc upgrade` away for users who installed early. The install experience also doubles as user research — if friction shows up here, every later milestone inherits it.

The audience for v0.x is "developers who already have Go and want to try a Claude Code bootstrap tool." That makes the easiest install path the one they already have wired up.

**Build & sign (once, in `release.yml`):**
- **goreleaser v2** produces binaries for `darwin/{amd64,arm64}`, `linux/{amd64,arm64}`, `windows/amd64` with archives, `checksums.txt`, and a software bill of materials (SBOM via `syft`).
- **`cosign` v3 keyless signing** (sigstore/Fulcio, free for OSS) on the checksums file. Users can verify with `cosign verify-blob --bundle …`.
- **GitHub Releases** is the canonical source of truth; everything else points at it.

**Recommended install paths (in order of priority):**

1. **`go install github.com/<owner>/oh-my-claude/cmd/omc@latest`** — *the headline*. Single command, zero infra, works the moment a tag exists. Caveat: `go install` strips `-ldflags`, so `omc --version` will read `(devel)` unless `internal/version` falls back to `runtime/debug.ReadBuildInfo()` (which it does — see "Cautions" below). This is the path the README leads with for v0.x.
2. **`install.sh`** generated by **[binstaller](https://github.com/binary-install/binstaller)** (the maintained godownloader successor). Auto-detects OS/arch, downloads from GitHub Releases, verifies SHA256s embedded in the script, installs to `~/.local/bin` (override with `$OMC_INSTALL_DIR`). Hosted at `https://raw.githubusercontent.com/<owner>/oh-my-claude/main/install.sh` *and* documented as the **download-then-inspect** form (`curl -fsSL …/install.sh -o install.sh && less install.sh && sh install.sh`) so security-conscious users have a clear path. We don't hand-roll this — binstaller handles checksum embedding, cosign attestations, musl/glibc detection, and reproducibility.
3. **Direct binary download** from GitHub Releases — universal fallback. README ships a copy-pasteable `gh release download` + `cosign verify-blob` + `sha256sum -c` recipe.
4. **Homebrew tap** (macOS + Linux) — `brew install <owner>/tap/omc`. Auto-published by goreleaser to a sibling tap repo. **Use `homebrew_casks:` in `.goreleaser.yaml`, NOT `brews:`** — the latter was deprecated in goreleaser v2.10 and slated for removal in v3. Casks now work on Linuxbrew too, so a single config covers both OSes.
5. **Scoop bucket** (Windows) — `scoop bucket add <owner> https://github.com/<owner>/scoop-bucket && scoop install omc`. Cheapest Windows path (own bucket, no review). Note: scoop manifests need a single archive per arch — goreleaser will error if `scoops:` selects multiple, so use the `ids:` selector.
6. **`mise` / `asdf` plugin** — nice-to-have for version-managed installs; defer until v0.2 unless trivially cheap.

**Cautions / gotchas (bake into M2 work):**

- **`go install` strips `-ldflags`.** Make `internal/version` read from `runtime/debug.ReadBuildInfo()` as a fallback so `vcs.revision` and the module pseudo-version surface even without ldflag injection. Without this, every `go install` user sees `(devel)`.
- **`brews:` is deprecated.** Migrate `.goreleaser.yaml` to `homebrew_casks:` from day one to avoid a v3 break. Goreleaser logs a deprecation every snapshot run until you fix it.
- **Don't write `install.sh` by hand.** Use binstaller. Hand-rolled scripts almost always skip checksum verification (direnv's still does in 2026). It's also more code than it looks once you handle musl, M1, ARM64, Windows-via-WSL edge cases.
- **`~/.local/bin` is the right default** for the install script (XDG, no sudo, mise/cargo/uv all use it). Fall back to `/usr/local/bin` only if `~/.local/bin` isn't writable or doesn't exist.
- **Don't pipe to `sh` unconditionally in the README.** Show the `curl | sh` line *and* the download-then-inspect form. It's table stakes in 2026.

**Deferred until v1.0 (post-product):**

- **WinGet** — manifest PR to `microsoft/winget-pkgs`. Review cycle is slow and they want stable versioning + signed binaries; revisit once the surface is stable.
- **homebrew-core formula** — submission bar is high; our own tap is fine until traction.
- **`mise` / `asdf` plugins** — only if there's user demand.

**Explicitly considered and rejected for v1:**

- **Linux distro packages** (deb/rpm/AUR/nix/snap) — high maintenance burden for early stage. Revisit once there's user demand.
- **Docker image** — adds little for a tool that needs to read/write the host filesystem. Skip.
- **npm wrapper** — tempting since the audience overlaps, but adds a Node dependency for a Go binary. Skip; let TS users `go install` or `brew install`.

**Reference implementations to copy from:**

- **install.sh generation:** [`binary-install/binstaller`](https://github.com/binary-install/binstaller).
- **install.sh prior art:** [mise.run script](https://mise.run/) — strict `set -eu`, `uname` detection incl. musl/glibc split, `shasum -c`, customizable install dir, cleanup.
- **goreleaser config:** [goreleaser's own `.goreleaser.yaml`](https://github.com/goreleaser/goreleaser) and [k9s](https://github.com/derailed/k9s) — both ship `homebrew_casks`, `scoops`, cosign signing, SBOM, and `checksums.txt` together.

**Versioning & release mechanics:**

- Semantic versioning (`v0.x.y` until API stable).
- Git tags drive everything; `release.yml` runs goreleaser on tag push.
- `internal/version` populated via `-ldflags "-X .../version.Version=$VERSION -X .../version.Commit=$SHA"`.
- `omc --version` prints version + commit + build date.
- `omc doctor` (later) checks installed version against latest GitHub release and warns if behind.

---

## Things you should think about (open questions / risks)

1. **Template versioning.** When templates evolve, re-running on an existing project will diff against an older render. v1 stamps each generated file with `# omc-template: typescript v0.3.0` so a future `omc update` can detect drift. Plan for this *now* even though `update` ships later.
2. **Conflict UX is the make-or-break.** Users will run `omc` in repos with existing `.claude/` setups. The `merge | overwrite | skip | backup` flow needs to be obvious and forgiving. Prototype this screen first.
3. **Monorepos.** A single repo can be Go service + TS frontend + Python ML scripts. Decide: do we generate one root `CLAUDE.md` covering all, or per-package `CLAUDE.md` files? v1 recommendation: root-level only, but detection surfaces per-directory hints in `CLAUDE.md`.
4. **Detection ambiguity.** A `package.json` with `"react"` and a `pages/` directory is *probably* Next.js but might be Remix-on-Vite. Always show evidence and let user override.
5. **Non-TTY environments.** CI / piped stdout. Detect with `isatty`; require `--yes --profile X` and skip TUI.
6. **Windows.** Bubble Tea works on Windows Terminal but path handling, line endings, and hook shell scripts need attention. Decide: do we ship Windows-friendly hooks (`.cmd`) or punt?
7. **Hooks are executable surface.** Anything we install under `.claude/hooks/` runs on the user's machine. Keep them tiny, auditable, and *opt-in by default* — never preselected in `minimal` profile.
8. **Telemetry.** Off by default. If ever added, opt-in only, anonymous, documented.
9. **Accessibility.** Respect `NO_COLOR`, `CLICOLOR`, `TERM=dumb`. Provide `--no-tui` for screen readers and CI.
10. **Testing strategy** (build this in from day one):
    - Unit tests per detector with `testdata/fixtures/<stack>/` synthetic project trees.
    - Golden-file tests for every template render.
    - E2E: `omc init --profile recommended --yes` in each fixture, snapshot the resulting tree.
    - TUI snapshot tests via `teatest` (Charm's official harness).
11. **Schema drift with Claude Code itself.** `settings.json` schema and hook event names evolve. Pin a Claude Code version in `README.md`, and have `omc doctor` warn if the installed `claude` CLI is older/newer than the one templates target.
12. **License of templates.** Templates ship inside the binary — pick a permissive license (MIT) and state it; users will copy/modify freely.

---

## Skills for building omc

Meta-twist: **this repo eats its own dog food**. Before writing any production code, M1 lays down a top-level `CLAUDE.md` and a set of `.claude/skills/*` so anyone (or Claude Code itself) working in this repo gets just-in-time guidance for the Go/Bubble Tea/Cobra/goreleaser stack. These dev-time aids are *not* shipped in the `omc` binary — they're for contributors. The product templates under `internal/templates/<stack>/` are a separate concern.

### `CLAUDE.md` (repo root)

Small, ~50 lines. Acts as a map, not a manual:
- One paragraph: what `omc` is + link to `PLAN.md` for design.
- Repo layout cheat-sheet (`cmd/omc`, `internal/...`, `testdata/...`).
- Common commands (`make lint`, `make test`, `make e2e`, `make snapshot`, `make release-snapshot`).
- "Where to add things" pointers: new detector → `internal/detect/`, new stack template → `internal/templates/<stack>/ + manifest.yaml`, new TUI screen → `internal/tui/`.
- Note: "See `.claude/skills/` for stack-specific patterns; they auto-load when relevant."

### Skills (`.claude/skills/<name>/SKILL.md`)

Each is a focused, progressive-disclosure doc with YAML frontmatter (`name`, `description` — third-person, includes *what + when*). Body capped at ~500 lines per skill. Six skills for v1:

1. **`bubble-tea-tui`** — Elm/MVU pattern as applied here: model tree with root-as-router, never block in `Update`/`View` (offload to `tea.Cmd`), store `WindowSizeMsg` dims and propagate to children, `lipgloss.Height/Width` for dynamic layout, `tea.Sequence` for ordered work, when to use pointer vs value receivers, Bubble Tea v2 specifics. Triggers when editing `internal/tui/*` or designing screens.

2. **`go-project-layout`** — Why this repo uses `cmd/<binary>` + `internal/`, when (not) to introduce `pkg/`, package naming and one-concept-per-package rule, internal boundary enforcement. Triggers when adding new packages or moving code between layers.

3. **`cobra-command-design`** — `RunE` not `Run`; persistent flags only for truly global concerns (`--dry-run`, `--no-tui`, `--yes`, `--profile`, `--config`); validate args/flags before doing work; populate `Example:` with copy-pasteable lines; auto-generated shell completions for `bash|zsh|fish|powershell`. Triggers when editing `cmd/omc/*` or adding subcommands.

4. **`go-testing`** — Table-driven test idiom, `testdata/` convention, golden files with a `-update` flag, `testing/fstest.MapFS` for synthetic project fixtures, `t.TempDir()` for write-path tests, `go test -fuzz` for `settings_merge.go`, `teatest` for TUI frame snapshots. Triggers when writing `*_test.go` files or designing test fixtures.

5. **`goreleaser-supply-chain`** — `.goreleaser.yaml` shape (builds matrix, archives, brews, scoops, sboms, signs), cosign v3 keyless signing with `--bundle`, SBOM via `syft`, GitHub Actions release workflow with `id-token: write` + `contents: write`, snapshot mode for CI. Triggers when editing `.goreleaser.yaml` or release/CI workflows.

6. **`claude-code-config-authoring`** — The schema of what `omc` *generates*: `settings.json` shape (`permissions.allow`/`deny` arrays of Bash patterns, `hooks` keyed by event name, `env`, `model`), hook event names (`PreToolUse`, `PostToolUse`, `Notification`, `Stop`, etc.), slash command file format (`.claude/commands/<name>.md` with frontmatter), subagent file format (`.claude/agents/<name>.md`), `.mcp.json` shape. Triggers when editing `internal/templates/*` or designing template output. **This is the highest-leverage skill** — getting the generated schema wrong breaks every install.

### Workflow (per user instruction)

1. M1 first creates `CLAUDE.md` and the six skills above.
2. User reloads Claude Code so the skills are picked up.
3. *Then* M1 implementation proper begins (Cobra skeleton, Bubble Tea welcome screen, embed scaffold, CI, goreleaser snapshot).

### What we are *not* adding to `.claude/` (yet)

- No hooks in `.claude/settings.json` for v1 — the contributor experience shouldn't enforce side-effects.
- No custom slash commands beyond what skills provide.
- No `.mcp.json` — revisit if a Postgres/SQLite-backed feature lands.
- No subagents — Claude Code's defaults plus the skills above are sufficient for early development.

---

## CI / testing pipeline (for this repo)

A dedicated GitHub Actions setup, in place from M1 so every PR is gated.

### Workflows

- **`.github/workflows/ci.yml`** — runs on every PR and push to `main`:
  - `lint` job — `golangci-lint run` (with a checked-in `.golangci.yml`: `gofmt`, `govet`, `staticcheck`, `errcheck`, `gosec`, `revive`).
  - `test` job — matrix over `{ubuntu-latest, macos-latest, windows-latest}` × `{go: 'stable'}` (+ `oldstable` to catch drift). Runs `go test -race -coverprofile=coverage.out ./...` and uploads coverage to Codecov.
  - `build` job — `go build ./...` on the same matrix to catch platform-specific compile errors (Bubble Tea + Windows is the usual landmine).
  - `goreleaser-check` job — `goreleaser release --snapshot --clean --skip=publish` on PRs to verify the release config still works without actually publishing.
- **`.github/workflows/release.yml`** — triggers on `v*` tags, runs `goreleaser release` with `GH_TOKEN` + `HOMEBREW_TAP_TOKEN`. Publishes binaries, updates Homebrew tap, generates checksums.
- **`.github/workflows/codeql.yml`** — weekly + on PR for security scanning (free, low-effort).
- **`.github/dependabot.yml`** — weekly Go module updates and GitHub Actions version bumps.

### Test layers

1. **Unit tests** — pure Go logic: `internal/detect/*_test.go`, `internal/apply/settings_merge_test.go` (deep-merge edge cases), `internal/profile/*_test.go`.
2. **Fixture tests for detectors** — `testdata/fixtures/<stack>/` synthetic project trees driven by table tests; assert expected `Signal` set with confidence ≥ threshold.
3. **Golden-file tests for templates** — render every template against representative signal sets, diff against `testdata/golden/`. `go test -update` regenerates. Catches accidental template churn.
4. **End-to-end CLI tests** — spawn `omc init --no-tui --yes --profile recommended` against each fixture project in a temp dir, snapshot the resulting tree (file paths + content hashes). This is the highest-value safety net.
5. **TUI snapshot tests** — `teatest` (Charm's harness) drives the Bubble Tea program with scripted key events; assert on rendered frames. Skipped on Windows runners initially (teatest stability) but run on linux/macOS.
6. **`settings.json` merge fuzz** — `go test -fuzz` against `apply/settings_merge.go` to harden the deep-merge logic against malformed user files.

### Local dev parity

- `Makefile` targets that mirror CI exactly: `make lint`, `make test`, `make e2e`, `make snapshot` (regenerate goldens), `make release-snapshot`.
- **`pre-commit`** config (`.pre-commit-config.yaml`) running `gofmt`, `go vet`, `golangci-lint`, and `go mod tidy` check. Optional but recommended — install instructions in `CONTRIBUTING.md`.
- `act` documented in `CONTRIBUTING.md` for running GH Actions locally.

### Branch protection / required checks

On `main`: require `lint`, `test (ubuntu-latest)`, `test (macos-latest)`, `test (windows-latest)`, `build`, `goreleaser-check` — all green before merge. Linear history, squash merges only.

### Caching & speed

- `actions/setup-go@v5` with `cache: true` (modules + build cache).
- Restrict TUI snapshot tests to linux to keep matrix fast; full matrix runs unit + e2e.
- Target end-to-end CI < 5 minutes for the median PR.

### Coverage & quality bars

- Track coverage in Codecov, **non-blocking** initially (don't gate PRs on coverage % while the project is small).
- Set a `informational` codecov threshold so regressions show up in PR comments without failing CI.
- Once stable, raise to a hard floor (e.g. 70% on `internal/detect` and `internal/apply`).

### What CI does *not* do (intentionally)

- No real `claude` CLI invocations in CI — we only verify generated files, not Claude Code's runtime behavior. That's manual smoke-testing territory (see Verification below).
- No publishing of templates as a separate artifact — they ship inside the binary; if templates change, e2e snapshots catch it.

---

## Phased delivery

We ship the **install path before the product**: M2 cuts a real `v0.1.0` (welcome screen + `omc doctor` + nothing else useful), so by the time detection / TUI / apply / templates land, every user already has a working `omc` binary on disk and is one `go install …@latest` (or `brew upgrade`) away from each new milestone.

| Milestone | Deliverable |
|---|---|
| **M1 — Skeleton** | `cmd/omc/main.go` (Cobra w/ `RunE`), `--version`, basic Bubble Tea v2 welcome screen, `internal/templates/embed.go` scaffold, goreleaser snapshot config, CI (lint+test), **plus repo-level Claude Code dev context**: top-level `CLAUDE.md` + `.claude/skills/*` (see "Dev-time Claude Code context" section below). |
| **M2 — Local install + ship v0.1.0** | `omc doctor` (read-only diagnostic), `internal/version` falls back to `runtime/debug.ReadBuildInfo()` so `go install` users get a sensible version string, migrate `.goreleaser.yaml` from `brews:` → `homebrew_casks:`, `release.yml` workflow (tag-driven, OIDC + cosign + syft), binstaller-generated `install.sh` checked into the repo, README rewritten with **install instructions as the headline section** (`go install` first, then `install.sh`, then GitHub Releases binaries, then brew/scoop), `v0.1.0` tag and release. The product does almost nothing yet, but it's installable, signed, and upgradeable. |
| **M3 — Detection** | `internal/detect/` with TS/Python/Go detectors + monorepo + CI + Docker. Fixture tests. `omc stacks` command. Cut `v0.2.0`. |
| **M4 — TUI core** | Scan, profile, components, preview, apply screens wired up. `--no-tui` headless path. Cut `v0.3.0`. |
| **M5 — Apply pipeline** | `apply/writer.go` with backup + dry-run, `apply/differ.go`, `apply/settings_merge.go` with deep-merge logic + tests. Cut `v0.4.0`. |
| **M6 — Templates** | Fully fleshed templates for the 3 launch stacks, `manifest.yaml` per template, golden-file tests. Cut `v1.0.0` once the surface feels stable. |

---

## Critical files to create (M1 starting set)

- `cmd/omc/main.go` — Cobra root + `init`, `doctor`, `stacks` subcommands.
- `internal/version/version.go` — version vars wired via `-ldflags`.
- `internal/detect/detector.go` — `Detector` interface, registry, `Run(fs.FS) []Signal`.
- `internal/templates/embed.go` — `//go:embed all:templates` and an accessor.
- `internal/profile/profile.go` — profile → component-id list mapping.
- `internal/component/component.go` — `Component` type + manifest loader.
- `internal/apply/writer.go`, `differ.go`, `settings_merge.go`.
- `internal/tui/app.go` — top-level Bubble Tea program; one file per screen.
- `internal/tui/theme.go` — Lip Gloss styles, `NO_COLOR` handling.
- `.goreleaser.yaml`, `Makefile`, `go.mod`, `.github/workflows/ci.yml`.

## Existing utilities worth reusing

- **Bubble Tea v2** (`github.com/charmbracelet/bubbletea/v2`) — TUI runtime. Pin v2 — v1 still gets cited online but v2 is current.
- **Bubbles v2** (`github.com/charmbracelet/bubbles/v2`) — list, viewport, spinner, textinput, table.
- **Lip Gloss** (`github.com/charmbracelet/lipgloss`) — styles, height/width helpers for dynamic layout.
- **Cobra** (`github.com/spf13/cobra`) — CLI scaffolding. Use `RunE` (not `Run`) for proper error returns; auto-gen shell completions.
- **go-diff** (`github.com/sergi/go-diff/diffmatchpatch`) — unified diffs for preview pane.
- **doublestar** (`github.com/bmatcuk/doublestar/v4`) — glob matching for detector evidence.
- **teatest** (`github.com/charmbracelet/x/exp/teatest`) — TUI snapshot tests (note: `x/exp` so API is unstable; pin and audit on bumps).
- **`testing/fstest.MapFS`** (stdlib) — synthetic project trees for detector tests; no temp-file overhead.
- **goreleaser v2** + **cosign v3** — release pipeline. Use cosign's `--bundle` mode (single `.sigstore.json`) and keyless OIDC signing via GitHub Actions (`id-token: write`); SBOM via `syft`.

(All standard, no need to reinvent any of the above.)

---

## Verification (how to know it works)

- `go test ./...` green, including detector fixtures and golden templates.
- `omc init --dry-run --profile recommended --no-tui` in each `testdata/fixtures/<stack>/` produces the expected file tree (snapshot-tested).
- Manual smoke test: clone a real public TS, Python, and Go repo each, run `omc init`, open a Claude Code session in that repo, confirm: (a) detected commands work via `/test`, (b) permission allowlist removes common prompts, (c) hooks fire as expected, (d) `CLAUDE.md` reads sensibly.
- TUI manually verified on macOS Terminal, iTerm2, Linux (WSL), and Windows Terminal.
- `omc doctor` flags a project that has `.claude/settings.local.json` un-gitignored.
