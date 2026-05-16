package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
)

// ComponentsModel is screen 4: a category-grouped checklist of
// applicable components, with a description pane toggled by `i`.
//
// We hand-roll the list rather than using bubbles/v2/list.Model because
// our widget needs grouped headers (non-selectable rows) plus a
// per-row checkbox — a custom delegate ends up being more code than
// the few lines below.
type ComponentsModel struct {
	theme         Theme
	width, height int

	catalog  []component.Component
	selected map[component.ID]bool

	rows   []componentRow
	cursor int

	showDesc bool
	loading  bool
}

// componentRow is one renderable line in the checklist: either a
// category header (no component) or a toggleable component row.
type componentRow struct {
	header   string // non-empty for category headers; rest fields unset
	comp     component.Component
	selected bool
}

func NewComponents(theme Theme) ComponentsModel {
	return ComponentsModel{
		theme:    theme,
		selected: map[component.ID]bool{},
		loading:  true,
	}
}

// WithSelection returns a copy that uses the given preset selection map
// when catalogReadyMsg arrives.
func (m ComponentsModel) WithSelection(sel map[component.ID]bool) ComponentsModel {
	m.selected = copyBoolMap(sel)
	m.loading = true
	return m
}

func (m ComponentsModel) Init() tea.Cmd { return nil }

func (m ComponentsModel) Update(msg tea.Msg) (ComponentsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case catalogReadyMsg:
		m.loading = false
		if msg.Err != nil {
			return m, nil
		}
		m.catalog = msg.Catalog
		// Seed selected from the root-supplied map (already merged with profile).
		if len(m.selected) == 0 {
			m.selected = copyBoolMap(msg.Selected)
		}
		m.rebuildRows()
		m.cursor = m.firstSelectableRow()

	case tea.KeyPressMsg:
		if m.loading {
			return m, nil
		}
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "esc", "b":
			return m, switchTo(profileScreen)
		case "j", "down":
			m.cursor = m.nextRow(m.cursor, +1)
		case "k", "up":
			m.cursor = m.nextRow(m.cursor, -1)
		case "space":
			if r, ok := m.currentRow(); ok && r.header == "" {
				m.toggle(r.comp.ID)
				m.rebuildRows()
			}
		case "a":
			if r, ok := m.currentRow(); ok && r.header == "" {
				m.toggleCategory(r.comp.Category)
				m.rebuildRows()
			}
		case "i":
			m.showDesc = !m.showDesc
		case "enter":
			return m, func() tea.Msg {
				return componentsCommittedMsg{Selected: copyBoolMap(m.selected)}
			}
		}
	}
	return m, nil
}

func (m ComponentsModel) View() tea.View {
	header := m.theme.Title.Render("omc — components") + " " + m.theme.Subtle.Render("(pick what to install)")

	footer := m.theme.Hint.Render(
		"[j/k] move  ·  [space] toggle  ·  [a] toggle category  ·  [i] description  ·  [enter] continue  ·  [b] back  ·  [q] quit")

	if m.loading {
		v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, "", m.theme.Subtle.Render("resolving components..."), "", footer))
		v.AltScreen = true
		return v
	}

	leftW := m.width
	rightW := 0
	if m.showDesc {
		leftW = m.width * 55 / 100
		rightW = m.width - leftW - 1
	}
	if leftW < 20 {
		leftW = m.width
		rightW = 0
	}

	left := m.renderList(leftW)
	var right string
	if m.showDesc {
		right = m.renderDescription(rightW)
	}

	body := left
	if right != "" {
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, lipgloss.NewStyle().Width(rightW).Padding(0, 1).Render(right))
	}

	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if bodyH < 5 {
		bodyH = 5
	}
	bodyBlock := lipgloss.NewStyle().Height(bodyH).Render(body)

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, "", bodyBlock, footer))
	v.AltScreen = true
	return v
}

func (m ComponentsModel) renderList(w int) string {
	if len(m.rows) == 0 {
		return m.theme.Subtle.Render("no components apply to this stack")
	}
	var lines []string
	for i, r := range m.rows {
		if r.header != "" {
			lines = append(lines, m.theme.Accent.Render(r.header))
			continue
		}
		mark := "[ ]"
		if r.selected {
			mark = m.theme.Accent.Render("[x]")
		}
		cursor := "  "
		if i == m.cursor {
			cursor = m.theme.Accent.Render("> ")
		}
		title := r.comp.Title
		line := fmt.Sprintf("%s%s %-22s  %s", cursor, mark, string(r.comp.ID), title)
		lines = append(lines, line)
	}
	return lipgloss.NewStyle().Width(w).Render(strings.Join(lines, "\n"))
}

func (m ComponentsModel) renderDescription(w int) string {
	r, ok := m.currentRow()
	if !ok || r.header != "" {
		return m.theme.Subtle.Render("(no component highlighted)")
	}
	paths := make([]string, 0, len(r.comp.Files))
	for _, f := range r.comp.Files {
		paths = append(paths, f.Path)
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		m.theme.Accent.Render(string(r.comp.ID)),
		m.theme.Subtle.Render(string(r.comp.Category)),
		"",
		wrap(r.comp.Description, w-2),
		"",
		m.theme.Title.Render("Files"),
		strings.Join(prefixEach(paths, "  · "), "\n"),
	)
	return body
}

func (m *ComponentsModel) rebuildRows() {
	m.rows = m.rows[:0]
	groups := map[component.Category][]component.Component{}
	for _, c := range m.catalog {
		groups[c.Category] = append(groups[c.Category], c)
	}
	cats := make([]component.Category, 0, len(groups))
	for cat := range groups {
		cats = append(cats, cat)
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i] < cats[j] })
	for _, cat := range cats {
		m.rows = append(m.rows, componentRow{header: "── " + string(cat) + " ──"})
		for _, c := range groups[cat] {
			m.rows = append(m.rows, componentRow{comp: c, selected: m.selected[c.ID]})
		}
	}
}

func (m *ComponentsModel) toggle(id component.ID) {
	if m.selected[id] {
		delete(m.selected, id)
	} else {
		m.selected[id] = true
	}
}

func (m *ComponentsModel) toggleCategory(cat component.Category) {
	// Determine current state: if all in category are on, turn them all off; else turn all on.
	allOn := true
	for _, c := range m.catalog {
		if c.Category == cat && !m.selected[c.ID] {
			allOn = false
			break
		}
	}
	for _, c := range m.catalog {
		if c.Category != cat {
			continue
		}
		if allOn {
			delete(m.selected, c.ID)
		} else {
			m.selected[c.ID] = true
		}
	}
}

func (m ComponentsModel) currentRow() (componentRow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return componentRow{}, false
	}
	return m.rows[m.cursor], true
}

func (m ComponentsModel) firstSelectableRow() int {
	for i, r := range m.rows {
		if r.header == "" {
			return i
		}
	}
	return 0
}

// nextRow advances cursor by dir, skipping header rows, clamped to [0,len-1].
func (m ComponentsModel) nextRow(from, dir int) int {
	n := len(m.rows)
	i := from + dir
	for i >= 0 && i < n {
		if m.rows[i].header == "" {
			return i
		}
		i += dir
	}
	return from
}

func copyBoolMap(in map[component.ID]bool) map[component.ID]bool {
	out := make(map[component.ID]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
