package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
	"github.com/SCB-SCREAM/oh-my-claude/internal/profile"
	"github.com/SCB-SCREAM/oh-my-claude/internal/report"
	"github.com/SCB-SCREAM/oh-my-claude/internal/session"
	"github.com/SCB-SCREAM/oh-my-claude/internal/tui"
	"github.com/SCB-SCREAM/oh-my-claude/internal/version"
)

type initOpts struct {
	profile            string
	yes                bool
	dryRun             bool
	noTUI              bool
	refreshDetection   bool
	components         []string
	simulateApplyDelay time.Duration
}

func newInitCmd() *cobra.Command {
	var opts initOpts
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Detect this project's stack and bootstrap a Claude Code setup",
		Long: `init scans the current directory, asks the local claude CLI to
classify the project (one of: webapp, api, cli, library, infra, monorepo),
chooses a profile, and writes CLAUDE.md, .claude/settings.json, hooks,
slash commands, and subagent stubs tuned to the detected Stack.

omc requires the claude CLI to be installed and authenticated against an
active subscription. Detection results are cached in .claude/omc/detected.json,
so repeat runs on an unchanged project are free (no claude call).

When stdout is not a TTY (e.g. piped to another command), init auto-flips
to --no-tui mode. --profile is required in that case.`,
		Example: `  omc init
  omc init --profile recommended --yes
  omc init --no-tui --profile minimal --components claude-md,settings
  omc init --refresh-detection
  omc init --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !opts.noTUI && !opts.yes && !isatty.IsTerminal(os.Stdout.Fd()) {
				opts.noTUI = true
			}

			if err := opts.validate(); err != nil {
				return err
			}

			// Hard preflight: omc requires a working claude CLI for
			// stack detection. Fail fast with an actionable message so
			// the user doesn't wait through TUI startup or a useless
			// pipeline run just to discover the dependency is missing.
			if err := preflightClaude(); err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			if opts.noTUI || opts.yes {
				return runInitHeadless(cmd.Context(), cmd.OutOrStdout(), cwd, opts)
			}
			return tui.Run(tui.Options{
				Version:            version.String(),
				RepoRoot:           cwd,
				SimulateApplyDelay: opts.simulateApplyDelay,
			})
		},
	}

	cmd.Flags().StringVarP(&opts.profile, "profile", "p", "", "minimal | recommended | full (required with --yes / --no-tui / non-TTY stdout)")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "skip the TUI and apply the chosen profile non-interactively")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print the plan but don't write anything")
	cmd.Flags().BoolVar(&opts.noTUI, "no-tui", false, "run without the interactive TUI (CI / piped stdout)")
	cmd.Flags().BoolVar(&opts.refreshDetection, "refresh-detection", false, "ignore the cached detection result and re-run claude even if no files changed")
	cmd.Flags().StringSliceVar(&opts.components, "components", nil, "comma-separated component IDs to include (headless only; intersects with profile selection)")
	cmd.Flags().DurationVar(&opts.simulateApplyDelay, "simulate-apply-delay", defaultApplyDelay(), "per-file tick interval on the TUI apply screen")
	_ = cmd.Flags().MarkHidden("simulate-apply-delay")

	_ = cmd.RegisterFlagCompletionFunc("profile",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{"minimal", "recommended", "full"}, cobra.ShellCompDirectiveNoFileComp
		})

	_ = cmd.RegisterFlagCompletionFunc("components",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			ids := component.AllIDs()
			out := make([]string, len(ids))
			for i, id := range ids {
				out[i] = string(id)
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		})

	return cmd
}

func defaultApplyDelay() time.Duration {
	if os.Getenv("CI") != "" {
		return 0
	}
	return 250 * time.Millisecond
}

func (o initOpts) validate() error {
	if (o.yes || o.noTUI) && o.profile == "" {
		return errors.New("--profile is required with --yes or --no-tui (or when stdout is not a TTY)")
	}
	switch o.profile {
	case "", "minimal", "recommended", "full":
	default:
		return fmt.Errorf("invalid --profile %q: want minimal | recommended | full", o.profile)
	}

	if len(o.components) > 0 {
		known := map[component.ID]bool{}
		for _, id := range component.AllIDs() {
			known[id] = true
		}
		for _, raw := range o.components {
			id := component.ID(strings.TrimSpace(raw))
			if id == "" {
				continue
			}
			if !known[id] {
				return fmt.Errorf("unknown --components id %q (try `omc init --help` or shell completion)", id)
			}
		}
	}
	return nil
}

// runInitHeadless is the non-TUI path: drive the whole pipeline via
// session.Run, then print a structured report.
func runInitHeadless(ctx context.Context, w io.Writer, root string, opts initOpts) error {
	sopts := session.Options{
		RepoRoot:           root,
		Profile:            profile.Name(opts.profile),
		DryRun:             opts.dryRun,
		ComponentAllowlist: toComponentIDs(opts.components),
		RefreshDetection:   opts.refreshDetection,
	}
	sopts.Detect.Verbose = func(format string, args ...any) {
		// Fire-and-forget: the verbose stream is informational; a write
		// failure here would surface again on the next write to w.
		_, _ = fmt.Fprintf(w, "  · "+format+"\n", args...)
	}

	res, err := session.Run(ctx, sopts)
	if err != nil {
		return err
	}
	return printHeadlessReport(w, opts, res)
}

func toComponentIDs(raw []string) []component.ID {
	out := make([]component.ID, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, component.ID(s))
	}
	return out
}

func printHeadlessReport(w io.Writer, opts initOpts, res *session.Result) error {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "omc init (headless): profile=%s dry-run=%v\n\n", opts.profile, opts.dryRun)

	if res.Stack.Type == "" {
		fmt.Fprintln(&buf, "no Stack detected — nothing to plan")
		_, werr := buf.WriteTo(w)
		return werr
	}

	fmt.Fprintf(&buf, "detected: %s\n", report.StackSummary(res.Stack))
	if res.Detection != nil {
		switch {
		case res.Detection.FromCache:
			fmt.Fprintln(&buf, "  (cached — no claude call needed)")
		case res.Detection.Refreshed && len(res.Detection.Changed) > 0:
			fmt.Fprintf(&buf, "  (re-detected — %d file(s) changed since last run)\n", len(res.Detection.Changed))
		}
	}
	if res.Snapshot != nil && res.Snapshot.Truncated {
		fmt.Fprintf(&buf, "  (file walk truncated at %d entries — some files were not shown to claude)\n", len(res.Snapshot.Files))
	}
	for _, line := range report.StackLines(res.Stack) {
		fmt.Fprintln(&buf, "  "+line)
	}

	if res.Plan == nil || len(res.Plan.Writes)+len(res.Plan.Skips) == 0 {
		fmt.Fprintln(&buf, "\nno components selected for this profile + Stack")
		_, werr := buf.WriteTo(w)
		return werr
	}

	fmt.Fprintf(&buf, "\nselected %d / %d applicable components for profile=%s:\n",
		len(res.Selected), len(res.Catalog), opts.profile)
	for _, c := range res.Catalog {
		mark := " "
		if res.Selected[c.ID] {
			mark = "x"
		}
		fmt.Fprintf(&buf, "  [%s] %-22s  %s\n", mark, c.ID, c.Title)
	}

	fmt.Fprintln(&buf, "\nplan:")
	for _, fw := range res.Plan.Writes {
		fmt.Fprintf(&buf, "  + %-8s %s (%d bytes)\n", fw.Action.String(), fw.Path, len(fw.Body))
	}
	for _, sk := range res.Plan.Skips {
		fmt.Fprintf(&buf, "  - skipped  %s (%s)\n", sk.Path, sk.Reason)
	}

	if len(res.WriteResults) > 0 {
		fmt.Fprintln(&buf, "\nsimulated apply:")
		for _, r := range res.WriteResults {
			tag := r.Status
			if r.Err != nil {
				tag = "error: " + r.Err.Error()
			}
			fmt.Fprintf(&buf, "  · %-12s %s (%d bytes)\n", tag, r.Path, r.Bytes)
		}
	}

	fmt.Fprintln(&buf, "\npreview build — no files were written.")
	fmt.Fprintln(&buf, "Real apply pipeline + per-Type templates land in M5/M6.")

	_, werr := buf.WriteTo(w)
	return werr
}

// preflightClaude is the hard-dep gate for `omc init`. omc's detection
// is LLM-backed — without a working claude CLI authenticated against a
// subscription, the pipeline cannot proceed. Errors are intentionally
// verbose: each one names the install / login command the user needs.
func preflightClaude() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return errors.New(
			"omc requires the claude CLI for project detection, but `claude` was not found on $PATH.\n" +
				"  install: https://docs.claude.com/en/docs/claude-code\n" +
				"  (omc never falls back to ANTHROPIC_API_KEY billing — it uses your existing subscription)",
		)
	}
	// `claude auth status` exits non-zero when there is no active subscription session.
	// #nosec G204 -- fixed args, no user input
	if err := exec.Command("claude", "auth", "status").Run(); err != nil {
		return errors.New(
			"omc requires an authenticated claude subscription session.\n" +
				"  fix: `claude auth login`",
		)
	}
	return nil
}

// guard against unused imports when the file is edited mid-refactor
var _ = apply.ActionCreate
