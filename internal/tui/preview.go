package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
)

// PreviewModel is screen 5: left pane lists every planned file with its
// action tag; right pane shows the rendered diff (M4 primitive — full
// unified diffs land in M5).
type PreviewModel struct {
	theme         Theme
	width, height int

	plan      *apply.Plan
	skipFiles map[string]bool
	cursor    int

	loading bool

	vp viewport.Model
}

func NewPreview(theme Theme) PreviewModel {
	vp := viewport.New()
	return PreviewModel{
		theme:     theme,
		skipFiles: map[string]bool{},
		loading:   true,
		vp:        vp,
	}
}

func (m PreviewModel) Init() tea.Cmd { return nil }

func (m PreviewModel) Update(msg tea.Msg) (PreviewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()
		m.refreshViewport()

	case planReadyMsg:
		m.loading = false
		if msg.Err != nil {
			return m, nil
		}
		m.plan = msg.Plan
		m.cursor = 0
		m.refreshViewport()

	case tea.KeyPressMsg:
		if m.loading || m.plan == nil {
			return m, nil
		}
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "esc", "b":
			return m, switchTo(componentsScreen)
		case "j", "down":
			if m.cursor < len(m.plan.Writes)-1 {
				m.cursor++
				m.refreshViewport()
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.refreshViewport()
			}
		case "s":
			if w, ok := m.currentWrite(); ok {
				if m.skipFiles[w.Path] {
					delete(m.skipFiles, w.Path)
				} else {
					m.skipFiles[w.Path] = true
				}
			}
		case "enter":
			skip := make(map[string]bool, len(m.skipFiles))
			for k, v := range m.skipFiles {
				skip[k] = v
			}
			return m, func() tea.Msg { return previewCommittedMsg{SkipFiles: skip} }
		default:
			// Forward scroll keys to the viewport.
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m PreviewModel) View() tea.View {
	header := m.theme.Title.Render("omc — preview") + " " +
		m.theme.Subtle.Render("(review changes before applying)")
	footer := m.theme.Hint.Render(
		"[j/k] move  ·  [s] toggle skip  ·  [pgup/pgdn/u/d] scroll  ·  [enter] continue  ·  [b] back  ·  [q] quit")

	if m.loading {
		v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, "", m.theme.Subtle.Render("building plan..."), "", footer))
		v.AltScreen = true
		return v
	}
	if m.plan == nil || len(m.plan.Writes)+len(m.plan.Skips) == 0 {
		v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, "", m.theme.Subtle.Render("nothing to preview"), "", footer))
		v.AltScreen = true
		return v
	}

	leftW := m.width * 38 / 100
	if leftW < 24 {
		leftW = 24
	}
	rightW := m.width - leftW - 2
	if rightW < 20 {
		rightW = 20
	}

	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if bodyH < 5 {
		bodyH = 5
	}

	left := m.renderFileList(leftW)
	leftBlock := lipgloss.NewStyle().Width(leftW).Height(bodyH).Render(left)

	rightBlock := lipgloss.NewStyle().Width(rightW).Height(bodyH).Padding(0, 1).Render(m.vp.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, rightBlock)
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, "", body, footer))
	v.AltScreen = true
	return v
}

func (m PreviewModel) renderFileList(w int) string {
	var lines []string
	for i, fw := range m.plan.Writes {
		tag := fw.Action.String()
		if m.skipFiles[fw.Path] {
			tag = "skip"
		}
		cursor := "  "
		if i == m.cursor {
			cursor = m.theme.Accent.Render("> ")
		}
		line := fmt.Sprintf("%s[%-9s] %s", cursor, tag, fw.Path)
		lines = append(lines, line)
	}
	for _, sk := range m.plan.Skips {
		lines = append(lines, m.theme.Subtle.Render(fmt.Sprintf("  [%-9s] %s", "preskip", sk.Path)))
	}
	return lipgloss.NewStyle().Width(w).Render(strings.Join(lines, "\n"))
}

func (m PreviewModel) currentWrite() (apply.FileWrite, bool) {
	if m.plan == nil || m.cursor < 0 || m.cursor >= len(m.plan.Writes) {
		return apply.FileWrite{}, false
	}
	return m.plan.Writes[m.cursor], true
}

func (m *PreviewModel) refreshViewport() {
	if m.plan == nil {
		m.vp.SetContent("")
		return
	}
	w, ok := m.currentWrite()
	if !ok {
		m.vp.SetContent("")
		return
	}
	if m.skipFiles[w.Path] {
		w.Action = apply.ActionSkip
	}
	m.vp.SetContent(apply.RenderDiff(w))
}

func (m *PreviewModel) resizeViewport() {
	leftW := m.width * 38 / 100
	if leftW < 24 {
		leftW = 24
	}
	rightW := m.width - leftW - 4
	if rightW < 10 {
		rightW = 10
	}
	bodyH := m.height - 4
	if bodyH < 5 {
		bodyH = 5
	}
	m.vp.SetWidth(rightW)
	m.vp.SetHeight(bodyH)
}
