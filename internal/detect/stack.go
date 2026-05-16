package detect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// listingCap bounds how many file paths the LLM prompt includes. Big
// monorepos blow up the prompt cost otherwise; 500 is enough to surface
// every manifest plus a representative slice of source paths.
const listingCap = 500

// buildListing produces a deterministic, bounded prompt input from a
// snapshot's file list. The cap is taken in sorted order (snap.Files is
// already sorted) so the cache key stays stable across runs.
func buildListing(files []string) []string {
	if len(files) <= listingCap {
		return files
	}
	out := make([]string, listingCap)
	copy(out, files[:listingCap])
	return out
}

// claudeResult is the top-level shape of `claude -p --output-format
// json`. When --json-schema is set, the schema-conforming payload is in
// StructuredOutput; Result holds prose or is empty. Unknown fields are
// ignored.
type claudeResult struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	StopReason       string          `json:"stop_reason"`
}

// ── public entry point ──────────────────────────────────────────────────

// RunStack invokes the local `claude` CLI to classify the project and
// return a structured [Stack]. It is the canonical detection entry point
// in the LLM-first design — the rule-based detectors are gone.
//
// Errors are returned (not swallowed) for every failure mode: missing
// claude binary, unauthenticated session, subprocess error, schema
// violation. The caller (session.Detect) decides whether to surface them
// to the user or abort the run.
//
// Cache integration lives in [Cache] (see cache.go). RunStack itself is
// a single uncached invocation — every call exec's claude.
func RunStack(ctx context.Context, snap *Snapshot, opts Options) (*Result, error) {
	if snap == nil || len(snap.Files) == 0 {
		return nil, errors.New("detect: empty project (no files indexed)")
	}
	opts = opts.withDefaults()

	if err := verifyClaudeReady(ctx, opts); err != nil {
		return nil, err
	}

	listing := buildListing(snap.Files)
	raw, err := callClaudeForStack(ctx, opts, listing)
	if err != nil {
		return nil, err
	}

	stack, err := parseStackResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse claude stack response: %w", err)
	}

	opts.Verbose("detected type=%s language=%s", stack.Type, stack.LanguagePrimary)
	return &Result{Stack: stack, Snapshot: snap}, nil
}

// ── auth gate ──────────────────────────────────────────────────────────

// verifyClaudeReady checks that the local `claude` binary is on PATH and
// the user has an active subscription session. Returns a typed error the
// CLI can match against to print install / login hints.
func verifyClaudeReady(ctx context.Context, opts Options) error {
	if _, err := opts.Runner.Run(ctx, []string{"--version"}, nil); err != nil {
		return ErrClaudeMissing
	}
	if _, err := opts.Runner.Run(ctx, []string{"auth", "status"}, nil); err != nil {
		return ErrClaudeUnauthenticated
	}
	return nil
}

// ErrClaudeMissing is returned when the `claude` binary is not on PATH.
// The CLI surfaces this with an install hint.
var ErrClaudeMissing = errors.New("claude CLI not found on PATH (install: https://docs.claude.com/claude-code)")

// ErrClaudeUnauthenticated is returned when claude is installed but the
// user has no active subscription session. The CLI surfaces this with a
// `claude auth login` hint.
var ErrClaudeUnauthenticated = errors.New("claude is not authenticated (run: claude auth login)")

// ── option defaults ────────────────────────────────────────────────────

func (o Options) withDefaults() Options {
	if o.Runner == nil {
		o.Runner = defaultRunner
	}
	if o.Timeout == 0 {
		o.Timeout = 90 * time.Second
	}
	if o.BudgetUSD == 0 {
		// Cold-cache Haiku runs hit ~$0.01-0.03 because the Claude Code
		// system prompt is ~7k tokens and gets billed at cache-creation
		// rate on the first call. Be generous on the cap so first runs
		// don't fail; later runs are free via our project-local cache.
		o.BudgetUSD = 0.10
	}
	if o.Verbose == nil {
		o.Verbose = func(string, ...any) {}
	}
	return o
}

// ── subprocess invocation ──────────────────────────────────────────────

// stackSchema constrains the LLM response. `type` is a closed enum —
// the LLM must classify into one of the six canonical project types.
// String length caps drive Haiku into terse, command-shaped answers
// rather than prose.
const stackSchema = `{
  "type": "object",
  "required": ["type"],
  "additionalProperties": false,
  "properties": {
    "type": {"type": "string", "enum": ["webapp","api","cli","library","infra","monorepo"]},
    "language_primary": {"type": "string", "maxLength": 50},
    "package_manager":  {"type": "string", "maxLength": 50},
    "build_cmd":        {"type": "string", "maxLength": 200},
    "test_cmd":         {"type": "string", "maxLength": 200},
    "lint_cmd":         {"type": "string", "maxLength": 200},
    "formatter":        {"type": "string", "maxLength": 200},
    "frameworks":       {"type": "array",  "maxItems": 8, "items": {"type": "string", "maxLength": 50}},
    "notes":            {"type": "string", "maxLength": 500}
  }
}`

func callClaudeForStack(parent context.Context, opts Options, listing []string) (string, error) {
	prompt := buildStackPrompt(listing)

	// No --bare: --bare forces ANTHROPIC_API_KEY (per-call billing) and
	// breaks the subscription-only guarantee. The runner sets cwd to a
	// neutral directory so claude doesn't auto-load the target project's
	// CLAUDE.md / settings into its inference context.
	args := []string{
		"-p",
		"--output-format", "json",
		"--json-schema", stackSchema,
		"--tools", "",
		"--model", "haiku",
		"--effort", "low",
		"--max-budget-usd", fmt.Sprintf("%.4f", opts.BudgetUSD),
		"--no-session-persistence",
		prompt,
	}

	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()

	stdout, err := opts.Runner.Run(ctx, args, nil)
	if err != nil {
		return "", fmt.Errorf("claude exec: %w", err)
	}

	var result claudeResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		return "", fmt.Errorf("parse claude output: %w", err)
	}
	if result.IsError || result.Subtype != "success" {
		return "", fmt.Errorf("claude reported %s", result.Subtype)
	}
	if len(result.StructuredOutput) > 0 && string(result.StructuredOutput) != "null" {
		return string(result.StructuredOutput), nil
	}
	if result.Result == "" {
		return "", errors.New("claude returned no structured_output and empty result")
	}
	return result.Result, nil
}

func buildStackPrompt(files []string) string {
	var sb strings.Builder
	// "Output ONLY JSON" is load-bearing — without it Haiku tends to
	// return prose in `result` and leaves `structured_output` empty,
	// costing roughly 3x more for nothing parseable.
	sb.WriteString("Output ONLY a JSON object matching the schema. No prose, no markdown.\n\n")
	sb.WriteString("Classify this project from its file paths. Choose `type` from the enum ")
	sb.WriteString("(one of: webapp, api, cli, library, infra, monorepo). ")
	sb.WriteString("Use the file listing as your only source — do not invent files you don't see.\n\n")
	sb.WriteString("Fill the optional fields ONLY when you have strong evidence:\n")
	sb.WriteString("  - language_primary: dominant language (e.g. \"go\", \"typescript\")\n")
	sb.WriteString("  - package_manager:  dep tooling (e.g. \"pnpm\", \"uv\", \"go-modules\")\n")
	sb.WriteString("  - build_cmd / test_cmd / lint_cmd / formatter: the literal command a developer would type (e.g. \"go test -race ./...\")\n")
	sb.WriteString("  - frameworks: detected framework names, most-defining first\n")
	sb.WriteString("  - notes: one-sentence summary suitable for a CLAUDE.md preamble\n\n")
	sb.WriteString("Leave any field empty if the listing doesn't support it. Prefer fewer, higher-confidence answers.\n\nFiles:\n")
	for _, f := range files {
		sb.WriteString(f)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ── post-parse validation ──────────────────────────────────────────────

// parseStackResponse takes the schema-validated JSON string and returns a
// sanitized Stack. Defense-in-depth: even though --json-schema enforces
// shape, we re-validate every field to defend against schema regressions
// and to scrub strings (trim, reject flag-injection candidates, etc.).
func parseStackResponse(raw string) (Stack, error) {
	var payload struct {
		Type            string   `json:"type"`
		LanguagePrimary string   `json:"language_primary"`
		PackageManager  string   `json:"package_manager"`
		BuildCmd        string   `json:"build_cmd"`
		TestCmd         string   `json:"test_cmd"`
		LintCmd         string   `json:"lint_cmd"`
		Formatter       string   `json:"formatter"`
		Frameworks      []string `json:"frameworks"`
		Notes           string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Stack{}, fmt.Errorf("unmarshal: %w", err)
	}

	t := Type(strings.ToLower(strings.TrimSpace(payload.Type)))
	if !t.Valid() {
		return Stack{}, fmt.Errorf("invalid type %q (want one of %v)", payload.Type, AllTypes())
	}

	return Stack{
		Type:            t,
		LanguagePrimary: sanitizeShort(payload.LanguagePrimary, 50),
		PackageManager:  sanitizeShort(payload.PackageManager, 50),
		BuildCmd:        sanitizeCommand(payload.BuildCmd),
		TestCmd:         sanitizeCommand(payload.TestCmd),
		LintCmd:         sanitizeCommand(payload.LintCmd),
		Formatter:       sanitizeCommand(payload.Formatter),
		Frameworks:      sanitizeFrameworks(payload.Frameworks),
		Notes:           sanitizeNotes(payload.Notes),
	}, nil
}

// sanitizeShort trims whitespace and enforces a length cap. Control
// chars are dropped (defends against terminal escape sequences in
// future log output).
func sanitizeShort(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 }) {
		return ""
	}
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

// sanitizeCommand trims and length-caps a command string. Leading "-"
// is allowed (some commands legitimately use flags as the first token
// after the binary, e.g. "go test"; the binary itself is the protection
// layer). We do not currently exec these commands ourselves — they end
// up in template-rendered .md files — but the bound limits blast radius
// if a future hook author shells one out.
func sanitizeCommand(s string) string {
	return sanitizeShort(s, 200)
}

// sanitizeFrameworks trims, lowercases, dedupes, and caps at 8 entries.
// Sorted for deterministic cache contents.
func sanitizeFrameworks(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" || len(s) > 50 {
			continue
		}
		if strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 }) {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) >= 8 {
			break
		}
	}
	sort.Strings(out)
	return out
}

// sanitizeNotes is sanitizeShort with the larger 500-char cap and
// CR/LF→space replacement so the string fits on one CLAUDE.md line.
func sanitizeNotes(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 }) {
		// Strip any remaining control bytes.
		var b strings.Builder
		b.Grow(len(s))
		for _, r := range s {
			if r >= 0x20 {
				b.WriteRune(r)
			}
		}
		s = b.String()
	}
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
