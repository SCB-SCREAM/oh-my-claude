# omc — contributor guide

`omc` (`oh-my-claude`) is a Go + Bubble Tea TUI CLI that detects a project's stack and bootstraps a tailored Claude Code setup (`CLAUDE.md`, `.claude/settings.json`, hooks, slash commands, subagent stubs, MCP suggestions). See [`PLAN.md`](PLAN.md) for the full design.

## Repo layout

```
cmd/omc/             entry point — Cobra root + subcommands
internal/
  detect/            file-only stack detection (Detector interface, registry)
  templates/         embedded per-stack templates (//go:embed)
  profile/           profile → component lists (minimal | recommended | full)
  component/         Component type + manifest loader
  apply/             writer, differ, settings.json deep-merge
  tui/               Bubble Tea v2 screens (welcome, scan, profile, components, preview, apply)
  version/           build-stamped version (-ldflags)
testdata/fixtures/   synthetic project trees for detector tests
testdata/golden/     expected outputs for golden-file tests
.goreleaser.yaml     release config (multi-platform, cosign-signed, SBOM)
```

## Common commands

- `make lint` — `golangci-lint run`
- `make test` — `go test -race ./...`
- `make e2e` — runs `omc init --no-tui --yes --profile recommended` against each fixture, snapshots resulting tree
- `make snapshot` — regenerate golden files (`go test -update ./...`)
- `make release-snapshot` — `goreleaser release --snapshot --clean --skip=publish`

## Where to add things

- **New language/framework detector** → `internal/detect/<name>.go` + fixture under `testdata/fixtures/<name>/`
- **New stack template** → `internal/templates/<stack>/` (`CLAUDE.md.tmpl`, `settings.json.tmpl`, `hooks/`, `commands/`, `agents/`, `mcp.json`) + sibling `manifest.yaml` declaring `AppliesTo`
- **New TUI screen** → `internal/tui/<screen>.go` + wire into the root model in `internal/tui/app.go`
- **New subcommand** → `cmd/omc/<name>.go`; register on root in `main.go`

## Conventions

- TUI uses **Bubble Tea v2** (`github.com/charmbracelet/bubbletea/v2`) — v1 patterns from older blog posts won't compile.
- Cobra commands use **`RunE`** (not `Run`) so errors propagate cleanly.
- Detection is **file-only** — never execute user code or shell out to package managers during detection.
- Generated files include a stamp comment (e.g. `# omc-template: typescript v0.x.y`) so future `omc update` can detect drift.
- Tests prefer `testing/fstest.MapFS` over `t.TempDir()` for read-only fixtures (faster, hermetic).

## Skills

`.claude/skills/` carries focused, just-in-time guidance per area:

- `bubble-tea-tui` — Elm/MVU patterns, model tree, message routing, layout
- `go-project-layout` — `cmd/`/`internal/` rules, when (not) to add `pkg/`
- `cobra-command-design` — flag/subcommand patterns, `RunE`, completions
- `go-testing` — table tests, golden files, `fstest.MapFS`, `teatest`
- `goreleaser-supply-chain` — releases, cosign v3, SBOM, GH Actions
- `claude-code-config-authoring` — schema for what `omc` *generates*

Skills auto-load when relevant — read them before starting non-trivial work in their domain.
