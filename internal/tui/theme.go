package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Theme is the set of styles used across all screens. Construct one with
// NewTheme and pass it down — never reach for package-level styles, since
// NO_COLOR-aware construction needs to happen at startup.
type Theme struct {
	Logo    lipgloss.Style
	Title   lipgloss.Style
	Subtle  lipgloss.Style
	Hint    lipgloss.Style
	Footer  lipgloss.Style
	Box     lipgloss.Style
	Accent  lipgloss.Style
	NoColor bool
}

// NewTheme builds the default theme. If NO_COLOR is set (any non-empty value)
// or TERM=dumb, it returns a monochrome variant.
//
// See https://no-color.org/ — presence of NO_COLOR with any value means "off".
func NewTheme() Theme {
	noColor := os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"

	t := Theme{NoColor: noColor}

	accent := lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#A78BFA"}
	muted := lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	text := lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F9FAFB"}

	if noColor {
		t.Logo = lipgloss.NewStyle().Bold(true)
		t.Title = lipgloss.NewStyle().Bold(true)
		t.Subtle = lipgloss.NewStyle()
		t.Hint = lipgloss.NewStyle()
		t.Footer = lipgloss.NewStyle()
		t.Box = lipgloss.NewStyle().Padding(1, 2)
		t.Accent = lipgloss.NewStyle().Bold(true)
		return t
	}

	t.Logo = lipgloss.NewStyle().Bold(true).Foreground(accent)
	t.Title = lipgloss.NewStyle().Bold(true).Foreground(text)
	t.Subtle = lipgloss.NewStyle().Foreground(muted)
	t.Hint = lipgloss.NewStyle().Foreground(muted).Italic(true)
	t.Footer = lipgloss.NewStyle().Foreground(muted)
	t.Box = lipgloss.NewStyle().Padding(1, 2)
	t.Accent = lipgloss.NewStyle().Foreground(accent).Bold(true)
	return t
}
