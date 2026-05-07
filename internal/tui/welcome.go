package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

// logo is "oh my claude" rendered in the figlet ANSI Shadow font on a single
// row. 96 cols wide × 6 rows tall — needs a terminal at least 96 cols wide.
const logo = ` ██████╗ ██╗  ██╗    ███╗   ███╗██╗   ██╗     ██████╗██╗      █████╗ ██╗   ██╗██████╗ ███████╗
██╔═══██╗██║  ██║    ████╗ ████║╚██╗ ██╔╝    ██╔════╝██║     ██╔══██╗██║   ██║██╔══██╗██╔════╝
██║   ██║███████║    ██╔████╔██║ ╚████╔╝     ██║     ██║     ███████║██║   ██║██║  ██║█████╗
██║   ██║██╔══██║    ██║╚██╔╝██║  ╚██╔╝      ██║     ██║     ██╔══██║██║   ██║██║  ██║██╔══╝
╚██████╔╝██║  ██║    ██║ ╚═╝ ██║   ██║       ╚██████╗███████╗██║  ██║╚██████╔╝██████╔╝███████╗
 ╚═════╝ ╚═╝  ╚═╝    ╚═╝     ╚═╝   ╚═╝        ╚═════╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝`

// WelcomeModel is the first screen the user sees. M1 ships only this screen;
// later screens (scan, profile, components, preview, apply) hang off the
// root model in app.go.
type WelcomeModel struct {
	theme         Theme
	version       string
	width, height int
	ready         bool
}

// NewWelcome constructs the welcome screen with the given theme and version
// string (the latter is rendered under the logo).
func NewWelcome(theme Theme, version string) WelcomeModel {
	return WelcomeModel{theme: theme, version: version}
}

// Init is a no-op: the welcome screen has no startup work.
func (m WelcomeModel) Init() tea.Cmd { return nil }

// Update handles window resizes and key events for the welcome screen.
func (m WelcomeModel) Update(msg tea.Msg) (WelcomeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "enter":
			// M1: no scan screen yet — exit cleanly so the user lands back
			// in the shell rather than getting stuck.
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the welcome screen as a centered logo + tagline + key hint.
func (m WelcomeModel) View() tea.View {
	if !m.ready {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	logoBlock := m.theme.Logo.Render(logo)
	tagline := m.theme.Subtle.Render("bootstrap Claude Code for your project, opinionated by stack")
	versionLine := m.theme.Subtle.Render("version " + m.version)
	hint := m.theme.Hint.Render("press [enter] to start  ·  [q] to quit")

	body := lipgloss.JoinVertical(
		lipgloss.Center,
		logoBlock,
		"",
		tagline,
		versionLine,
		"",
		hint,
	)

	content := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
