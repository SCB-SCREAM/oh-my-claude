package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SCB-SCREAM/oh-my-claude/internal/doctor"
	"github.com/SCB-SCREAM/oh-my-claude/internal/version"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the local environment for common Claude Code setup issues",
		Long: `doctor inspects the current project for things that commonly break a
Claude Code setup: missing claude CLI, .claude/settings.local.json not in
.gitignore, and so on.

Read-only: always exits 0 unless invoked incorrectly. Findings are reported
on stdout so it's safe to pipe into scripts.`,
		Example: `  omc doctor`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			return runDoctor(cmd.OutOrStdout(), cwd)
		},
	}
}

func runDoctor(w io.Writer, root string) error {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "omc %s\n", version.String())
	fmt.Fprintf(&buf, "scanning: %s\n\n", root)

	findings := doctor.Run(root)
	indent := strings.Repeat(" ", 30)
	for _, f := range findings {
		fmt.Fprintf(&buf, "  %s  %-30s %s\n", f.Severity.Symbol(), f.Name, f.Message)
		if f.Hint != "" {
			fmt.Fprintf(&buf, "     %s↳ %s\n", indent, f.Hint)
		}
	}
	buf.WriteByte('\n')

	var errs, warns int
	for _, f := range findings {
		switch f.Severity {
		case doctor.SeverityError:
			errs++
		case doctor.SeverityWarn:
			warns++
		}
	}
	switch {
	case errs > 0:
		fmt.Fprintf(&buf, "%d error, %d warning\n", errs, warns)
	case warns > 0:
		fmt.Fprintf(&buf, "no errors, %d warning\n", warns)
	default:
		fmt.Fprintln(&buf, "all checks passed")
	}

	_, err := buf.WriteTo(w)
	return err
}
