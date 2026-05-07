---
name: claude-subprocess
description: Patterns for invoking the `claude` CLI as a subprocess from omc — the canonical safe-invocation flag set (`--tools "" --json-schema --max-budget-usd --no-session-persistence` from a neutral cwd), the verified JSON output schema (`{type, is_error, subtype, result, structured_output, total_cost_usd, ...}`), subscription-only auth gate via `claude auth status`, graceful degradation when not on PATH or unauthenticated, cost discipline (Haiku-only, hard budget caps, file-listing-only prompts, cache by sha256 of inputs), and prompt-injection mitigations through schema-enforced output. Use this whenever editing `internal/detect/llm*.go`, `internal/detect/runner.go`, or anywhere we exec the `claude` binary. Apply also when changing the LLM augmentation flag wiring in `cmd/omc/`.
---

# Invoking Claude Code as a subprocess

omc uses `claude -p` — Claude Code's non-interactive mode — to do LLM-augmented stack detection: when file-rule detectors come up empty (or when the user opts in with `--llm-augment`), we shell out to the user's already-installed, already-authenticated `claude` binary. **This is not the same as making Anthropic API calls.** We never see the user's API key; we never bill them per call from our side; we lean on their existing Claude Code subscription auth.

**Subscription-only contract.** omc deliberately does *not* fall back to `ANTHROPIC_API_KEY` when subscription auth isn't available. The whole pitch of this path is "use what the user already pays for, never make per-call billed API requests on their behalf." If `claude auth status` returns non-zero, we degrade to zero LLM signals — even if `ANTHROPIC_API_KEY` happens to be set in the environment. Don't reintroduce the fallback "to be helpful": a user with a subscription who also has an API key set for unrelated work would be silently billed every time their OAuth hiccupped. That violates the contract.

That distinction shapes every rule below.

## The canonical safe invocation

```
cd /tmp && claude \
  -p \
  --output-format json \
  --json-schema '<schema>' \
  --tools "" \
  --model haiku \
  --effort low \
  --max-budget-usd 0.10 \
  --no-session-persistence \
  '<prompt>'
```

Note **what's NOT here**: `--bare`. `--bare` looks tempting because it skips CLAUDE.md / settings.json auto-discovery, but it *requires* `ANTHROPIC_API_KEY` (OAuth and keychain are explicitly disabled in bare mode). Using `--bare` would mean "use the user's API key for per-call billing if they have one set," which contradicts the subscription-only contract. We get the same auto-discovery suppression by simply running from a **neutral cwd** (e.g. `os.TempDir()`) so there's no project-level `CLAUDE.md` or `.claude/settings.json` to discover.

Every other flag is load-bearing. Drop one, get a footgun:

| Flag | Why required |
|---|---|
| `cmd.Dir = os.TempDir()` (not a CLI flag, but the equivalent runner setting) | Prevents `claude` from auto-loading the target project's `CLAUDE.md`, `.claude/settings.json`, hooks, or plugin config into the inference context. Without it, scanning `oh-my-claude` itself biases the answer ("ready to help with your oh-my-claude project!"). User-level config (`~/.claude/settings.json`, user CLAUDE.md) still loads, which is fine — those represent the user's deliberate context, and `--tools ""` prevents anything in them from acting on the target project. |
| `-p` (`--print`) | Non-interactive. One prompt → one response → exit. |
| `--output-format json` | Single result object. The `text` default is unparseable; `stream-json` requires `--verbose` and adds NDJSON parsing complexity we don't need. |
| `--json-schema '<schema>'` | Claude validates output against the schema *before returning*. **This is the determinism guarantee** — without it, parsing free-form text turns every prompt-injection attempt into a real bug. With it, the worst case is "the model returns no signals," not "the model returns shell commands." |
| `--tools ""` | Disables every tool. Pure inference, no tool calls. Without this Claude could `Bash`, `Read`, etc. — a security and surprise hazard. |
| `--model haiku` | Cheap and fast. A file-listing classifier doesn't need Sonnet/Opus, and the cost discipline section below is enforced against Haiku's pricing. |
| `--effort low` | Minimum thinking budget. Stack identification doesn't need extended reasoning. |
| `--max-budget-usd 0.10` | Hard cap. Overrun returns `is_error: true, subtype: "error_max_budget_usd"`. Treat that as a clean degradation, not a failure. **This is the real safety net — do not also pass `--max-turns`.** When `--json-schema` is set, the model often takes 2–4 turns to converge on a schema-compliant answer (response → re-prompt with schema error → corrected response). A tight `--max-turns` guarantees `error_max_turns` failures even on valid prompts; let the budget cap be the floor. |
| `--no-session-persistence` | Don't save this session to `~/.claude/projects/...`. We're scripting; sessions clutter the user's resume picker for nothing. |

## The JSON output schema (verified against claude 2.1.x)

```jsonc
{
  "type": "result",
  "subtype": "success" | "error_max_budget_usd" | "error_auth" | "error_max_turns" | ...,
  "is_error": false,
  "result": "<prose response, OR empty string when --json-schema set>",
  "structured_output": { ... schema-conforming object, ONLY when --json-schema set ... },
  "stop_reason": "end_turn" | "max_turns" | "error",
  "session_id": "uuid",
  "total_cost_usd": 0.00978,
  "duration_ms": 2353,
  "num_turns": 2,
  "usage": { "input_tokens": int, "output_tokens": int, "cache_creation_input_tokens": int, "cache_read_input_tokens": int, ... },
  "modelUsage": { "claude-haiku-4-5-...": { "inputTokens": int, "outputTokens": int, "costUSD": num, ... } },
  "permission_denials": [],
  "uuid": "..."
}
```

**Critical: when `--json-schema` is set, the schema-conforming payload lives in `structured_output`, NOT `result`.** `result` will be empty (or contain prose if you forgot to tell the model "output ONLY JSON"). A naïve "parse `result` as JSON" reader will see empty and degrade incorrectly. Read `structured_output` first; fall back to `result` only as a compatibility shim for older Claude Code versions.

Don't trust `is_error == false` alone; also check `subtype == "success"`. CLI exit code is `0` on success and `1` on any structured error — but the JSON is still on stdout, so always parse it before treating exit-1 as a failure.

**Also load-bearing: tell the model "Output ONLY a JSON object matching the schema. No prose, no markdown." in the user prompt.** Without that instruction, Haiku returns prose in `result` and leaves `structured_output` empty *even though `--json-schema` is set* — and it burns ~3× the tokens doing it. Schema enforcement is a hint to the model, not a hard guarantee, unless the user prompt aligns.

## Auth detection: subscription only

There is exactly one valid auth mode for omc:

- **Subscription user (Pro/Max).** Already `claude auth login`'d. `claude auth status` exits `0`. We invoke without `--bare` so OAuth works.

If `claude auth status` returns non-zero, we degrade — even if `ANTHROPIC_API_KEY` is set in the environment. **Do not "helpfully" fall back to API-key mode.** That would silently bill the user per call from omc, contradicting the subscription-only contract and creating a footgun: a subscription user with an API key set for unrelated work would get billed every time their OAuth hiccupped.

```go
type claudeMode int

const (
    claudeModeUnavailable claudeMode = iota
    claudeModeUnauthenticated
    claudeModeSubscription
)

func detectClaudeMode(ctx context.Context, runner Runner) claudeMode {
    if out, err := runner.Run(ctx, []string{"--version"}, nil); err != nil || len(out) == 0 {
        return claudeModeUnavailable
    }
    if _, err := runner.Run(ctx, []string{"auth", "status"}, nil); err == nil {
        return claudeModeSubscription
    }
    return claudeModeUnauthenticated
}
```

`claudeModeUnavailable` and `claudeModeUnauthenticated` are not errors that should propagate to the user as failures — they should produce a friendly "skipping LLM augmentation: claude not installed / not authenticated against a subscription (run `claude auth login`)" line and let the file-rule signals stand.

## Graceful degradation — never a hard failure

LLM detection is **always** opt-in or fallback. It must never:

- Fail the entire `omc init` run because `claude` isn't on PATH.
- Pop a permission prompt asking the user to authorize anything.
- Block forever — every invocation must have a `context.Context` with a deadline (default 90s; cold-cache schema-validated Haiku calls regularly take 10–20s).
- Spend more than `--max-budget-usd` says.

**Critical detail: `claude -p` returns exit code 1 on structured errors** (e.g. `error_max_budget_usd`, `error_max_turns`) but still writes a parseable JSON object to stdout. Your runner must surface stdout even when exit is non-zero — otherwise you lose the `subtype` and degrade with a useless "exit status 1" log instead of "skipping (budget exceeded)" or "skipping (schema convergence failed)". The right shape: try parsing stdout first; only treat as exec failure if stdout is empty.

The full ladder, in order:

1. `claude` not on PATH → return zero signals, log "skipping (claude not installed)" at info level.
2. `claude auth status` exit 1 *and* no `ANTHROPIC_API_KEY` → return zero signals, log "skipping (claude not authenticated; run `claude auth login`)".
3. Cache hit (sha256 of sorted file listing matches) → return cached signals, no subprocess invocation.
4. Subprocess invocation: 30s context deadline, parsed JSON, schema-validated `result`, cost recorded.
5. Any of: timeout, non-zero exit + non-JSON output, `is_error: true`, malformed `result` → return zero signals, log the reason.
6. Success → write to cache, return parsed signals.

The user sees the same `omc init` output regardless of which rung the ladder stopped at — file-rule signals come out, LLM signals come out, both come out, or neither — but the subprocess never crashes the parent.

## Prompt construction: file listings only

**Send file paths, not file contents.** A file *path* is short, structured, hard to weaponize. A file *content* is unbounded, can contain prompt-injection payloads, and inflates token cost dramatically.

Bad:
```
"Here is package.json: {{ contents of package.json }}. What stack is this?"
```

Good:
```
"Identify the technology stacks present in this project from the file paths below.
Files:
package.json
src/main.ts
.github/workflows/ci.yml
..."
```

If the file listing exceeds ~500 paths, sample deterministically (e.g., keep all root-level files + first N of each top-level subdir, sorted) so the cache key stays stable.

## Prompt-injection mitigations

`--json-schema` does most of the work — Claude can't return arbitrary text when the schema constrains output to e.g.:

```json
{
  "type": "object",
  "required": ["signals"],
  "properties": {
    "signals": {
      "type": "array", "maxItems": 20,
      "items": {
        "type": "object",
        "required": ["name", "category", "evidence"],
        "properties": {
          "name":     {"type": "string"},
          "category": {"type": "string", "enum": ["language","package-manager","framework","db","orm","infra","ci","monorepo"]},
          "evidence": {"type": "array", "minItems": 1, "items": {"type": "string"}}
        }
      }
    }
  }
}
```

**Keep the schema lean.** Tight constraints (`additionalProperties: false`, narrow `maxLength`/`maxItems`/`minLength` everywhere, deeply nested required-fields) push the model into correction loops that double or triple turn count and cost. The minimal schema above costs ~$0.005 per call on a small project; the maximalist version regularly hits the 0.02 budget cap. Use the post-parse validator, not the schema, for content rules — the schema is for *shape*, the validator is for *content*.

After parsing, **still validate post-hoc**:

- `name` matches `^[a-z][a-z0-9._-]*$`. Discard non-conforming.
- `category` matches a known tag. Discard otherwise.
- `evidence` strings are bounded length, contain no control chars, don't start with `-` (no flag-smuggling if we ever render them on a CLI).

Treat the LLM output as adversarial input. The schema prevents *shape* attacks. The validator prevents *content* attacks.

## Cost discipline

omc must be free to run for the user. No exceptions to the following:

- **Always** `--model haiku`. We don't expose a flag to override.
- **Always** `--max-budget-usd <small>`. Default 0.10. Cold-cache Haiku calls regularly cost $0.01–0.03 because the Claude Code system prompt (~7k tokens) is charged at cache-creation rate on the first call; subsequent calls amortize via our local response cache, so the user effectively pays once per project. A $0.02 cap fails the first-run on most projects; $0.10 is generous enough to never fail and small enough to be irrelevant.
- **Always** cache by `sha256(sorted_file_listing)` under `os.UserCacheDir()/omc/llm-detect/<hash>.json`, TTL 7 days. Same project → same answer → no second call.
- **Never** include file *contents* in prompts. Listings only.
- **Never** retry on `error_max_budget_usd` — the user's already paid for that attempt and we deliberately set the cap.
- **Track** `total_cost_usd` from each response and surface it in `omc doctor --verbose` so users can see how much we've spent on their behalf.

For a typical 200-path project on Haiku with `--bare --effort low`, expect ~$0.001-0.005 per call; cache amortizes to free.

## The Runner abstraction (for tests)

The exec call is wrapped in an interface so tests don't need a real `claude` binary:

```go
type Runner interface {
    Run(ctx context.Context, args []string, stdin []byte) (stdout []byte, err error)
}
```

Production: `realRunner{}` calls `exec.CommandContext`. Tests: `fakeRunner{response: <canned JSON>}`. Tests run hermetically and can simulate every error path (auth failure, budget cap, malformed output, timeout).

## Things you should NOT do

- **Don't shell out to `claude` from inside a rule-based detector.** Rule detectors are file-only and synchronous; the LLM path is a separate seam in `internal/detect/llm.go` invoked by the runner.
- **Don't pass file contents in the prompt** unless you've added a sanitizer that strips known prompt-injection markers and bounded the size. Listings are the contract.
- **Don't drop `--json-schema`** for "easier parsing." Free-form text parsing is how injection attacks land.
- **Don't drop `--tools ""`.** The day you forget, a malicious repo gets to run `Bash` on the user's machine.
- **Don't loop on errors.** One attempt per scan. Failures degrade silently.
- **Don't write `--max-budget-usd 0`.** Some Claude Code versions treat `0` as "no budget" — use `0.001` if you mean "almost nothing."
- **Don't assume the schema is stable across Claude Code major versions.** Pin a minimum version in `omc doctor` and parse defensively (unknown top-level fields are fine; missing `result` is a hard failure).
