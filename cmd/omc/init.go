package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
	"github.com/SCB-SCREAM/oh-my-claude/internal/tui"
	"github.com/SCB-SCREAM/oh-my-claude/internal/version"
)

type initOpts struct {
	profile string
	yes     bool
	dryRun  bool
	noTUI   bool
}

func newInitCmd() *cobra.Command {
	var opts initOpts
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Detect this project's stack and bootstrap a Claude Code setup",
		Long: `init scans the current directory, asks for a profile, and writes
CLAUDE.md, .claude/settings.json, hooks, slash commands, and subagent stubs.

In M3 the headless path (` + "`--no-tui`" + ` / ` + "`--yes`" + `) prints the detected stack
signals so users can verify what omc found before the apply pipeline lands
in M5. The TUI flow ships in M4.`,
		Example: `  omc init
  omc init --profile recommended --yes
  omc init --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := opts.validate(); err != nil {
				return err
			}
			if opts.noTUI || opts.yes {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("get working directory: %w", err)
				}
				return runInitHeadless(cmd.OutOrStdout(), cwd, opts)
			}
			return tui.Run(version.String())
		},
	}

	cmd.Flags().StringVarP(&opts.profile, "profile", "p", "", "minimal | recommended | full (required with --yes / --no-tui)")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "skip the TUI and apply the chosen profile non-interactively")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print the plan but don't write anything")
	cmd.Flags().BoolVar(&opts.noTUI, "no-tui", false, "run without the interactive TUI (CI / piped stdout)")

	_ = cmd.RegisterFlagCompletionFunc("profile",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{"minimal", "recommended", "full"}, cobra.ShellCompDirectiveNoFileComp
		})

	return cmd
}

func (o initOpts) validate() error {
	if (o.yes || o.noTUI) && o.profile == "" {
		return errors.New("--profile is required with --yes or --no-tui")
	}
	switch o.profile {
	case "", "minimal", "recommended", "full":
	default:
		return fmt.Errorf("invalid --profile %q: want minimal | recommended | full", o.profile)
	}
	return nil
}

// runInitHeadless is the non-TUI path. M3: walk the cwd, run every
// detector, and print the resulting signals so users (and CI dry-runs)
// can see what omc detected before the apply pipeline lands in M5.
func runInitHeadless(w io.Writer, root string, opts initOpts) error {
	signals, snap, err := detect.Run(os.DirFS(root))
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "omc init (headless): profile=%s dry-run=%v\n\n", opts.profile, opts.dryRun)

	if len(signals) == 0 {
		fmt.Fprintln(&buf, "no stack signals detected — nothing to apply yet")
		_, werr := buf.WriteTo(w)
		return werr
	}

	fmt.Fprintf(&buf, "detected %d signal(s) in %s:\n", len(signals), root)
	if snap.Truncated {
		fmt.Fprintf(&buf, "  (file walk truncated at %d entries — some signals may be missing)\n", len(snap.Files))
	}
	for _, s := range signals {
		fmt.Fprintf(&buf, "  %s  %-22s  %s  %s\n",
			confidenceMark(s.Confidence),
			s.Name,
			tagsInline(s.Tags),
			evidenceInline(s.Evidence),
		)
	}
	fmt.Fprintln(&buf, "\napply pipeline lands in M5 — re-run after upgrading once that ships.")
	_, werr := buf.WriteTo(w)
	return werr
}

// confidenceMark renders a confidence value as a four-pip glyph so the
// strongest signals stand out without colour.
func confidenceMark(c float64) string {
	switch {
	case c >= detect.ConfLockfile:
		return "●●●●"
	case c >= detect.ConfManifest:
		return "●●●○"
	case c >= detect.ConfDepDeclared:
		return "●●○○"
	case c >= detect.ConfFileConvention:
		return "●○○○"
	default:
		return "○○○○"
	}
}

func tagsInline(tags []string) string {
	if len(tags) == 0 {
		return "[-]"
	}
	return "[" + strings.Join(tags, ",") + "]"
}

func evidenceInline(ev []string) string {
	const maxShow = 3
	if len(ev) <= maxShow {
		return strings.Join(ev, ", ")
	}
	return strings.Join(ev[:maxShow], ", ") + fmt.Sprintf(" (+%d more)", len(ev)-maxShow)
}
