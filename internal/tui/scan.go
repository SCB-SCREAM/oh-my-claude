package tui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
	"github.com/SCB-SCREAM/oh-my-claude/internal/report"
)

// ScanModel is screen 2: asks the local `claude` CLI to classify the
// project and renders the structured [detect.Stack] it returns. The
// older "tree of signals" UX is gone — detection now produces a single
// Stack, not a stream of named heuristics, so the screen is a simple
// "spinner → key:value summary → [enter] to continue" flow.
type ScanModel struct {
	theme         Theme
	width, height int
	repoRoot      string

	spinner   spinner.Model
	loading   bool
	stack     detect.Stack
	detection *detect.Result
	err       error
}

// NewScan constructs a fresh ScanModel pointed at the supplied repo
// root. Detection is kicked off by the root model when the user
// transitions onto this screen.
func NewScan(theme Theme, repoRoot string) ScanModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return ScanModel{
		theme:    theme,
		repoRoot: repoRoot,
		spinner:  sp,
		loading:  true,
	}
}

// Reset returns a fresh scan model that re-runs detection. Root calls
// this on screen-switch (re-entering the scan screen via the `r` key).
func (m ScanModel) Reset() ScanModel {
	m.loading = true
	m.stack = detect.Stack{}
	m.detection = nil
	m.err = nil
	return m
}

// Init implements [tea.Model] — starts the loading-spinner tick so the
// screen has motion while the LLM call is in flight.
func (m ScanModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements [tea.Model] — handles spinner ticks, the inbound
// stackMsg, and the keyboard bindings ([enter] continue, [r] re-detect,
// [b] back, [q] quit).
func (m ScanModel) Update(msg tea.Msg) (ScanModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case stackMsg:
		m.loading = false
		m.err = msg.Err
		m.stack = msg.Stack
		m.detection = msg.Detection
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "esc", "b":
			return m, switchTo(welcomeScreen)
		case "r":
			m = m.Reset()
			return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
				return switchScreenMsg{To: scanScreen}
			})
		case "enter":
			if m.loading || m.err != nil {
				return m, nil
			}
			return m, switchTo(profileScreen)
		}
	}
	return m, nil
}

// View implements [tea.Model] — spinner while loading, then the Stack
// rendered as a key:value summary with a cache-status one-liner.
func (m ScanModel) View() tea.View {
	header := m.theme.Title.Render("omc — scan") + " " + m.theme.Subtle.Render("("+m.repoRoot+")")
	footer := m.theme.Hint.Render("[enter] continue  ·  [r] re-detect  ·  [b] back  ·  [q] quit")

	var body string
	switch {
	case m.loading:
		body = m.spinner.View() + "  asking claude to classify this project..."
	case m.err != nil:
		body = m.theme.Subtle.Render("detection error: " + m.err.Error())
	default:
		body = m.renderStack()
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

func (m ScanModel) renderStack() string {
	if m.stack.Type == "" {
		return m.theme.Subtle.Render("detection returned no Stack — press [r] to retry")
	}

	var lines []string
	lines = append(lines, m.theme.Accent.Render("detected: ")+report.StackSummary(m.stack))

	if status := m.cacheStatus(); status != "" {
		lines = append(lines, m.theme.Subtle.Render(status))
	}
	lines = append(lines, "")
	lines = append(lines, report.StackLines(m.stack)...)
	return strings.Join(lines, "\n")
}

// cacheStatus turns Detection's FromCache/Refreshed/Changed metadata
// into a one-liner. Empty when there's nothing interesting to say
// (first ever run with no prior cache).
func (m ScanModel) cacheStatus() string {
	if m.detection == nil {
		return ""
	}
	switch {
	case m.detection.FromCache:
		return "(cached — no claude call needed)"
	case m.detection.Refreshed && len(m.detection.Changed) > 0:
		head := m.detection.Changed[0]
		if len(m.detection.Changed) > 1 {
			return "(re-detected — " + head + " and others changed)"
		}
		return "(re-detected — " + head + " changed)"
	}
	return ""
}
