package component

import (
	"fmt"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

// Generic, Stack-driven slash commands. Each one gates on the presence
// of the corresponding command in the detected Stack — so a project
// without a build_cmd gets no /build, and a Rust project's /test runs
// `cargo test` because the LLM filled TestCmd accordingly.
//
// This replaces the older typescript.go / python.go / golang.go files
// that hardcoded one component per language. New languages now get full
// /test|/lint|/format support without touching this package.

func init() {
	register(Component{
		ID:          "cmd.test",
		Title:       "/test slash command",
		Description: "Runs the test command declared by the detected stack (e.g. `go test`, `pnpm test`, `pytest`).",
		Category:    CategoryCommand,
		AppliesTo:   func(s detect.Stack) bool { return s.TestCmd != "" },
		Conflict:    ConflictSkipIfExists,
		Files: []TargetFile{{
			Path: ".claude/commands/test.md",
			Mode: 0o644,
			Body: placeholderBody(commandTemplate("test", "Run the project's test suite.")),
		}},
	})

	register(Component{
		ID:          "cmd.lint",
		Title:       "/lint slash command",
		Description: "Runs the lint command declared by the detected stack.",
		Category:    CategoryCommand,
		AppliesTo:   func(s detect.Stack) bool { return s.LintCmd != "" },
		Conflict:    ConflictSkipIfExists,
		Files: []TargetFile{{
			Path: ".claude/commands/lint.md",
			Mode: 0o644,
			Body: placeholderBody(commandTemplate("lint", "Run the project's linter.")),
		}},
	})

	register(Component{
		ID:          "cmd.format",
		Title:       "/format slash command",
		Description: "Runs the formatter declared by the detected stack.",
		Category:    CategoryCommand,
		AppliesTo:   func(s detect.Stack) bool { return s.Formatter != "" },
		Conflict:    ConflictSkipIfExists,
		Files: []TargetFile{{
			Path: ".claude/commands/format.md",
			Mode: 0o644,
			Body: placeholderBody(commandTemplate("format", "Format the codebase using the project's canonical formatter.")),
		}},
	})

	register(Component{
		ID:          "cmd.build",
		Title:       "/build slash command",
		Description: "Runs the build command declared by the detected stack.",
		Category:    CategoryCommand,
		AppliesTo:   func(s detect.Stack) bool { return s.BuildCmd != "" },
		Conflict:    ConflictSkipIfExists,
		Files: []TargetFile{{
			Path: ".claude/commands/build.md",
			Mode: 0o644,
			Body: placeholderBody(commandTemplate("build", "Build the project.")),
		}},
	})
}

// commandTemplate renders the front-matter + body for a slash-command
// markdown file. The real M6 templates substitute the detected Stack's
// command into the body; this preview leaves a placeholder a human can
// fill manually.
func commandTemplate(name, summary string) string {
	return fmt.Sprintf(
		"---\nname: %s\ndescription: %s\n---\n\nM6 will substitute the detected `%s_cmd` from the project's Stack here.\n",
		name, summary, name,
	)
}
