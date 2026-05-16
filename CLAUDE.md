# omc — contributor guide

`omc` (`oh-my-claude`) is a Go + Bubble Tea TUI CLI that classifies a project via the local `claude` CLI and bootstraps a tailored Claude Code setup (`CLAUDE.md`, `.claude/settings.json`, hooks, slash commands, subagent stubs, MCP suggestions). See [`PLAN.md`](PLAN.md) for the full design.

## Architecture in one paragraph

Detection is LLM-backed and stack-agnostic: omc walks the project, asks `claude` to classify it into one of a fixed `Type` enum (`webapp | api | cli | library | infra | monorepo`) and to fill a structured `Stack` (build_cmd, test_cmd, lint_cmd, formatter, frameworks, …). Results are cached project-locally at `.claude/omc/detected.json` with per-file SHA fingerprints — repeat runs on an unchanged tree pay zero LLM cost. The component layer gates on `Stack` fields, never on language names, so adding a new language is zero work in omc itself: the LLM just emits a different `language_primary` and the same `cmd.test` / `cmd.lint` / `cmd.format` / `hook.format-on-save` components apply.

## Repo layout

```
cmd/omc/             entry point — Cobra root + subcommands
internal/
  detect/            LLM-backed Stack classifier + project-local cache
                       types.go    — Type enum, Stack struct, Result, Options
                       stack.go    — RunStack: prompt + JSON-schema + post-parse validator
                       cache.go    — RunCached: per-file SHA delta detection
                       runner.go   — `claude` subprocess seam (neutral cwd, no --bare)
                       snapshot.go — FS walk producing the file listing
  templates/         embedded per-Type templates (//go:embed) — M6 fills in
  profile/           profile → component-ID table (minimal | recommended | full)
  component/         Component type + in-Go registry; AppliesTo(Stack) bool
                       shared.go   — baseline + format-on-save hook (gates on Stack.Formatter)
                       commands.go — /test, /lint, /format, /build (gate on Stack.*Cmd)
  apply/             writer, differ, settings.json deep-merge (M4 stub; real pipeline lands in M5)
  session/           shared pipeline: detect → resolve → buildplan → execute
  report/            Stack-rendering helpers shared by TUI + headless
  tui/               Bubble Tea v2 screens (welcome, scan, profile, components, preview, apply, done)
  doctor/            read-only environment checks (claude present + authed, .gitignore hygiene)
  version/           build-stamped version (-ldflags)
testdata/golden/     expected outputs for golden-file tests
.goreleaser.yaml     release config (multi-platform, cosign-signed, SBOM)
```

## Common commands

- `make lint` — `golangci-lint run`
- `make test` — `go test -race ./...`
- `make e2e` — `go test -tags=e2e -race ./cmd/omc/` (uses a fake `claude` shim + a pre-seeded cache so tests are hermetic)
- `make snapshot` — regenerate golden files (`go test -update ./...`)
- `make release-snapshot` — `goreleaser release --snapshot --clean --skip=publish`

## Where to add things

- **New project Type** → extend the `detect.Type` enum in `internal/detect/types.go`, update the `enum` in `stackSchema` (stack.go), update `AllTypes()`, and add a one-line blurb in `cmd/omc/stacks.go`. Then add components in `internal/component/` that gate on the new Type.
- **New Stack-gated component** → register it in `internal/component/<name>.go` (or extend `shared.go`/`commands.go`). Always gate via `AppliesTo: func(s detect.Stack) bool { … }` on Stack fields — never on language name. New languages get the component for free.
- **New stack template** → `internal/templates/types/<type>/` (`CLAUDE.md.tmpl`, `settings.json.tmpl`, `hooks/`, `commands/`, `agents/`, `mcp.json`) (M6).
- **New TUI screen** → `internal/tui/<screen>.go` + add a `screenID` const in `messages.go` + add a field to `appModel` + extend the routing switch in `internal/tui/app.go`. Emit `switchTo(<screen>)` from the previous screen and wire any cross-screen `tea.Cmd` in `appModel.handleScreenSwitch`. See the `bubble-tea-tui` skill.
- **New shared pipeline step** → add to `internal/session/` so the headless and TUI paths consume the same code.
- **New subcommand** → `cmd/omc/<name>.go`; register on root in `main.go`.
- **New doctor check** → add a `func checkX(root string) Finding` to `internal/doctor/doctor.go` and append it to the slice in `Run`.

## Conventions

- TUI uses **Bubble Tea v2** (`charm.land/bubbletea/v2` + `charm.land/bubbles/v2`) — v1 patterns from older blog posts won't compile. The canonical module path is `charm.land/...`, NOT `github.com/charmbracelet/...` for the v2 packages.
- **TUI tests** use direct `Update`/`View` calls for per-screen behavior; `github.com/charmbracelet/x/exp/teatest/v2` for whole-program / quit-on-q smoke. Use `m.View().Content` (the `Content` field) to assert against the rendered string.
- Cobra commands use **`RunE`** (not `Run`) so errors propagate cleanly.
- **Detection never executes the target project's code or its package managers.** The only subprocess invoked is the local `claude` CLI — treated as an external service: capped budget, schema-enforced output (`--json-schema`), neutral cwd, `--tools ""`. No project files are read by the subprocess itself.
- **claude is a hard runtime dependency.** `omc init` preflights `claude --version` and `claude auth status` and refuses to proceed without both. omc never falls back to `ANTHROPIC_API_KEY` billing — it uses the user's subscription session via `--no-session-persistence` (no `--bare`).
- **Components gate on `Stack` fields, never on language names.** `func(s detect.Stack) bool { return s.Formatter != "" }`, not `hasSignal(signals, "go")`. The `Type` switch (`s.Type == detect.TypeCLI`) is fine when the gate is genuinely Type-specific.
- Generated files include a stamp comment (e.g. `# omc-template: webapp v0.x.y`) so future `omc update` can detect drift.
- Tests prefer `testing/fstest.MapFS` over `t.TempDir()` for read-only fixtures (faster, hermetic). Tests that need to drive detection inject a fake `detect.Runner` via `session.Options.Detect.Runner`.

## Skills

`.claude/skills/` carries focused, just-in-time guidance per area:

- `bubble-tea-tui` — Elm/MVU patterns, model tree, message routing, layout
- `go-project-layout` — `cmd/`/`internal/` rules, when (not) to add `pkg/`
- `cobra-command-design` — flag/subcommand patterns, `RunE`, completions
- `go-testing` — table tests, golden files, `fstest.MapFS`, `teatest`
- `goreleaser-supply-chain` — releases, cosign v3, SBOM, GH Actions
- `claude-code-config-authoring` — schema for what `omc` *generates*
- `claude-subprocess` — invoking the local `claude` CLI from omc (the canonical safe-invocation flag set, the verified JSON output schema, subscription-only auth, cost discipline, prompt-injection mitigations)

Skills auto-load when relevant — read them before starting non-trivial work in their domain.
