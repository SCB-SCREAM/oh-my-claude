// Command omc bootstraps a Claude Code setup for the current project.
//
// This file is intentionally tiny — it builds the Cobra root and wires
// subcommands. All real logic lives under internal/.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/SCB-SCREAM/oh-my-claude/internal/version"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		// Cobra has already printed the error to stderr; just exit non-zero.
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "omc",
		Short: "Bootstrap Claude Code for your project, opinionated by stack",
		Long: `omc detects a project's stack from its files, lets the user pick a profile,
and writes a tailored Claude Code setup (CLAUDE.md, .claude/settings.json,
hooks, slash commands, subagent stubs, MCP suggestions).`,
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	// Make `--version` print just the version string (default Cobra format
	// includes the binary name, which is fine but verbose).
	root.SetVersionTemplate(fmt.Sprintf("omc %s\n", version.String()))

	root.AddCommand(
		newInitCmd(),
		newDoctorCmd(),
		newStacksCmd(),
	)

	return root
}
