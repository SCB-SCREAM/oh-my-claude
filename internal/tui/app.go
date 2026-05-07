// Package tui hosts the Bubble Tea v2 program that drives `omc init`'s
// interactive flow. The root model in this file routes messages to the
// active screen; each screen lives in its own file (welcome.go, scan.go, …).
//
// M1 ships only the welcome screen — additional screens are wired in over
// subsequent milestones.
package tui

import (
	tea "charm.land/bubbletea/v2"
)

// screenID identifies the currently active screen in the root model.
type screenID int

const (
	welcomeScreen screenID = iota
	// future: scanScreen, profileScreen, componentsScreen, previewScreen, applyScreen
)

type appModel struct {
	theme   Theme
	current screenID

	welcome WelcomeModel
}

// New constructs the root Bubble Tea model.
func New(version string) tea.Model {
	theme := NewTheme()
	return appModel{
		theme:   theme,
		current: welcomeScreen,
		welcome: NewWelcome(theme, version),
	}
}

// Run starts the Bubble Tea program against stdout/stdin.
func Run(version string) error {
	p := tea.NewProgram(New(version))
	_, err := p.Run()
	return err
}

func (m appModel) Init() tea.Cmd { return m.welcome.Init() }

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global messages first.
	if w, ok := msg.(tea.WindowSizeMsg); ok {
		// Broadcast to every child so each can update its layout cache.
		var cmd tea.Cmd
		m.welcome, cmd = m.welcome.Update(w)
		return m, cmd
	}

	// Route to the active screen.
	var cmd tea.Cmd
	switch m.current {
	case welcomeScreen:
		m.welcome, cmd = m.welcome.Update(msg)
	}
	return m, cmd
}

func (m appModel) View() tea.View {
	switch m.current {
	case welcomeScreen:
		return m.welcome.View()
	}
	return tea.NewView("")
}
