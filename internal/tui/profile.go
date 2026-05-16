package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
	"github.com/SCB-SCREAM/oh-my-claude/internal/profile"
)

// ProfileModel is the third screen: pick minimal / recommended / full.
// Two-pane layout: profile list on the left, blurb + "would install N
// components" hint on the right.
type ProfileModel struct {
	theme         Theme
	width, height int

	stack   detect.Stack
	choices []profile.Name
	cursor  int
}

func NewProfile(theme Theme) ProfileModel {
	return ProfileModel{theme: theme, choices: profile.All()}
}

// WithStack returns a copy with the detected Stack captured. Root calls
// this on screen-switch so the "would install: N components" hint can
// pre-compute.
func (m ProfileModel) WithStack(stack detect.Stack) ProfileModel {
	m.stack = stack
	m.cursor = 1 // default to Recommended
	return m
}

func (m ProfileModel) Init() tea.Cmd { return nil }

func (m ProfileModel) Update(msg tea.Msg) (ProfileModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "1":
			m.cursor = 0
		case "2":
			if len(m.choices) >= 2 {
				m.cursor = 1
			}
		case "3":
			if len(m.choices) >= 3 {
				m.cursor = 2
			}
		case "enter", "space":
			chosen := m.choices[m.cursor]
			return m, func() tea.Msg { return profileChosenMsg{Profile: string(chosen)} }
		}
	}
	return m, nil
}

func (m ProfileModel) View() tea.View {
	header := m.theme.Title.Render("omc — profile") + " " + m.theme.Subtle.Render("(pick a starting point)")
	footer := m.theme.Hint.Render("[j/k] move  ·  [1/2/3] quick pick  ·  [enter] choose  ·  [q] quit")

	leftW := 22
	rightW := m.width - leftW - 4
	if rightW < 20 {
		rightW = 20
	}

	var leftLines []string
	for i, p := range m.choices {
		marker := "  "
		if i == m.cursor {
			marker = m.theme.Accent.Render("> ")
		}
		leftLines = append(leftLines, marker+string(p))
	}
	left := lipgloss.JoinVertical(lipgloss.Left, leftLines...)
	leftBlock := lipgloss.NewStyle().Width(leftW).Render(left)

	hl := m.choices[m.cursor]
	blurb := profile.Describe(hl)
	hint := installHint(hl, m.stack)
	right := lipgloss.JoinVertical(
		lipgloss.Left,
		m.theme.Accent.Render(string(hl)),
		"",
		wrap(blurb, rightW),
		"",
		m.theme.Hint.Render(hint),
	)
	rightBlock := lipgloss.NewStyle().Width(rightW).Padding(0, 2).Render(right)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, rightBlock)
	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if bodyH < 5 {
		bodyH = 5
	}
	bodyBlock := lipgloss.NewStyle().Height(bodyH).Render(body)

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, "", bodyBlock, footer))
	v.AltScreen = true
	return v
}

// installHint returns a "would install N of M applicable components" line.
func installHint(p profile.Name, stack detect.Stack) string {
	ids, err := profile.IDs(p)
	if err != nil {
		return ""
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	cat := component.Materialize(stack)
	hits := 0
	for _, c := range cat {
		if want[string(c.ID)] {
			hits++
		}
	}
	return fmt.Sprintf("would install: %d component(s) given the detected stack", hits)
}

// wrap is a tiny soft-wrap helper since lipgloss doesn't word-wrap by
// default. Splits long lines on word boundaries to fit width.
func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	for _, line := range strings.Split(s, "\n") {
		col := 0
		for i, word := range strings.Fields(line) {
			if i > 0 {
				if col+1+len(word) > width {
					out.WriteByte('\n')
					col = 0
				} else {
					out.WriteByte(' ')
					col++
				}
			}
			out.WriteString(word)
			col += len(word)
		}
		out.WriteByte('\n')
	}
	return strings.TrimRight(out.String(), "\n")
}
