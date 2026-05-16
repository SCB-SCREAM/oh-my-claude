package detect

import "time"

// Type is the closed-enum classification an LLM-backed detector assigns to
// a project. Borrowed from Backstage's component.type vocabulary, narrowed
// to the categories that meaningfully shape a Claude Code setup.
//
// Constraining detection output to a fixed enum has two benefits:
//   - The LLM cannot invent unstable category strings ("nextjs-app",
//     "react-frontend") that would fragment downstream component matching.
//   - Components gate on Type with a small, exhaustive switch — adding a
//     new Type is a deliberate code change, not a silent surprise.
type Type string

const (
	TypeWebApp   Type = "webapp"
	TypeAPI      Type = "api"
	TypeCLI      Type = "cli"
	TypeLibrary  Type = "library"
	TypeInfra    Type = "infra"
	TypeMonorepo Type = "monorepo"
)

// AllTypes returns every Type in its canonical display order. Used by
// cmd/omc/stacks to enumerate supported overlays and by the JSON schema
// builder to constrain the LLM response.
func AllTypes() []Type {
	return []Type{TypeWebApp, TypeAPI, TypeCLI, TypeLibrary, TypeInfra, TypeMonorepo}
}

// Valid reports whether t is one of the canonical types. Used in
// post-parse validation of LLM output.
func (t Type) Valid() bool {
	for _, k := range AllTypes() {
		if k == t {
			return true
		}
	}
	return false
}

// Stack is the structured project description an LLM-backed detector
// returns. Every field is optional except Type. Empty strings mean "not
// applicable" or "could not determine" — components gate on field
// presence (e.g. format-on-save fires iff Formatter != "").
//
// Field design philosophy: prefer commands the user would actually type
// ("go test -race ./...") over abstract identifiers ("go-test"). The
// resulting CLAUDE.md should feel hand-written, and a command string
// drops straight into a /test slash-command template.
type Stack struct {
	Type Type `json:"type"`

	// LanguagePrimary is the dominant language ("go", "typescript",
	// "rust") — lowercase, no version. Optional.
	LanguagePrimary string `json:"language_primary,omitempty"`

	// PackageManager identifies the dependency tooling
	// ("pnpm", "uv", "go-modules", "cargo", "bundler"). Optional.
	PackageManager string `json:"package_manager,omitempty"`

	// BuildCmd is the canonical build invocation
	// ("go build ./...", "pnpm build", "cargo build --release").
	// Empty means the stack has no build step.
	BuildCmd string `json:"build_cmd,omitempty"`

	// TestCmd is the canonical test invocation.
	TestCmd string `json:"test_cmd,omitempty"`

	// LintCmd is the canonical lint invocation.
	LintCmd string `json:"lint_cmd,omitempty"`

	// Formatter is the canonical formatter invocation
	// ("gofmt -w .", "prettier --write .", "ruff format ."). The
	// format-on-save hook component gates on this being non-empty.
	Formatter string `json:"formatter,omitempty"`

	// Frameworks lists detected framework names ("next.js", "django",
	// "axum"). Order is by salience — most-defining first. Used to
	// flesh out the CLAUDE.md narrative but not for component gating.
	Frameworks []string `json:"frameworks,omitempty"`

	// Notes is a one-paragraph human summary the LLM may emit for the
	// CLAUDE.md preamble. Bounded to 500 chars by post-parse validation.
	Notes string `json:"notes,omitempty"`
}

// Result wraps a successful detection. The cache layer adds FromCache /
// Refreshed metadata so the headless and TUI paths can surface
// "(cached)" / "(re-detected: package.json changed)" to the user.
type Result struct {
	Stack    Stack
	Snapshot *Snapshot

	// FromCache is true if Stack was loaded from the project-local
	// cache file without calling the LLM.
	FromCache bool

	// Refreshed is true when a stale cache was found, invalidated, and
	// the LLM was re-invoked. False on first-ever runs (no cache to
	// refresh) and on cache hits.
	Refreshed bool

	// Changed, when Refreshed is true, lists the repo-relative paths
	// whose contents changed since the cached run. Used by Verbose
	// logging to explain WHY we re-ran. Empty on cache hits and on
	// first-ever runs.
	Changed []string
}

// Options bundles configuration for one detection run.
type Options struct {
	// Runner is the subprocess seam. nil → the default `claude`-exec
	// runner. Override in tests with a fake.
	Runner Runner

	// Timeout bounds a single subprocess invocation. Zero → 90s.
	Timeout time.Duration

	// BudgetUSD is forwarded to claude's --max-budget-usd. Zero → 0.10.
	BudgetUSD float64

	// CachePath is the project-local cache file. Zero → no cache (used
	// by tests that want to drive the subprocess every time). The
	// canonical value is filepath.Join(repoRoot, ".claude/omc/detected.json")
	// and is supplied by the session layer.
	CachePath string

	// SkipCache disables reading the cache (still writes on success).
	// Wired to `omc detect --refresh` / `omc init --refresh-detection`.
	SkipCache bool

	// Verbose, if non-nil, receives one human-readable line per
	// significant event: cache hit, cache invalidation (with delta),
	// degradation, success. Plumbed up to stdout in the headless path
	// and to the scan-screen log pane in the TUI.
	Verbose func(format string, args ...any)
}
