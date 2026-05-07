package detect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// LLMOptions configures the LLM-augmented detection path. The zero value
// is the production default: 30s timeout, 0.02 USD budget cap, default
// cache TTL of 7 days, default Runner that shells out to `claude`.
type LLMOptions struct {
	// Runner executes the underlying `claude` subprocess. Override in
	// tests with a fake; nil falls back to [defaultRunner].
	Runner Runner

	// Timeout bounds a single subprocess invocation. Zero means 30s.
	Timeout time.Duration

	// BudgetUSD is forwarded to `--max-budget-usd`. Zero means 0.02.
	// Never set this above 1.0 — the whole point of the path is that
	// it's free-or-near-free for the user.
	BudgetUSD float64

	// CacheDir is the directory to read/write cached responses in.
	// Zero means [os.UserCacheDir]/omc/llm-detect/.
	CacheDir string

	// CacheTTL is the maximum age of a cache hit. Zero means 7d.
	CacheTTL time.Duration

	// SkipCache disables reading from the cache (still writes on success).
	SkipCache bool

	// Verbose, if non-nil, receives one human-readable line per
	// degradation rung the runner stops at. Used by `omc init` to
	// surface "skipping (claude not authenticated)" messages.
	Verbose func(format string, args ...any)
}

// llmCategoryToTag maps the schema's `category` enum to the Tag vocabulary
// we use elsewhere in this package. Anything outside this map is rejected
// post-parse.
var llmCategoryToTag = map[string]string{
	"language":        "language",
	"package-manager": "package-manager",
	"framework":       "framework",
	"db":              "db",
	"orm":             "orm",
	"infra":           "infra",
	"ci":              "ci",
	"monorepo":        "monorepo",
}

// reLLMName mirrors the post-parse validator in the claude-subprocess
// skill: lowercase identifier with optional dots, dashes, underscores.
var reLLMName = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)

// RunViaLLM augments the supplied rule-based signals with LLM-inferred
// signals. It never returns an error that should fail the parent run —
// any failure is logged via opts.Verbose and the function returns nil.
//
// Signals returned are guaranteed to:
//   - have at least one evidence path
//   - carry the "llm-inferred" tag plus exactly one category tag
//   - have Confidence ≤ [ConfHeuristic]
//   - not duplicate any name in `existing` (rule signals win)
func RunViaLLM(ctx context.Context, snap *Snapshot, existing []Signal, opts LLMOptions) []Signal {
	if snap == nil || len(snap.Files) == 0 {
		return nil
	}
	opts = opts.withDefaults()

	mode := detectClaudeMode(ctx, opts)
	if mode == claudeModeUnavailable {
		opts.Verbose("skipping LLM augmentation: `claude` not on PATH")
		return nil
	}
	if mode == claudeModeUnauthenticated {
		opts.Verbose("skipping LLM augmentation: claude not authenticated against a subscription (run `claude auth login`)")
		return nil
	}

	listing := buildListing(snap.Files)
	cacheKey := hashListing(listing)

	if !opts.SkipCache {
		if hit, ok := readCache(opts.CacheDir, cacheKey, opts.CacheTTL); ok {
			return filterAgainstExisting(hit, existing)
		}
	}

	resp, err := callClaude(ctx, opts, listing)
	if err != nil {
		opts.Verbose("skipping LLM augmentation: %v", err)
		return nil
	}

	signals := parseLLMResponse(resp)
	signals = filterAgainstExisting(signals, existing)
	writeCache(opts.CacheDir, cacheKey, signals)
	return signals
}

func (o LLMOptions) withDefaults() LLMOptions {
	if o.Runner == nil {
		o.Runner = defaultRunner
	}
	if o.Timeout == 0 {
		o.Timeout = 90 * time.Second
	}
	if o.BudgetUSD == 0 {
		// Cold-cache Haiku calls run ~$0.01-0.03 because the Claude Code
		// system prompt is ~7k tokens and gets charged at cache-creation
		// rate on first call. Subsequent calls hit our local response
		// cache and cost $0.00. Be generous on the cap so first-runs
		// don't fail; the user pays at most this once per project.
		o.BudgetUSD = 0.10
	}
	if o.CacheTTL == 0 {
		o.CacheTTL = 7 * 24 * time.Hour
	}
	if o.CacheDir == "" {
		if base, err := os.UserCacheDir(); err == nil {
			o.CacheDir = filepath.Join(base, "omc", "llm-detect")
		}
	}
	if o.Verbose == nil {
		o.Verbose = func(string, ...any) {}
	}
	return o
}

// buildListing produces a deterministic, bounded prompt input from a
// snapshot's file list. We cap at 500 paths so a giant monorepo doesn't
// blow up the prompt cost; the cap is taken in sorted order so the cache
// key stays stable across runs.
func buildListing(files []string) []string {
	const listingCap = 500
	if len(files) <= listingCap {
		return files
	}
	out := make([]string, listingCap)
	copy(out, files[:listingCap])
	return out
}

func hashListing(files []string) string {
	h := sha256.New()
	for _, f := range files {
		_, _ = h.Write([]byte(f))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// filterAgainstExisting drops any LLM signal whose name is already
// present in the rule-based result. Rule wins on collision — we don't
// merge confidence or evidence.
func filterAgainstExisting(llm []Signal, existing []Signal) []Signal {
	if len(llm) == 0 {
		return nil
	}
	have := make(map[string]struct{}, len(existing))
	for _, s := range existing {
		have[s.Name] = struct{}{}
	}
	out := llm[:0]
	for _, s := range llm {
		if _, dup := have[s.Name]; dup {
			continue
		}
		out = append(out, s)
	}
	// Sort deterministically: highest confidence first, then by name.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// parseLLMResponse takes the raw `result` string (already JSON-validated
// by `--json-schema`) and converts it into a slice of [Signal]s. Defends
// against schema regressions with post-parse validation.
func parseLLMResponse(result string) []Signal {
	var payload struct {
		Signals []struct {
			Name      string   `json:"name"`
			Category  string   `json:"category"`
			Evidence  []string `json:"evidence"`
			Rationale string   `json:"rationale"`
		} `json:"signals"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return nil
	}
	var out []Signal
	for _, s := range payload.Signals {
		name := strings.ToLower(strings.TrimSpace(s.Name))
		if !reLLMName.MatchString(name) {
			continue
		}
		tag, ok := llmCategoryToTag[s.Category]
		if !ok {
			continue
		}
		ev := sanitizeEvidence(s.Evidence)
		if len(ev) == 0 {
			continue
		}
		out = append(out, Signal{
			Name:       name,
			Confidence: ConfHeuristic,
			Evidence:   ev,
			Tags:       []string{tag, "llm-inferred"},
		})
	}
	return out
}

// sanitizeEvidence drops empty strings, control chars, and any entry that
// would let a malicious filename smuggle a flag onto a CLI later
// (anything starting with `-`).
func sanitizeEvidence(in []string) []string {
	const maxItems = 5
	const maxLen = 200
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || len(s) > maxLen || strings.HasPrefix(s, "-") {
			continue
		}
		if strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 }) {
			continue
		}
		out = append(out, s)
		if len(out) >= maxItems {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

// ── cache (sha256 → JSON) ────────────────────────────────────────────────

type cacheFile struct {
	Written time.Time `json:"written"`
	Signals []Signal  `json:"signals"`
}

func cachePath(dir, key string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, key+".json")
}

func readCache(dir, key string, ttl time.Duration) ([]Signal, bool) {
	p := cachePath(dir, key)
	if p == "" {
		return nil, false
	}
	data, err := os.ReadFile(p) //#nosec G304 -- cache path is sha256-derived, under our cache dir
	if err != nil {
		return nil, false
	}
	var f cacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, false
	}
	if time.Since(f.Written) > ttl {
		return nil, false
	}
	return f.Signals, true
}

func writeCache(dir, key string, signals []Signal) {
	p := cachePath(dir, key)
	if p == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return
	}
	data, err := json.MarshalIndent(cacheFile{Written: time.Now(), Signals: signals}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o600)
}

// ── auth detection ──────────────────────────────────────────────────────

// claudeMode tracks whether `claude` is available and the user is
// authenticated against their Claude Code subscription. We deliberately
// do NOT support ANTHROPIC_API_KEY fallback — the whole pitch of this
// path is "use the user's existing subscription, never make per-call
// billed API requests on their behalf." If the subscription auth isn't
// available, we degrade.
type claudeMode int

const (
	claudeModeUnavailable claudeMode = iota
	claudeModeUnauthenticated
	claudeModeSubscription
)

func detectClaudeMode(ctx context.Context, opts LLMOptions) claudeMode {
	out, err := opts.Runner.Run(ctx, []string{"--version"}, nil)
	if err != nil || len(out) == 0 {
		return claudeModeUnavailable
	}
	// `claude auth status` exits 0 iff a subscription session is active.
	if _, err := opts.Runner.Run(ctx, []string{"auth", "status"}, nil); err == nil {
		return claudeModeSubscription
	}
	return claudeModeUnauthenticated
}

// ── subprocess invocation ──────────────────────────────────────────────

// outputSchema is the JSON Schema enforced by `--json-schema`. Kept lean
// — `additionalProperties: false` and tight string-length bounds drive
// the model into correction loops, which burns budget; the post-parse
// validator in [parseLLMResponse] catches the long-tail garbage.
const outputSchema = `{
  "type": "object",
  "required": ["signals"],
  "properties": {
    "signals": {
      "type": "array",
      "maxItems": 20,
      "items": {
        "type": "object",
        "required": ["name", "category", "evidence"],
        "properties": {
          "name": {"type": "string"},
          "category": {"type": "string", "enum": ["language","package-manager","framework","db","orm","infra","ci","monorepo"]},
          "evidence": {"type": "array", "minItems": 1, "items": {"type": "string"}}
        }
      }
    }
  }
}`

// claudeResult is the top-level shape of `claude -p --output-format
// json`. When `--json-schema` is set, the schema-conforming payload is
// in StructuredOutput, NOT Result — Result holds prose or is empty.
// Unknown fields are ignored.
type claudeResult struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	StopReason       string          `json:"stop_reason"`
}

func callClaude(parent context.Context, opts LLMOptions, listing []string) (string, error) {
	prompt := buildPrompt(listing)

	// We deliberately do NOT pass `--bare` — `--bare` strictly uses
	// ANTHROPIC_API_KEY (per-call billing), which contradicts the
	// "subscription only" guarantee. To still avoid CLAUDE.md /
	// .claude/settings.json auto-discovery from the target project
	// (which would bias detection), the runner sets cwd to a neutral
	// directory before exec. See [realRunner].
	args := []string{
		"-p",
		"--output-format", "json",
		"--json-schema", outputSchema,
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
	// With --json-schema set, the schema-conforming payload is in
	// `structured_output` and `result` is empty/prose. Fall back to
	// `result` only if structured_output is absent (older Claude Code
	// versions or schema validation disabled).
	if len(result.StructuredOutput) > 0 && string(result.StructuredOutput) != "null" {
		return string(result.StructuredOutput), nil
	}
	if result.Result == "" {
		return "", errors.New("claude returned no structured_output and empty result")
	}
	return result.Result, nil
}

func buildPrompt(files []string) string {
	var sb strings.Builder
	// "Output ONLY JSON" is load-bearing — without it, even with
	// --json-schema set, Haiku returns prose in `result` and leaves
	// `structured_output` empty, which costs ~3x more and gives us
	// nothing to parse.
	sb.WriteString("Output ONLY a JSON object matching the schema. No prose, no markdown.\n\n")
	sb.WriteString("Identify the technology stacks present in this project from the file paths below. ")
	sb.WriteString("Only emit signals you have strong evidence for; prefer fewer, higher-quality entries. ")
	sb.WriteString("Choose `category` from the schema enum that fits each signal. ")
	sb.WriteString("Use the file paths as evidence; do not invent files that aren't listed.\n\nFiles:\n")
	for _, f := range files {
		sb.WriteString(f)
		sb.WriteByte('\n')
	}
	return sb.String()
}
