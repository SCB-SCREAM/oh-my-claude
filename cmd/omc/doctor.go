package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the local environment for common Claude Code setup issues",
		Long: `doctor inspects the current project for things that commonly break a
Claude Code setup: missing claude CLI, .claude/settings.local.json not in
.gitignore, broken hook scripts, drifted template versions.

Read-only: always exits 0 unless invoked incorrectly. Findings are reported
on stdout so it's safe to pipe into scripts.`,
		Example: `  omc doctor`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("omc doctor: not yet implemented (milestone M2)")
			return nil
		},
	}
}
