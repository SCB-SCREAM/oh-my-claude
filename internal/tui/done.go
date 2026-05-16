package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
	"github.com/SCB-SCREAM/oh-my-claude/internal/profile"
)

// DoneModel is the final screen: summary, next-steps, M4 caveat banner.
type DoneModel struct {
	theme         Theme
	width, height int

	profile profile.Name
	results []apply.WriteResult
	err     error
}

// NewDone constructs a fresh DoneModel. Results are attached later via
// WithResults when root switches to this screen.
func NewDone(theme Theme) DoneModel {
	return DoneModel{theme: theme}
}

// WithResults returns a copy with the per-file write results captured.
// Root calls this on screen-switch.
func (m DoneModel) WithResults(p profile.Name, results []apply.WriteResult, err error) DoneModel {
	m.profile = p
	m.results = results
	m.err = err
	return m
}

// Init implements [tea.Model] — the done screen is purely presentational.
func (m DoneModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model] — any of [enter|esc|q] quits the program.
func (m DoneModel) Update(msg tea.Msg) (DoneModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements [tea.Model] — summary, banner, next-steps list,
// centred via lipgloss.Place.
func (m DoneModel) View() tea.View {
	header := m.theme.Title.Render("omc — done")

	summary := fmt.Sprintf("profile=%s  ·  %d would-write file(s)", m.profile, len(m.results))
	if m.err != nil {
		summary += "  ·  " + m.theme.Subtle.Render("(error: "+m.err.Error()+")")
	}

	banner := m.theme.Accent.Render("M4 preview build") + " — " +
		m.theme.Subtle.Render("no files were written to disk")
	caveat := m.theme.Hint.Render(
		"The apply pipeline lands in M5; per-stack templates land in M6.\n" +
			"Re-run after upgrading once those ship.")

	var nextSteps []string
	nextSteps = append(nextSteps,
		"open this repo in `claude` to try out the preview",
		"run `omc init --no-tui --profile "+string(m.profile)+" --dry-run` to see the same plan in your shell",
		"check `omc doctor` to verify your local Claude Code setup",
	)
	steps := strings.Join(prefixEach(nextSteps, "  · "), "\n")

	hint := m.theme.Hint.Render("[enter/q] exit")

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		summary,
		"",
		banner,
		caveat,
		"",
		m.theme.Title.Render("Next steps"),
		steps,
	)

	bodyBlock := m.theme.Box.Render(body)
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", bodyBlock, "", hint)

	v := tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content))
	v.AltScreen = true
	return v
}

func prefixEach(lines []string, prefix string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = prefix + l
	}
	return out
}
