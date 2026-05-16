// Package report renders a [detect.Stack] for human consumption — used
// by both the headless `omc init` printer and the TUI scan screen so the
// two render the same content.
package report

import (
	"fmt"
	"strings"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

// StackLines renders the Stack as a short list of "key: value" lines
// suitable for stacking under a "detected:" header. Empty fields are
// omitted entirely — a sparse Stack produces a sparse listing.
func StackLines(s detect.Stack) []string {
	var out []string
	add := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		out = append(out, fmt.Sprintf("%-18s %s", label+":", value))
	}
	add("type", string(s.Type))
	add("language", s.LanguagePrimary)
	add("package manager", s.PackageManager)
	add("build", s.BuildCmd)
	add("test", s.TestCmd)
	add("lint", s.LintCmd)
	add("formatter", s.Formatter)
	if len(s.Frameworks) > 0 {
		add("frameworks", strings.Join(s.Frameworks, ", "))
	}
	if s.Notes != "" {
		add("notes", s.Notes)
	}
	return out
}

// StackSummary returns a single-line "type · language · framework" tag
// for use in compact headers (e.g. the profile screen title).
func StackSummary(s detect.Stack) string {
	parts := []string{string(s.Type)}
	if s.LanguagePrimary != "" {
		parts = append(parts, s.LanguagePrimary)
	}
	if len(s.Frameworks) > 0 {
		parts = append(parts, s.Frameworks[0])
	}
	return strings.Join(parts, " · ")
}
