package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStacksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stacks",
		Short: "List supported stacks and what each profile installs",
		Long: `stacks prints a table of every stack omc knows about (TypeScript, Python,
Go, …) along with the components each profile (minimal | recommended |
full) ships for that stack.`,
		Example: `  omc stacks`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("omc stacks: not yet implemented (milestone M2)")
			return nil
		},
	}
}
