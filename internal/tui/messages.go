package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

// screenID identifies the currently active screen in the root model.
type screenID int

const (
	welcomeScreen screenID = iota
	scanScreen
	profileScreen
	componentsScreen
	previewScreen
	applyScreen
	doneScreen
)

// switchScreenMsg is the universal "advance to this screen" message
// every child emits when it's done. Root handles routing.
type switchScreenMsg struct {
	To screenID
}

// switchTo wraps a screenID in a tea.Cmd. Convention: children return
// switchTo(...) from Update rather than mutating root state directly.
func switchTo(s screenID) tea.Cmd {
	return func() tea.Msg { return switchScreenMsg{To: s} }
}

// --- detection ---

// stackMsg is the result of the LLM-backed detector. Stack carries the
// structured project classification (Type + commands + frameworks);
// Detection wraps cache metadata (FromCache, Refreshed, Changed) so the
// scan screen can render "(cached)" / "(re-detected: foo.go changed)".
type stackMsg struct {
	Stack     detect.Stack
	Detection *detect.Result
	Snapshot  *detect.Snapshot
	Err       error
}

// --- profile commit ---

type profileChosenMsg struct {
	Profile string // profile.Name as a string; profile package import lives where used
}

// --- components ---

type catalogReadyMsg struct {
	Catalog  []component.Component
	Selected map[component.ID]bool
	Err      error
}

type componentsCommittedMsg struct {
	Selected map[component.ID]bool
}

// --- preview ---

type planReadyMsg struct {
	Plan *apply.Plan
	Err  error
}

type previewCommittedMsg struct {
	SkipFiles map[string]bool
}

// --- apply ---

type writeDoneMsg struct {
	Result apply.WriteResult
	Index  int
	Total  int
}

type applyDoneMsg struct {
	Results []apply.WriteResult
	Err     error
}
