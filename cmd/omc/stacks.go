package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

func newStacksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stacks",
		Short: "List the project Types omc can classify into",
		Long: `stacks prints every project [Type] the LLM-backed detector is
constrained to choose from. Detection itself is open-ended — claude
classifies your project from its file paths and emits a structured
Stack — but the Type field is a closed enum so downstream components
gate on a small, predictable set.`,
		Example: `  omc stacks`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStacks(cmd.OutOrStdout())
		},
	}
}

func runStacks(w io.Writer) error {
	if _, err := fmt.Fprintln(w, "Project Types omc classifies into:"); err != nil {
		return err
	}
	for _, t := range detect.AllTypes() {
		if _, err := fmt.Fprintf(w, "  %-10s %s\n", t, typeBlurb(t)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\nThe LLM picks one of the above when you run `omc init`."); err != nil {
		return err
	}
	return nil
}

// typeBlurb is a one-line explanation of when each Type applies. Used
// only for human-facing CLI help; downstream gating works off the Type
// enum directly, not these strings.
func typeBlurb(t detect.Type) string {
	switch t {
	case detect.TypeWebApp:
		return "UI-serving applications (frontend or full-stack)"
	case detect.TypeAPI:
		return "HTTP/gRPC services with no UI"
	case detect.TypeCLI:
		return "binary-producing command-line tools"
	case detect.TypeLibrary:
		return "published as a dependency; no main entry point"
	case detect.TypeInfra:
		return "infrastructure-as-code / declarative ops"
	case detect.TypeMonorepo:
		return "multi-package workspaces"
	}
	return ""
}
