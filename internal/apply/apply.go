// Package apply turns a selected set of components into a flat Plan and
// (in M5) executes that plan against disk with backups, deep-merge, and
// unified diffs.
//
// In M4 this package is a deliberate stub: BuildPlan flattens the
// selection into FileWrites and tags each one ActionCreate (no disk
// inspection, no merge detection), Execute simulates writes without
// touching disk, RenderDiff prints the would-be content with a "+ "
// prefix instead of computing a real diff. Every Plan created here
// carries Stub=true; Execute refuses to run on a non-stub Plan so
// nothing M5 builds can accidentally leak through M4 callsites.
package apply

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
)

// Action describes what apply.Execute would do to the target path.
type Action int

const (
	ActionCreate    Action = iota // file does not exist
	ActionOverwrite               // file exists; component.Conflict says overwrite
	ActionMerge                   // file exists; M5 will deep-merge (e.g. settings.json)
	ActionSkip                    // user toggled this off in preview, or skip-if-exists policy hit
)

// String returns a short tag suitable for the preview screen's file
// list ("new", "overwrite", "merge", "skip").
func (a Action) String() string {
	switch a {
	case ActionCreate:
		return "new"
	case ActionOverwrite:
		return "overwrite"
	case ActionMerge:
		return "merge"
	case ActionSkip:
		return "skip"
	}
	return "?"
}

// FileWrite is one planned change.
type FileWrite struct {
	Path     string // repo-relative, slash-separated
	Body     []byte // bytes that would be written (placeholder content in M4)
	Mode     uint32 // file mode (lowered from os.FileMode for plain serialization)
	Action   Action // create / overwrite / merge / skip
	Existing bool   // true if the path already exists on disk; always false in M4 (no FS probing)
	OwnerID  string // component.ID that produced this write (debug + traceability)
}

// Skip is a write that was elided before Execute ran — either because
// the user toggled it off in the preview screen, or because BuildOpts
// flagged it.
type Skip struct {
	Path   string
	Reason string
}

// WriteResult is one result emitted by Execute (one per FileWrite, in
// order). In M4 every Status is "would-write".
type WriteResult struct {
	Path   string
	Status string
	Bytes  int
	Err    error
}

// Plan is the materialized list of changes for a selection. The DryRun
// and Stub flags are inspected by Execute as a safety belt.
type Plan struct {
	RepoRoot string
	Writes   []FileWrite
	Skips    []Skip
	Backups  []string // M4: nil; M5 will populate with .claude/.omc-backup-* paths

	// DryRun: when true, Execute reports what it *would* do without
	// performing real writes. In M4 every Plan is dry-run regardless,
	// because Stub is always true.
	DryRun bool

	// Stub: M4-only marker. Always true for plans this package
	// produces. Execute returns an error on a non-stub Plan — keeps
	// half-finished M5 work from accidentally writing through M4
	// callsites before the real writer lands.
	Stub bool
}

// BuildOpts tweaks plan construction.
type BuildOpts struct {
	DryRun bool

	// SkipPaths, when non-empty, marks the listed repo-relative paths
	// as ActionSkip (and moves them to plan.Skips). Used by the preview
	// screen's `s` toggle and by --components allow/deny flows.
	SkipPaths map[string]bool
}

// BuildPlan flattens the selected components into a Plan. selected is a
// map[component.ID]bool consumed verbatim (true => include); IDs not in
// the map are excluded.
//
// repoRoot is captured on the Plan for the preview-pane title and (in
// M5) for the writer to resolve absolute paths. M4 does not validate
// that repoRoot exists.
func BuildPlan(repoRoot string, components []component.Component, selected map[component.ID]bool, opts BuildOpts) (*Plan, error) {
	if selected == nil {
		selected = map[component.ID]bool{}
	}

	plan := &Plan{
		RepoRoot: repoRoot,
		DryRun:   opts.DryRun,
		Stub:     true,
	}

	for _, c := range components {
		if !selected[c.ID] {
			continue
		}
		for _, f := range c.Files {
			cleaned := path.Clean(f.Path)
			if cleaned != f.Path || strings.HasPrefix(cleaned, "..") || strings.HasPrefix(cleaned, "/") {
				return nil, fmt.Errorf("apply: component %q produced unsafe path %q", c.ID, f.Path)
			}

			if opts.SkipPaths[cleaned] {
				plan.Skips = append(plan.Skips, Skip{
					Path:   cleaned,
					Reason: "user-skipped in preview",
				})
				continue
			}

			plan.Writes = append(plan.Writes, FileWrite{
				Path:    cleaned,
				Body:    f.Body,
				Mode:    uint32(f.Mode),
				Action:  actionFor(c.Conflict),
				OwnerID: string(c.ID),
			})
		}
	}

	// Deterministic order: by Path. Components with overlapping output
	// paths (a TS and a Python /test, say) will collide here — the
	// first wins. M5 will turn this into an explicit conflict policy.
	sort.SliceStable(plan.Writes, func(i, j int) bool {
		return plan.Writes[i].Path < plan.Writes[j].Path
	})
	sort.SliceStable(plan.Skips, func(i, j int) bool {
		return plan.Skips[i].Path < plan.Skips[j].Path
	})

	return plan, nil
}

// actionFor maps a ConflictPolicy to the M4 Action. Without disk-probe
// data, "merge" and "overwrite" remain hints — M5 will compare against
// the existing file to pick one of {create, merge, overwrite-prompt}.
func actionFor(p component.ConflictPolicy) Action {
	switch p {
	case component.ConflictMerge:
		return ActionMerge
	case component.ConflictOverwritePrompt:
		return ActionOverwrite
	}
	return ActionCreate
}

// ErrNotStub is returned by Execute when called with a non-stub Plan.
// Until the M5 real executor lands, every Plan must be stub-flagged.
var ErrNotStub = errors.New("apply: real executor not implemented yet (M5)")

// Execute is M4's stub: returns WriteResults without touching disk.
// Each result is tagged Status="would-write".
//
// The Stub-flag guard at the top of this function is intentional: it
// will fire the moment any caller (or future M5 work-in-progress)
// builds a non-stub Plan, ensuring real writes can't slip through
// before the real executor and its tests land.
func Execute(_ context.Context, plan *Plan) ([]WriteResult, error) {
	if plan == nil {
		return nil, errors.New("apply: nil plan")
	}
	if !plan.Stub {
		return nil, ErrNotStub
	}

	results := make([]WriteResult, 0, len(plan.Writes))
	for _, w := range plan.Writes {
		results = append(results, WriteResult{
			Path:   w.Path,
			Status: "would-write",
			Bytes:  len(w.Body),
		})
	}
	return results, nil
}

// RenderDiff is M4's primitive for the preview pane: for ActionCreate
// or ActionOverwrite it prefixes every body line with "+ "; for
// ActionMerge it prints a placeholder banner ("M5 will diff against
// existing"); for ActionSkip it returns the skip explanation.
//
// M5 swaps this in for go-diff unified diffs.
func RenderDiff(w FileWrite) string {
	var b strings.Builder

	switch w.Action {
	case ActionSkip:
		b.WriteString("(skipped — preview only)\n")
		return b.String()
	case ActionMerge:
		fmt.Fprintf(&b, "[M5 will diff this against the existing %s]\n\n", w.Path)
	}

	lines := strings.Split(string(w.Body), "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			// trim trailing empty line so the pane doesn't have a blank tail
			continue
		}
		b.WriteString("+ ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
