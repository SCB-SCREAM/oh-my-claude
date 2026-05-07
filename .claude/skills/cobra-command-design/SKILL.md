---
name: cobra-command-design
description: How to author Cobra commands and flags in this repo. Use this whenever editing files under cmd/omc/, adding a new subcommand, designing flags (persistent vs local, short vs long), populating help text or examples, validating inputs, or wiring up shell completions. Always prefer RunE over Run for proper error returns. Apply this even to "small tweaks" of existing commands — the conventions here are the difference between a clean help output and a confusing one.
---

# Cobra command design in this repo

`omc`'s CLI surface is built with [spf13/cobra](https://github.com/spf13/cobra). This skill captures the conventions every command in `cmd/omc/` follows.

## The skeleton

Every subcommand lives in its own file (`cmd/omc/<name>.go`) and exports a constructor. `main.go` builds the root and wires children.

```go
// cmd/omc/init.go
package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/<owner>/oh-my-claude/internal/apply"
	"github.com/<owner>/oh-my-claude/internal/profile"
)

type initOpts struct {
	profile string
	yes     bool
	dryRun  bool
}

func newInitCmd() *cobra.Command {
	var opts initOpts
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Detect this project's stack and bootstrap a Claude Code setup",
		Long: `init scans the current directory, asks for a profile, and writes
CLAUDE.md, .claude/settings.json, hooks, slash commands, and subagent stubs.`,
		Example: `  omc init
  omc init --profile recommended --yes
  omc init --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateProfile(opts.profile); err != nil {
				return err
			}
			return runInit(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.profile, "profile", "", "minimal | recommended | full (required with --yes)")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "skip the TUI and apply the chosen profile non-interactively")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print the plan but don't write anything")
	return cmd
}
```

```go
// cmd/omc/main.go
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1) // Cobra already printed the error to stderr.
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "omc",
		Short:         "Bootstrap Claude Code for your project, opinionated by stack",
		SilenceUsage:  true, // don't dump help on every error
		SilenceErrors: false,
	}
	root.AddCommand(newInitCmd(), newDoctorCmd(), newStacksCmd(), newVersionCmd())
	return root
}
```

## Rules

### `RunE`, not `Run`

`RunE` returns an `error` so Cobra can print it and exit non-zero. `Run` swallows everything, leaving the user with `exit 0` on a failed command. **Always `RunE`.** The only exception is a command with literally no failure mode (rare).

### Pass `cmd.Context()`

Inside `RunE`, get the context from `cmd.Context()` and pass it down. We set up `signal.NotifyContext` in `main`, so `Ctrl+C` propagates as a cancelled context. Long-running operations (`detect.Run`, `apply.Execute`) take `ctx context.Context` as their first arg.

### `SilenceUsage: true` on root

Without this, Cobra prints the full help on every returned error — annoying for runtime errors that aren't usage problems. Set it on the root; subcommands inherit.

### Flags: persistent vs local

**Persistent flags** are inherited by subcommands. Use them only for genuinely cross-cutting concerns:

- `--config <path>` — alternate config file
- `--no-color` (or rely on `NO_COLOR` env) — disable styling
- `-v / --verbose` — global log verbosity

**Local flags** are the default. `--profile`, `--yes`, `--dry-run` belong on `init` only — putting them on the root would be confusing.

### Short flags only for the obvious

Reserve single-letter shortcuts for flags users will type often (`-v`, `-h`). Don't burn the alphabet on rarely-typed flags. `-p` for `--profile` is fine; `-y` for `--yes` is fine; `-d` for `--dry-run` is *probably* fine. When unsure, long-only is safer.

### `Args:` constraint, always

Set an explicit `Args:` (`cobra.NoArgs`, `cobra.ExactArgs(1)`, `cobra.MinimumNArgs(1)`, etc.). Don't accept undeclared positional arguments — they silently swallow typos.

### Validate before doing work

Inside `RunE`:

1. Check flag combos (e.g. `--yes` requires `--profile`).
2. Check the environment (e.g. is this a git repo? is `.claude/` writable?).
3. *Then* call into `internal/...`.

Failing fast with a clear error beats half-applying a plan.

### `Example:` is documentation

Help text is what users see. Always set:

- `Short:` — single line, < 80 chars, no period.
- `Long:` — a paragraph; describes what the command does and key behaviors.
- `Example:` — copy-pasteable command lines, indented two spaces.

Examples should *work* against a representative repo. Stale examples mislead users.

### Errors are messages to the user

Errors returned from `RunE` are printed verbatim. Wrap them with context:

```go
if err := apply.Execute(ctx, plan); err != nil {
    return fmt.Errorf("apply: %w", err)
}
```

Don't `os.Exit(1)` from inside a handler — return the error and let Cobra/`main` exit.

### Shell completions

Cobra generates them automatically. Wire a `completion` command (or rely on the auto-registered one) so users can `omc completion bash > /etc/bash_completion.d/omc`. For dynamic completions on flag values (e.g. `--profile <Tab>` listing `minimal|recommended|full`), use `RegisterFlagCompletionFunc`:

```go
_ = cmd.RegisterFlagCompletionFunc("profile",
    func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
        return []string{"minimal", "recommended", "full"}, cobra.ShellCompDirectiveNoFileComp
    })
```

## Common patterns specific to omc

- **TUI vs no-TUI**: `init` defaults to TUI. If `--yes` is set or stdout is not a TTY (`!isatty.IsTerminal(os.Stdout.Fd())`), require `--profile` and run headless.
- **Dry-run**: `--dry-run` builds a plan, prints it, returns. No file writes anywhere.
- **`omc doctor`**: read-only diagnostic. Always exits 0 unless invoked incorrectly; "found problems" is communicated through stdout, not exit code, so users can run it in scripts.
- **`omc stacks`**: prints a table — use `text/tabwriter` for alignment.
- **`omc --version`**: handled by a `--version` persistent flag on root, populated from `internal/version`. Don't write a separate `version` subcommand unless we have a reason.

## Don't

- Don't put business logic in `RunE` — it should be ~10 lines: parse flags, validate, call into `internal/`.
- Don't read environment variables ad-hoc inside subcommands; centralize in a small config struct that flags + env feed into.
- Don't use Viper unless we have a real config file. We don't — flags + a few env vars are enough.
- Don't mix interactive prompts with the TUI. Either we're in Bubble Tea or we're not.
