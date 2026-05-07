<div align="center">

```
 ██████╗ ██╗  ██╗    ███╗   ███╗██╗   ██╗     ██████╗██╗      █████╗ ██╗   ██╗██████╗ ███████╗
██╔═══██╗██║  ██║    ████╗ ████║╚██╗ ██╔╝    ██╔════╝██║     ██╔══██╗██║   ██║██╔══██╗██╔════╝
██║   ██║███████║    ██╔████╔██║ ╚████╔╝     ██║     ██║     ███████║██║   ██║██║  ██║█████╗
██║   ██║██╔══██║    ██║╚██╔╝██║  ╚██╔╝      ██║     ██║     ██╔══██║██║   ██║██║  ██║██╔══╝
╚██████╔╝██║  ██║    ██║ ╚═╝ ██║   ██║       ╚██████╗███████╗██║  ██║╚██████╔╝██████╔╝███████╗
 ╚═════╝ ╚═╝  ╚═╝    ╚═╝     ╚═╝   ╚═╝        ╚═════╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝
```

### Bootstrap a tailored Claude Code setup in seconds — opinionated by stack.

[![CI](https://github.com/SCB-SCREAM/oh-my-claude/actions/workflows/ci.yml/badge.svg)](https://github.com/SCB-SCREAM/oh-my-claude/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/SCB-SCREAM/oh-my-claude?sort=semver)](https://github.com/SCB-SCREAM/oh-my-claude/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/SCB-SCREAM/oh-my-claude.svg)](https://pkg.go.dev/github.com/SCB-SCREAM/oh-my-claude)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

`omc` (`oh-my-claude`) detects your project's stack from its files, asks you to pick a profile, and writes a curated Claude Code setup — `CLAUDE.md`, `.claude/settings.json`, hooks, slash commands, subagent stubs, and MCP suggestions — tailored to that stack. Think of it as **oh-my-zsh, but for Claude Code**.

---

## Install

Pick the option that matches your environment. Every path lands the same signed binary; cosign + SHA256 verification is automatic on `install.sh`.

<table>
<tr>
<td width="50%" valign="top">

### Go (recommended for now)

```bash
go install github.com/SCB-SCREAM/oh-my-claude/cmd/omc@latest
```

The headline path while we're pre-1.0. If `omc` isn't found after installing, add Go's bin directory to your shell `$PATH`:

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.bashrc   # or ~/.zshrc
exec $SHELL
```

<sub>Requires Go 1.25+.</sub>

</td>
<td width="50%" valign="top">

### One-line script (Linux / macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/SCB-SCREAM/oh-my-claude/main/install.sh | sh
```

Auto-detects OS/arch, fetches the latest release, verifies SHA256, installs to `~/.local/bin`. Override with `OMC_VERSION=v0.1.0` or `OMC_INSTALL_DIR=…`.

<details>
<summary>Security-conscious form</summary>

```bash
curl -fsSL https://raw.githubusercontent.com/SCB-SCREAM/oh-my-claude/main/install.sh -o install.sh
less install.sh
sh install.sh
```

</details>

</td>
</tr>
<tr>
<td width="50%" valign="top">

### Homebrew (macOS / Linux)

```bash
brew install SCB-SCREAM/tap/omc
```

Tap is auto-published from each tagged release. `brew upgrade` keeps you current.

</td>
<td width="50%" valign="top">

### Scoop (Windows)

```powershell
scoop bucket add scb https://github.com/SCB-SCREAM/scoop-bucket
scoop install omc
```

</td>
</tr>
<tr>
<td colspan="2" valign="top">

### Direct download

Grab the right archive from [GitHub Releases](https://github.com/SCB-SCREAM/oh-my-claude/releases) and verify with cosign:

```bash
gh release download v0.1.0 -p '*linux_amd64*' -p 'checksums.txt' -p 'checksums.txt.sigstore.json'

cosign verify-blob \
  --certificate-identity-regexp 'https://github.com/SCB-SCREAM/oh-my-claude/.+' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --bundle checksums.txt.sigstore.json \
  checksums.txt

sha256sum -c --ignore-missing checksums.txt
tar -xzf omc_*_linux_amd64.tar.gz
```

</td>
</tr>
</table>

Verify the install:

```text
$ omc --version
omc v0.1.0 (commit abc1234, built 2026-05-07T09:00:00Z)
```

---

## Quick start

```bash
cd ~/your-project
omc init        # interactive TUI
omc doctor      # read-only diagnostic of the current setup
```

Non-interactive (CI / scripted bootstraps):

```bash
omc init --no-tui --yes --profile recommended
omc init --dry-run --profile minimal      # preview the plan, don't write
```

---

## What `omc` does

- **Detects your stack** from files only — no shelling out to `npm`, `pip`, `go list`, etc. TypeScript/JavaScript, Python, Go are first-class for v1; Rust, Ruby, Java are stretch.
- **Asks for a profile** — `minimal`, `recommended`, or `full`. Every component is a checkbox you can toggle before applying.
- **Generates `CLAUDE.md`** — stack-aware, with project commands (test/build/lint/format), conventions, and known gotchas pre-filled.
- **Generates `.claude/settings.json`** — pre-approved Bash allowlist tuned to the stack (`pnpm test`, `pytest -q`, `go test ./...`), conservative deny list, model defaults left blank.
- **Suggests hooks** — `PostToolUse` formatters (prettier/ruff/gofmt), `PreToolUse` guards for destructive ops, optional `Stop` "did you run tests?" reminders. **Opt-in by default.**
- **Drops `/test`, `/lint`, `/format`, `/typecheck`** — slash command stubs you can extend.
- **Recommends MCP servers** — GitHub MCP if `.github/` present, Postgres MCP if Postgres detected, etc.
- **Is safe and idempotent** — diff preview before apply, `settings.json` deep-merge instead of overwrite, automatic backups under `.claude/.omc-backup-<ts>/`, `--dry-run` shows the plan and exits 0.

---

## Status

We ship the install path before the product, so every later milestone is just `brew upgrade` (or `go install …@latest`) away.

| | Milestone | Status |
|---|---|---|
| **M1** | Skeleton — Cobra CLI, Bubble Tea v2 welcome screen, embed scaffold, CI, dev-time skills | done |
| **M2** | Local install + ship `v0.1.0` — `omc doctor`, install.sh, Homebrew tap, Scoop bucket, cosign-signed releases | in progress |
| **M3** | Detection — TS/Python/Go detectors, monorepo + Docker + CI signals, `omc stacks` | planned |
| **M4** | TUI core — scan / profile / components / preview / apply screens | planned |
| **M5** | Apply pipeline — backup, dry-run, `settings.json` deep-merge, unified diff | planned |
| **M6** | Templates — fully-fleshed launch stacks, golden-file tests, `v1.0.0` | planned |

The full design is in [`PLAN.md`](PLAN.md).

---

## Contributing

This repo is built with Claude Code's help — there's a top-level [`CLAUDE.md`](CLAUDE.md) plus six skills under [`.claude/skills/`](.claude/skills) that any Claude Code session in the repo will auto-load:

- `bubble-tea-tui` — Bubble Tea v2 patterns
- `cobra-command-design` — flag/subcommand conventions
- `go-project-layout` — `cmd/` vs `internal/`, when (not) to add `pkg/`
- `go-testing` — table tests, golden files, `fstest.MapFS`, `teatest`
- `goreleaser-supply-chain` — release pipeline, cosign, SBOM
- `claude-code-config-authoring` — schema of what `omc` *generates*

Local dev parity:

```bash
make build              # ./bin/omc
make test               # go test -race ./...
make lint               # golangci-lint run
make release-snapshot   # goreleaser release --snapshot --clean --skip=publish,sign
```

PRs are gated on lint + test (linux/macos/windows) + build + a goreleaser snapshot check; see [`.github/workflows/ci.yml`](.github/workflows/ci.yml).

---

## License

MIT — see [`LICENSE`](LICENSE). Templates ship inside the binary; copy and modify freely.
