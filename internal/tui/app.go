// Package tui hosts the Bubble Tea v2 program that drives `omc init`'s
// interactive flow. The root model in this file routes messages to the
// active screen; each screen lives in its own file (welcome, scan,
// profile, components, preview, apply, done).
//
// The root owns a *sessionState pointer, mutated only inside Update.
// Screens emit decision messages (profileChosenMsg, componentsCommittedMsg, …)
// that root copies into sessionState before firing switchScreenMsg.
// Long-running work (detect.Run, apply.Execute, …) flows through tea.Cmds
// owned by the root, so each screen's Init stays trivial.
package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
	"github.com/SCB-SCREAM/oh-my-claude/internal/profile"
	"github.com/SCB-SCREAM/oh-my-claude/internal/session"
)

// Options configures the Bubble Tea program. RepoRoot defaults to "." when
// empty; Version is rendered on the welcome screen; SimulateApplyDelay
// throttles the apply progress animation (set to 0 in CI / tests for
// deterministic teatest snapshots).
type Options struct {
	Version            string
	RepoRoot           string
	SimulateApplyDelay time.Duration
}

// sessionState is the running picture of what the user has chosen so far.
// Only Update mutates it; never mutate from inside a tea.Cmd.
type sessionState struct {
	RepoRoot string
	Version  string

	Stack     detect.Stack
	Detection *detect.Result
	Snapshot  *detect.Snapshot

	Profile profile.Name

	Catalog  []component.Component
	Selected map[component.ID]bool

	Plan      *apply.Plan
	SkipFiles map[string]bool

	Results  []apply.WriteResult
	ApplyErr error
}

type appModel struct {
	state *sessionState
	theme Theme

	current screenID

	welcome    WelcomeModel
	scan       ScanModel
	profile    ProfileModel
	components ComponentsModel
	preview    PreviewModel
	apply      ApplyModel
	done       DoneModel

	width, height int

	// Hooks for test injection. Real implementations defer to the
	// session package; tests substitute fakes that synthesize result
	// messages instantly.
	detectCmd    func(string) tea.Cmd
	resolveCmd   func(profile.Name, detect.Stack) tea.Cmd
	buildPlanCmd func(string, []component.Component, map[component.ID]bool, map[string]bool) tea.Cmd
	applyCmd     func(*apply.Plan, time.Duration) tea.Cmd

	// applyDelay is captured from Options at construction time.
	applyDelay time.Duration
}

// New constructs the root model. Exposed so tests can wrap it in
// teatest.NewTestModel; production callers use Run.
func New(opts Options) tea.Model {
	theme := NewTheme()
	if opts.RepoRoot == "" {
		opts.RepoRoot = "."
	}
	state := &sessionState{
		RepoRoot:  opts.RepoRoot,
		Version:   opts.Version,
		Selected:  map[component.ID]bool{},
		SkipFiles: map[string]bool{},
	}
	return appModel{
		state:        state,
		theme:        theme,
		current:      welcomeScreen,
		welcome:      NewWelcome(theme, opts.Version),
		scan:         NewScan(theme, opts.RepoRoot),
		profile:      NewProfile(theme),
		components:   NewComponents(theme),
		preview:      NewPreview(theme),
		apply:        NewApply(theme),
		done:         NewDone(theme),
		applyDelay:   opts.SimulateApplyDelay,
		detectCmd:    defaultDetectCmd,
		resolveCmd:   defaultResolveCmd,
		buildPlanCmd: defaultBuildPlanCmd,
		applyCmd:     defaultApplyCmd,
	}
}

// Run starts the Bubble Tea program with the given Options against
// stdout/stdin. Returns the error from program.Run, or nil on clean exit.
func Run(opts Options) error {
	p := tea.NewProgram(New(opts))
	_, err := p.Run()
	return err
}

// Init delegates to the welcome screen — that's what the user sees first.
func (m appModel) Init() tea.Cmd { return m.welcome.Init() }

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global resize: broadcast to every child so layout caches update.
	if w, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = w.Width, w.Height
		var cmds []tea.Cmd
		m.welcome, _ = m.welcome.Update(w)
		var c tea.Cmd
		m.scan, c = m.scan.Update(w)
		cmds = append(cmds, c)
		m.profile, _ = m.profile.Update(w)
		m.components, _ = m.components.Update(w)
		m.preview, _ = m.preview.Update(w)
		m.apply, _ = m.apply.Update(w)
		m.done, _ = m.done.Update(w)
		return m, tea.Batch(cmds...)
	}

	// Global Ctrl+C — always quit. (Screen-local 'q' bindings vary.)
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Cross-screen orchestration messages.
	switch msg := msg.(type) {
	case switchScreenMsg:
		return m.handleScreenSwitch(msg.To)

	case stackMsg:
		if msg.Err == nil {
			m.state.Stack = msg.Stack
			m.state.Detection = msg.Detection
			m.state.Snapshot = msg.Snapshot
		}
		// Forward to the scan screen so it can flip to its "done" state.
		var cmd tea.Cmd
		m.scan, cmd = m.scan.Update(msg)
		return m, cmd

	case profileChosenMsg:
		m.state.Profile = profile.Name(msg.Profile)
		return m, switchTo(componentsScreen)

	case catalogReadyMsg:
		if msg.Err == nil {
			m.state.Catalog = msg.Catalog
			if len(m.state.Selected) == 0 {
				m.state.Selected = msg.Selected
			}
		}
		var cmd tea.Cmd
		m.components, cmd = m.components.Update(msg)
		return m, cmd

	case componentsCommittedMsg:
		m.state.Selected = msg.Selected
		return m, switchTo(previewScreen)

	case planReadyMsg:
		if msg.Err == nil {
			m.state.Plan = msg.Plan
		}
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd

	case previewCommittedMsg:
		m.state.SkipFiles = msg.SkipFiles
		return m, switchTo(applyScreen)

	case writeDoneMsg:
		m.state.Results = append(m.state.Results, msg.Result)
		var cmd tea.Cmd
		m.apply, cmd = m.apply.Update(msg)
		return m, cmd

	case applyDoneMsg:
		if msg.Err != nil {
			m.state.ApplyErr = msg.Err
		}
		var cmd tea.Cmd
		m.apply, cmd = m.apply.Update(msg)
		return m, cmd
	}

	// Otherwise: route to the active screen.
	var cmd tea.Cmd
	switch m.current {
	case welcomeScreen:
		m.welcome, cmd = m.welcome.Update(msg)
	case scanScreen:
		m.scan, cmd = m.scan.Update(msg)
	case profileScreen:
		m.profile, cmd = m.profile.Update(msg)
	case componentsScreen:
		m.components, cmd = m.components.Update(msg)
	case previewScreen:
		m.preview, cmd = m.preview.Update(msg)
	case applyScreen:
		m.apply, cmd = m.apply.Update(msg)
	case doneScreen:
		m.done, cmd = m.done.Update(msg)
	}
	return m, cmd
}

// handleScreenSwitch transitions to the requested screen, copying any
// state the new screen needs into it and firing the cross-screen tea.Cmd
// that produces its data.
func (m appModel) handleScreenSwitch(to screenID) (tea.Model, tea.Cmd) {
	m.current = to
	switch to {
	case scanScreen:
		m.scan = m.scan.Reset()
		return m, m.detectCmd(m.state.RepoRoot)
	case profileScreen:
		m.profile = m.profile.WithStack(m.state.Stack)
		return m, nil
	case componentsScreen:
		m.components = m.components.WithSelection(m.state.Selected)
		return m, m.resolveCmd(m.state.Profile, m.state.Stack)
	case previewScreen:
		return m, m.buildPlanCmd(m.state.RepoRoot, m.state.Catalog, m.state.Selected, m.state.SkipFiles)
	case applyScreen:
		m.apply = m.apply.WithPlan(m.state.Plan)
		return m, m.applyCmd(m.state.Plan, m.applyDelay)
	case doneScreen:
		m.done = m.done.WithResults(m.state.Profile, m.state.Results, m.state.ApplyErr)
		return m, nil
	}
	return m, nil
}

func (m appModel) View() tea.View {
	switch m.current {
	case welcomeScreen:
		return m.welcome.View()
	case scanScreen:
		return m.scan.View()
	case profileScreen:
		return m.profile.View()
	case componentsScreen:
		return m.components.View()
	case previewScreen:
		return m.preview.View()
	case applyScreen:
		return m.apply.View()
	case doneScreen:
		return m.done.View()
	}
	return tea.NewView("")
}

// --- default cross-screen tea.Cmds, real implementations ---

func defaultDetectCmd(repoRoot string) tea.Cmd {
	return func() tea.Msg {
		opts := session.Options{RepoRoot: repoRoot}
		det, err := session.Detect(context.Background(), opts)
		if err != nil {
			return stackMsg{Err: err}
		}
		return stackMsg{Stack: det.Stack, Detection: det, Snapshot: det.Snapshot}
	}
}

func defaultResolveCmd(p profile.Name, stack detect.Stack) tea.Cmd {
	return func() tea.Msg {
		opts := session.Options{Profile: p}
		catalog, selected, err := session.ResolveAndMaterialize(opts, stack)
		return catalogReadyMsg{Catalog: catalog, Selected: selected, Err: err}
	}
}

func defaultBuildPlanCmd(repoRoot string, comps []component.Component, selected map[component.ID]bool, skipFiles map[string]bool) tea.Cmd {
	return func() tea.Msg {
		opts := session.Options{RepoRoot: repoRoot, SkipFiles: skipFiles}
		plan, err := session.BuildPlan(opts, comps, selected)
		return planReadyMsg{Plan: plan, Err: err}
	}
}

// defaultApplyCmd animates the (stub) apply by emitting one writeDoneMsg
// per file with `delay` between each, then a final applyDoneMsg.
// delay=0 makes it instant (CI / tests).
func defaultApplyCmd(plan *apply.Plan, delay time.Duration) tea.Cmd {
	if plan == nil || len(plan.Writes) == 0 {
		return func() tea.Msg {
			return applyDoneMsg{Results: nil, Err: nil}
		}
	}
	cmds := make([]tea.Cmd, 0, len(plan.Writes)+1)
	total := len(plan.Writes)
	for i, w := range plan.Writes {
		i, w := i, w
		emit := func() tea.Msg {
			return writeDoneMsg{
				Result: apply.WriteResult{Path: w.Path, Status: "would-write", Bytes: len(w.Body)},
				Index:  i,
				Total:  total,
			}
		}
		if delay > 0 {
			cmds = append(cmds, tea.Tick(delay, func(time.Time) tea.Msg { return emit() }))
		} else {
			cmds = append(cmds, emit)
		}
	}
	cmds = append(cmds, func() tea.Msg {
		// Trailing applyDoneMsg with the full results slice.
		results := make([]apply.WriteResult, 0, total)
		for _, w := range plan.Writes {
			results = append(results, apply.WriteResult{Path: w.Path, Status: "would-write", Bytes: len(w.Body)})
		}
		return applyDoneMsg{Results: results}
	})
	return tea.Sequence(cmds...)
}
