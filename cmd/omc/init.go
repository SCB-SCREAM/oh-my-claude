package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

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

In M1 only the welcome screen is wired up; pressing [enter] exits cleanly.
Detection, profile selection, and apply are delivered in later milestones.`,
		Example: `  omc init
  omc init --profile recommended --yes
  omc init --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := opts.validate(); err != nil {
				return err
			}
			if opts.noTUI || opts.yes {
				return runInitHeadless(opts)
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

// runInitHeadless is the non-TUI path. M1 stub: prints what it *would* do.
func runInitHeadless(opts initOpts) error {
	fmt.Printf("omc init (headless): profile=%s dry-run=%v\n", opts.profile, opts.dryRun)
	fmt.Println("detection + apply not yet implemented (milestones M2-M4)")
	return nil
}
