---
name: goreleaser-supply-chain
description: How releases work in this repo — .goreleaser.yaml shape (builds, archives, brews, scoops, sboms, signs), cosign v3 keyless signing, syft SBOM generation, and the GitHub Actions release workflow. Use this when editing .goreleaser.yaml, the release or CI workflows, signing config, snapshot/dry-run mechanics, or anything touching multi-platform binary publishing. Apply also when bumping goreleaser, cosign, or syft versions — they have breaking changes worth being deliberate about.
---

# Releases & supply-chain integrity

Releases are tag-driven. `git tag -a v0.1.0 -m '…' && git push --tags` triggers `.github/workflows/release.yml`, which runs goreleaser. Goreleaser builds binaries for every supported platform, signs them with cosign (keyless via sigstore), generates SBOMs with syft, publishes a GitHub Release, and updates the Homebrew tap and Scoop bucket.

## `.goreleaser.yaml` skeleton

```yaml
version: 2  # goreleaser v2 schema

before:
  hooks:
    - go mod tidy
    - go test ./...

builds:
  - id: omc
    main: ./cmd/omc
    binary: omc
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X github.com/<owner>/oh-my-claude/internal/version.Version={{.Version}}
      - -X github.com/<owner>/oh-my-claude/internal/version.Commit={{.Commit}}
      - -X github.com/<owner>/oh-my-claude/internal/version.Date={{.Date}}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ignore:
      - { goos: windows, goarch: arm64 }   # drop or keep depending on demand

archives:
  - id: omc
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip
    files:
      - LICENSE
      - README.md

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

sboms:
  - id: bin-sboms
    artifacts: binary
    documents:
      - "{{ .ArtifactName }}.spdx.json"

signs:
  - id: cosign-blob
    cmd: cosign
    artifacts: checksum
    signature: "${artifact}.sig"
    certificate: "${artifact}.pem"
    args:
      - sign-blob
      - "--bundle=${artifact}.sigstore.json"
      - "--yes"
      - "${artifact}"
    output: true

brews:
  - repository:
      owner: <owner>
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    homepage: "https://github.com/<owner>/oh-my-claude"
    description: "Bootstrap Claude Code for your project, opinionated by stack"
    license: MIT
    install: |
      bin.install "omc"
    test: |
      system "#{bin}/omc", "--version"

scoops:
  - repository:
      owner: <owner>
      name: scoop-bucket
      token: "{{ .Env.SCOOP_BUCKET_TOKEN }}"
    homepage: "https://github.com/<owner>/oh-my-claude"
    description: "Bootstrap Claude Code for your project, opinionated by stack"
    license: MIT

release:
  draft: false
  prerelease: auto

snapshot:
  name_template: "{{ incpatch .Version }}-snapshot+{{ .ShortCommit }}"

changelog:
  use: github-native
```

Anchors that matter:

- **`version: 2`** — the v2 schema. Don't accidentally check in `version: 1` configs from old gists.
- **`-trimpath`** + `-s -w` — reproducible-ish builds without local paths and without DWARF.
- **`ldflags`** inject version/commit/date into `internal/version`.
- **`signs.cmd: cosign`** with `--bundle` is the v3 idiom: emits a single `.sigstore.json` per artifact instead of separate `.sig` + `.pem`.
- **`sboms`** ships `.spdx.json` SBOMs alongside binaries.
- **`brews` / `scoops`** auto-PR/auto-push to a sibling tap/bucket repo. Tokens come from secrets (PAT scoped to those repos).

## GitHub Actions: release workflow

`.github/workflows/release.yml`:

```yaml
name: release
on:
  push:
    tags: ["v*"]

permissions:
  contents: write    # create the GitHub Release
  id-token: write    # cosign keyless signing via Fulcio
  packages: write    # only if we add OCI artifacts later

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with:
          go-version: stable
          cache: true
      - uses: sigstore/cosign-installer@v3
      - uses: anchore/sbom-action/download-syft@v0
      - uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
          SCOOP_BUCKET_TOKEN: ${{ secrets.SCOOP_BUCKET_TOKEN }}
```

The two non-obvious bits:

- **`id-token: write`** is the OIDC token that cosign exchanges with Fulcio for a short-lived signing certificate. Without it, cosign keyless will fail.
- **`fetch-depth: 0`** because goreleaser reads the full git history for changelog generation.

## CI: snapshot check on PRs

`.github/workflows/ci.yml` includes a `goreleaser-check` job that runs goreleaser in snapshot mode. It validates the config compiles and the build matrix succeeds, *without* publishing. Run locally with `make release-snapshot`.

```yaml
goreleaser-check:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
      with: { fetch-depth: 0 }
    - uses: actions/setup-go@v5
      with: { go-version: stable, cache: true }
    - uses: goreleaser/goreleaser-action@v6
      with:
        version: latest
        args: release --snapshot --clean --skip=publish,sign
```

Note `--skip=sign` in CI: the snapshot job doesn't have OIDC tokens; we skip signing in PRs and verify it instead in the real release job.

## Verification (what users do)

Document in the README so paranoid users have a clear path:

```bash
# Download the artifact + its sigstore bundle + checksums.
gh release download v0.1.0 -p '*linux_amd64*' -p 'checksums.txt' -p 'checksums.txt.sigstore.json'

# Verify the checksums file's signature.
cosign verify-blob \
  --certificate-identity-regexp 'https://github.com/<owner>/oh-my-claude/.+' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --bundle checksums.txt.sigstore.json \
  checksums.txt

# Verify the binary against its checksum.
sha256sum -c --ignore-missing checksums.txt
```

The `--certificate-identity-regexp` pins what identity is allowed to sign for this repo.

## Versioning

- Semver. `v0.x.y` until the CLI surface stabilizes; `v1.0.0` once we promise compatibility.
- One tag = one release. Don't reuse tags. Don't push minor/patch tags before a major.
- **Never** sign or publish out-of-band; everything goes through the release workflow.

## Cosign v2 → v3

If you see legacy config:

- `--bundle ...` (v3) replaces the separate `--certificate ${artifact}.pem` + `--signature ${artifact}.sig` outputs.
- `cosign-installer@v3` ensures cosign v3.
- `--yes` skips the interactive confirmation prompt — required in CI.

## Don't

- Don't add `replace` directives to `go.mod` for releases. Tag-driven releases must build from public modules only.
- Don't set `CGO_ENABLED=1` unless we have a real C dep. Static binaries are the whole point of distributing a Go CLI.
- Don't publish nightlies from `main` until users ask for them. Adds maintenance and token surface.
- Don't store cosign keys in this repo. Keyless via OIDC is the only signing path.
- Don't run goreleaser locally with `release` (no `--snapshot`) — it will try to publish.
