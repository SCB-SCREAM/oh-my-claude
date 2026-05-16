// Package session owns the canonical pipeline: detect → resolve
// (profile + Stack-gated catalog) → build plan → execute (stub).
//
// Both cmd/omc/init.go (headless) and internal/tui/app.go (interactive)
// call into this package so neither duplicates the wiring. The TUI uses
// Detect + ResolveAndMaterialize + BuildPlan + Execute as discrete
// steps wrapped in tea.Cmds; the headless path uses the Run convenience.
//
// The legacy []detect.Signal pipeline is gone — detection produces a
// single structured [detect.Stack] from the LLM-backed detector, and
// every downstream step keys off Stack.
package session

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
	"github.com/SCB-SCREAM/oh-my-claude/internal/profile"
)

// Options bundles everything both the TUI and headless paths need.
// RepoRoot or RepoFS is required (RepoFS wins when both are set —
// useful for tests).
type Options struct {
	RepoRoot string
	RepoFS   fs.FS

	Profile profile.Name
	DryRun  bool

	// ComponentAllowlist, when non-empty, intersects with the profile's
	// selection so only listed IDs are pre-checked. Used by the
	// headless --components flag. Unknown IDs are silently dropped at
	// this layer (cmd/omc validates them up front).
	ComponentAllowlist []component.ID

	// SkipFiles marks repo-relative paths as Skip in the plan. Threaded
	// by the preview screen's `s` toggle.
	SkipFiles map[string]bool

	// Detect bundles options forwarded to detect.Run — primarily the
	// LLM Runner seam (overridden by tests) and Verbose plumbing.
	// CachePath, when zero, defaults to
	// [detect.DefaultCachePath](RepoRoot).
	Detect detect.Options

	// RefreshDetection bypasses the cache read on this run. Wired to
	// `omc init --refresh-detection`.
	RefreshDetection bool
}

// Result is the materialized end-state of Run. The TUI flow builds it
// up screen-by-screen; the headless path fills it in one shot.
type Result struct {
	Stack        detect.Stack
	Detection    *detect.Result
	Snapshot     *detect.Snapshot
	Catalog      []component.Component
	Selected     map[component.ID]bool
	Plan         *apply.Plan
	WriteResults []apply.WriteResult
}

func (o Options) fsys() fs.FS {
	if o.RepoFS != nil {
		return o.RepoFS
	}
	if o.RepoRoot == "" {
		return os.DirFS(".")
	}
	return os.DirFS(o.RepoRoot)
}

func (o Options) cachePath() string {
	if o.Detect.CachePath != "" {
		return o.Detect.CachePath
	}
	if o.RepoFS != nil && o.RepoRoot == "" {
		// In-memory FS test runs: skip the cache by default.
		return ""
	}
	return detect.DefaultCachePath(o.RepoRoot)
}

// Detect runs the LLM-backed classifier (with project-local cache) and
// returns the structured Stack plus the underlying [detect.Result] so
// callers can inspect cache state (FromCache / Refreshed / Changed).
func Detect(ctx context.Context, opts Options) (*detect.Result, error) {
	detectOpts := opts.Detect
	if opts.RefreshDetection {
		detectOpts.SkipCache = true
	}
	res, err := detect.Run(ctx, opts.fsys(), opts.cachePath(), detectOpts)
	if err != nil {
		return nil, fmt.Errorf("session.Detect: %w", err)
	}
	return res, nil
}

// ResolveAndMaterialize intersects the profile's baseline component
// list with the Stack-gated catalog and the caller's ComponentAllowlist.
//
// Returns the materialized component list (what the components screen
// renders), and the pre-checked selection map (what the user toggles
// before BuildPlan).
func ResolveAndMaterialize(opts Options, stack detect.Stack) ([]component.Component, map[component.ID]bool, error) {
	if err := profile.Validate(opts.Profile); err != nil {
		return nil, nil, fmt.Errorf("session: %w (got %q)", err, opts.Profile)
	}

	catalog := component.Materialize(stack)

	ids, err := profile.IDs(opts.Profile)
	if err != nil {
		return nil, nil, fmt.Errorf("session: %w", err)
	}
	wantedByProfile := make(map[component.ID]bool, len(ids))
	for _, id := range ids {
		wantedByProfile[component.ID(id)] = true
	}

	var allow map[component.ID]bool
	if len(opts.ComponentAllowlist) > 0 {
		allow = make(map[component.ID]bool, len(opts.ComponentAllowlist))
		for _, id := range opts.ComponentAllowlist {
			allow[id] = true
		}
	}

	selected := make(map[component.ID]bool, len(catalog))
	for _, c := range catalog {
		if !wantedByProfile[c.ID] {
			continue
		}
		if allow != nil && !allow[c.ID] {
			continue
		}
		selected[c.ID] = true
	}

	return catalog, selected, nil
}

// BuildPlan flattens the user-toggled selection into an apply.Plan.
// Pure delegation today; kept here so the TUI plumbs through a single
// package.
func BuildPlan(opts Options, comps []component.Component, selected map[component.ID]bool) (*apply.Plan, error) {
	return apply.BuildPlan(opts.RepoRoot, comps, selected, apply.BuildOpts{
		DryRun:    opts.DryRun,
		SkipPaths: opts.SkipFiles,
	})
}

// Execute is a thin wrapper over apply.Execute.
func Execute(ctx context.Context, plan *apply.Plan) ([]apply.WriteResult, error) {
	return apply.Execute(ctx, plan)
}

// Run is the convenience method for the --yes / --no-tui headless
// path: it walks every step and returns the assembled Result. The TUI
// never calls Run — it uses the discrete steps so each screen can drive
// its own spinner / progress feedback.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.RepoRoot == "" && opts.RepoFS == nil {
		return nil, errors.New("session.Run: RepoRoot or RepoFS required")
	}

	det, err := Detect(ctx, opts)
	if err != nil {
		return nil, err
	}

	res := &Result{Stack: det.Stack, Detection: det, Snapshot: det.Snapshot}

	catalog, selected, err := ResolveAndMaterialize(opts, det.Stack)
	if err != nil {
		return nil, err
	}
	res.Catalog = catalog
	res.Selected = selected

	plan, err := BuildPlan(opts, catalog, selected)
	if err != nil {
		return nil, err
	}
	res.Plan = plan

	results, err := Execute(ctx, plan)
	if err != nil {
		return nil, err
	}
	res.WriteResults = results

	return res, nil
}
