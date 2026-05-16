package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
)

// ApplyModel is screen 6: progress bar + per-file result list driven by
// writeDoneMsgs from the root's applyCmd. In M4 nothing actually writes
// to disk — the animation is synthetic, paced by --simulate-apply-delay.
type ApplyModel struct {
	theme         Theme
	width, height int

	plan    *apply.Plan
	results []apply.WriteResult
	done    bool
	err     error

	bar progress.Model
}

// NewApply constructs a fresh ApplyModel themed with the supplied
// palette. The internal progress bar is initialised with default
// dimensions and resized on the first WindowSizeMsg.
func NewApply(theme Theme) ApplyModel {
	bar := progress.New()
	return ApplyModel{theme: theme, bar: bar}
}

// WithPlan returns a copy with the new plan captured and progress reset.
func (m ApplyModel) WithPlan(plan *apply.Plan) ApplyModel {
	m.plan = plan
	m.results = nil
	m.done = false
	m.err = nil
	return m
}

// Init implements [tea.Model]. The apply screen waits for writeDoneMsgs
// from the root-owned apply tea.Cmd, so it has no work to schedule on
// activation.
func (m ApplyModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model] — handles per-file progress ticks,
// terminal resizes, progress-bar frame messages, and the [enter] →
// done-screen transition.
func (m ApplyModel) Update(msg tea.Msg) (ApplyModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		barW := m.width - 4
		if barW < 20 {
			barW = 20
		}
		m.bar.SetWidth(barW)

	case writeDoneMsg:
		m.results = append(m.results, msg.Result)
		var cmd tea.Cmd
		if msg.Total > 0 {
			cmd = m.bar.SetPercent(float64(len(m.results)) / float64(msg.Total))
		}
		return m, cmd

	case applyDoneMsg:
		m.done = true
		if msg.Err != nil {
			m.err = msg.Err
		}
		// Auto-advance to done after a beat is overkill — let the user press enter.

	case progress.FrameMsg:
		var cmd tea.Cmd
		m.bar, cmd = m.bar.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "enter":
			if m.done {
				return m, switchTo(doneScreen)
			}
		}
	}
	return m, nil
}

// View implements [tea.Model] — header + progress bar + the running
// per-file result list + footer hint.
func (m ApplyModel) View() tea.View {
	header := m.theme.Title.Render("omc — apply") + " " +
		m.theme.Subtle.Render("(M4 simulates — no real writes)")

	total := 0
	if m.plan != nil {
		total = len(m.plan.Writes)
	}

	status := fmt.Sprintf("%d / %d", len(m.results), total)
	if m.done {
		status = m.theme.Accent.Render("done") + "  " + status
	}

	var rows []string
	for _, r := range m.results {
		row := fmt.Sprintf("  · %-12s %s (%d bytes)", r.Status, r.Path, r.Bytes)
		rows = append(rows, row)
	}
	if m.err != nil {
		rows = append(rows, m.theme.Subtle.Render("error: "+m.err.Error()))
	}

	footer := m.theme.Hint.Render("[enter] continue when done  ·  [q] quit")

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		status,
		"",
		m.bar.View(),
		"",
		strings.Join(rows, "\n"),
	)

	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if bodyH < 5 {
		bodyH = 5
	}
	bodyBlock := lipgloss.NewStyle().Height(bodyH).Render(body)

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, "", bodyBlock, footer))
	v.AltScreen = true
	return v
}
