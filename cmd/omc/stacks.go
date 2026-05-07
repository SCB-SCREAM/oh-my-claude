package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

func newStacksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stacks",
		Short: "List supported stacks and what triggers each detector",
		Long: `stacks prints every detector omc ships, grouped by category
(language, package-manager, framework, infra, ci, monorepo, …) along with
the file patterns that trigger them and the confidence range each can
emit. Useful for answering "would omc detect my project?" without
running ` + "`omc init`" + `.`,
		Example: `  omc stacks`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStacks(cmd.OutOrStdout())
		},
	}
}

func runStacks(w io.Writer) error {
	groups := detect.GroupedCatalog()
	if len(groups) == 0 {
		_, err := fmt.Fprintln(w, "no detectors registered")
		return err
	}

	for i, g := range groups {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%s\n", strings.ToUpper(g.Tag)); err != nil {
			return err
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, e := range g.Entries {
			conf := fmt.Sprintf("%.2f-%.2f", e.MinConf, e.MaxConf)
			if e.MinConf == e.MaxConf {
				conf = fmt.Sprintf("%.2f", e.MinConf)
			}
			triggers := strings.Join(e.Triggers, ", ")
			if _, err := fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", e.ID, conf, triggers, e.Summary); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}
