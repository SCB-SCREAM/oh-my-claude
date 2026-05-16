// Package component models one renderable unit omc would install — a
// CLAUDE.md, a settings.json patch, a slash-command file, a hook script.
//
// Components self-register via init() in per-type files (webapp.go,
// cli.go, …). Each component declares an [AppliesTo] predicate against
// the detected [detect.Stack] — typically a Type switch
// (`stack.Type == detect.TypeCLI`) or a field probe
// (`stack.Formatter != ""`). Nothing in this package hardcodes language
// names; the substrate stays open for new project types the LLM may
// classify into.
//
// The signal-set gating of older versions is gone: components see the
// structured Stack, not a heuristic-emitted name list. Adding a new
// project Type ([detect.Type]) means writing one component file here +
// one template overlay under internal/templates/.
package component

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

// ID is a stable, kebab-case identifier ("claude-md", "cmd.test").
// IDs appear in profile tables, in --components allowlists, in tests,
// and in user-facing docs — never change one without a migration plan.
type ID string

// Category groups components in the TUI checklist and in --help.
type Category string

// Canonical category values used by registered components. The TUI's
// components screen groups under these headings in declaration order.
const (
	CategoryClaudeMD  Category = "CLAUDE.md"
	CategorySettings  Category = "Settings"
	CategoryCommand   Category = "Slash command"
	CategoryHook      Category = "Hook"
	CategoryAgent     Category = "Subagent"
	CategoryMCP       Category = "MCP server"
	CategoryGitignore Category = ".gitignore"
)

// ConflictPolicy is a hint surfaced to the user in the preview screen
// and enforced by apply.Execute.
type ConflictPolicy int

// Conflict-handling policies a Component declares for its target files.
// apply.BuildPlan maps these onto Action values.
const (
	ConflictSkipIfExists ConflictPolicy = iota
	ConflictMerge
	ConflictOverwritePrompt
)

// TargetFile is one rendered file a component would write. Path is
// repo-relative, slash-separated.
type TargetFile struct {
	Path string
	Body []byte
	Mode os.FileMode
}

// Component is the unit of "one thing omc would install."
type Component struct {
	ID          ID
	Title       string
	Description string
	Category    Category

	// AppliesTo gates the component on the detected [detect.Stack].
	// Return true to include this component in the materialized catalog.
	// nil is treated as "always applies" (e.g. CLAUDE.md).
	AppliesTo func(detect.Stack) bool

	Files    []TargetFile
	Conflict ConflictPolicy
}

// Catalog returns every registered component, sorted by Category then
// ID for deterministic golden-file tests. Returns a fresh slice each
// call so callers can sort/filter without disturbing the registry.
func Catalog() []Component {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]Component, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Materialize is the AppliesTo-gated subset of Catalog() for the given
// Stack. A component with nil AppliesTo always applies.
func Materialize(stack detect.Stack) []Component {
	cat := Catalog()
	out := make([]Component, 0, len(cat))
	for _, c := range cat {
		if c.AppliesTo == nil || c.AppliesTo(stack) {
			out = append(out, c)
		}
	}
	return out
}

// ByID looks up a component by its ID. Returns the zero value + false
// if no such component is registered.
func ByID(id ID) (Component, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	c, ok := registry[id]
	return c, ok
}

// AllIDs returns every registered component ID. Used by the
// --components shell completion in cmd/omc/init.go.
func AllIDs() []ID {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]ID, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// --- registry ---

var (
	registryMu sync.RWMutex
	registry   = map[ID]Component{}
)

// register adds a Component to the catalog. Duplicate IDs panic at
// init() time so we catch the conflict on the first test run.
func register(c Component) {
	if c.ID == "" {
		panic("component.register: empty ID")
	}
	if c.Title == "" {
		panic(fmt.Sprintf("component.register(%q): empty Title", c.ID))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[c.ID]; dup {
		panic(fmt.Sprintf("component.register: duplicate ID %q", c.ID))
	}
	registry[c.ID] = c
}

// --- placeholder helpers ---

// previewBanner is the preview-only prelude prepended to every component
// body until templates lands in M6. Users see this in the preview pane;
// it makes clear that real templates are still pending.
const previewBanner = "# omc preview — final template lands in M6\n" +
	"# Edit this file freely; nothing was actually written to disk.\n\n"

// placeholderBody wraps a short body with the preview banner.
func placeholderBody(body string) []byte {
	return []byte(previewBanner + strings.TrimLeft(body, "\n"))
}
