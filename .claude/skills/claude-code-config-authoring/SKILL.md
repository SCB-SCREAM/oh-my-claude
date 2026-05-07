---
name: claude-code-config-authoring
description: The schema of Claude Code project config files (settings.json, hooks, .claude/commands/*.md, .claude/agents/*.md, .mcp.json) — i.e. the artifacts omc generates. Use this skill whenever editing internal/templates/<stack>/, designing template output, writing the settings.json deep-merge logic in internal/apply/, or unsure of the exact shape of a Claude Code config artifact. Critical — getting the schema wrong silently breaks every install of every stack we support, so apply this skill aggressively even on template tweaks that look cosmetic.
---

# The shapes `omc` generates

`omc` writes Claude Code config files into a user's repo. **The schemas below are what we target.** Templates under `internal/templates/<stack>/` must produce conformant output; the deep-merge logic in `internal/apply/settings_merge.go` must respect this structure.

> The Claude Code config schema evolves. Pin a Claude Code version in `README.md` ("templates target Claude Code ≥ X.Y"), and have `omc doctor` warn on drift. When the schema changes upstream, this skill is the first thing to update.

## File map

```
<repo>/
├── CLAUDE.md                       Project-level instructions (always loaded)
├── .mcp.json                       Project-scoped MCP server config (optional, opt-in)
└── .claude/
    ├── settings.json               Project settings (committed)
    ├── settings.local.json         Per-user overrides (gitignored)
    ├── commands/<name>.md          Slash commands (markdown w/ frontmatter)
    ├── agents/<name>.md            Subagents (markdown w/ frontmatter)
    └── hooks/<script>              Hook scripts referenced from settings.json
```

`omc` always offers to add `.claude/settings.local.json` and `.claude/.omc-backup-*/` to `.gitignore`.

## `CLAUDE.md`

Plain markdown. No frontmatter. Loaded into context automatically. Keep it tight (<200 lines). Sections we generate per stack:

- One-paragraph project description (templated from detected stack name).
- "Common commands" (test/build/lint/format/dev) using the stack's package manager.
- "Conventions" (stack-specific gotchas — e.g. "use `uv`, not `pip`").
- "Where to add things" (placeholder sections the user fills in).

Stamp the top with `<!-- omc-template: <stack> v<version> -->` so a future `omc update` can detect drift.

## `.claude/settings.json`

JSON. The shape we target:

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(npm test*)",
      "Bash(npm run build*)",
      "Bash(pnpm test*)"
    ],
    "deny": [
      "Bash(rm -rf*)",
      "Bash(curl * | sh)"
    ]
  },
  "env": {
    "FOO": "bar"
  },
  "model": "claude-opus-4-7",
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          { "type": "command", "command": ".claude/hooks/guard-rm.sh" }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          { "type": "command", "command": ".claude/hooks/format.sh" }
        ]
      }
    ]
  }
}
```

### Permissions

- `permissions.allow` and `permissions.deny` are arrays of **rule strings**, not objects.
- Most common form is `Tool(pattern)` — e.g. `Bash(npm test*)`, `Read(/etc/**)`, `Write(./tmp/**)`. The `*` is a glob.
- Bash patterns match the *command line*, not the binary alone. `Bash(npm test*)` matches `npm test`, `npm test --watch`, etc.
- `deny` wins over `allow`. Always include a small deny list for destructive primitives (`rm -rf*`, `curl * | sh`, `sudo *`).
- We dedupe across `allow`/`deny` during merge (see "Deep merge" below).

### `model`

Optional. Leave blank in templates — users pick. Document supported IDs in the generated `CLAUDE.md` if relevant.

### `env`

Map of `string → string`. Vars set when Claude Code launches the session.

### `hooks`

Keyed by **event name**. Each event holds an array of `{ matcher, hooks }` objects. Each `hooks[]` entry is `{ type, command }` (only `type: "command"` is widely supported).

Events we use:

- `PreToolUse` — before any tool runs. Matcher targets a tool (`Bash`, `Edit`, `Write`, `Read`, etc.). Stdin is the tool input as JSON; non-zero exit blocks the call.
- `PostToolUse` — after a tool runs. Same matcher. Used for formatters / linters.
- `Notification` — UI notifications (rarely needed).
- `Stop` — when the model stops. Used for "did you run tests?" reminders.
- `UserPromptSubmit` — before a user prompt is sent. Use sparingly.
- `SessionStart` / `SessionEnd` — bookend hooks. No matcher.

Hook scripts go in `.claude/hooks/<name>.sh` (or `.cmd` on Windows — see the Windows note below). They receive JSON on stdin and may emit JSON on stdout to feed back into Claude Code (e.g. `{"decision":"block","reason":"…"}` from a `PreToolUse`).

**Hooks are executable surface.** Templates ship them *opt-in only* — never preselected in the `minimal` profile, always small and auditable, always gated on a relevant signal.

### Deep-merge rules (`internal/apply/settings_merge.go`)

When the user already has a `settings.json`, we merge rather than overwrite:

- `permissions.allow`, `permissions.deny`: union, dedupe (preserve user's order; append new entries from us).
- `hooks.<event>`: append our `{matcher, hooks}` objects to the user's array. **Don't** replace by matcher — multiple matchers per event are valid.
- `env`: shallow merge; user values win on conflict.
- `model`: user wins if set; we never overwrite.
- Anything we don't recognize: pass through untouched.

Round-trip: a no-op merge (`MergeSettings(x, {}) == x`) must be exact.

## `.mcp.json`

Project-scoped MCP server configuration. JSON.

```json
{
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_TOKEN": "${GITHUB_TOKEN}" }
    },
    "postgres": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres", "${DATABASE_URL}"]
    }
  }
}
```

Per server:

- `command` — executable.
- `args` — argv.
- `env` — extra env. `${VAR}` expansion is done by Claude Code, not us — keep the literal string.

Templates ship `.mcp.json` **commented out by default** (write it as `.mcp.json.example` or include a header note); MCP servers exec arbitrary code, so opt-in is the right default.

## Slash commands: `.claude/commands/<name>.md`

Markdown file with optional YAML frontmatter. The filename (without `.md`) becomes the slash command name.

```markdown
---
description: Run the project's test suite and report failures
allowed-tools: Bash, Read
---

Run `npm test`. If it fails, summarize the failure and suggest the smallest
change that would make the test pass.
```

- `description` — shown in `/help` and used by Claude Code to decide whether to surface it.
- `allowed-tools` — restrict which tools the command may use (optional).
- Body — the prompt that runs when the user types `/<name>`.

Templates we ship per stack: `/test`, `/lint`, `/format`, `/typecheck`, `/migrate` (when ORM detected), `/deploy` (when CI detected).

## Subagents: `.claude/agents/<name>.md`

Same shape as commands but the body describes a *persona* and Claude Code spawns a sub-instance with that system prompt when invoked.

```markdown
---
name: test-runner
description: Runs and interprets failing tests. Use whenever the user reports a test failure.
tools: Bash, Read, Edit
model: claude-haiku-4-5-20251001
---

You are a focused test-running assistant. Run the failing tests, read tracebacks,
and propose the minimal fix. Don't refactor unrelated code.
```

- `description` is third-person and pushy (same rules as Skill descriptions).
- `tools` restricts the subagent to a subset.
- `model` overrides the default.

Templates we ship: `test-runner` (always), `migration-reviewer` (when DB signal present).

## Skills (`.claude/skills/<name>/SKILL.md`)

`omc` does **not** generate user-facing skills in v1. (This repo has skills for *contributors* — that's separate.) If we add skill generation later, the format is documented in [Anthropic's skills docs](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview); `name` ≤64 chars / lowercase-hyphenated, `description` ≤1024 chars / third person / what + when.

## Cross-cutting: Windows

- Hook scripts on Windows want `.cmd` or `.ps1`, not `.sh`. Decide per template: ship a polyglot launcher, or ship two parallel scripts and emit different paths in `settings.json` based on detected OS.
- Path separators in `settings.json` should always be `/` — Claude Code normalizes.
- `.claude/.omc-backup-*` paths use forward slashes too.

## Validating before writing

In `internal/apply`:

1. After deep-merging `settings.json`, validate the result against a JSON schema (committed under `internal/apply/testdata/`) before writing.
2. For each generated file, run a "dry parse" appropriate to the format (JSON for `.json`, YAML for `.yaml`, frontmatter parser for command/agent files).
3. If validation fails, abort the whole plan with a clear error — never write a partially-broken setup.

## Don't

- Don't invent fields. If the schema doesn't have it, we don't write it.
- Don't write absolute paths into `settings.json` — they break for every other contributor.
- Don't put secrets in `settings.json` — point at env vars, leave the user to populate `settings.local.json`.
- Don't pre-enable hooks or MCP servers in the `minimal` profile.
- Don't generate `.claude/settings.local.json` — that's the *user's* file.
