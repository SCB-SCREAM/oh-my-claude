package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
)

// TestWelcome_QuitsOnQ drives the full root model through teatest and
// confirms that `q` on the welcome screen exits the program cleanly.
// This is the canonical teatest smoke test in this package — the rest
// of the per-screen tests exercise Update directly because the screens
// don't satisfy tea.Model on their own.
func TestWelcome_QuitsOnQ(t *testing.T) {
	tm := teatest.NewTestModel(t, New(Options{Version: "test"}),
		teatest.WithInitialTermSize(120, 30))
	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestWelcome_RendersVersion calls View() directly — exercising the
// rendered content via teatest is brittle (the terminal-cleanup escape
// codes dominate FinalOutput once the program exits). Direct View()
// inspection is what every other screen's test uses.
func TestWelcome_RendersVersion(t *testing.T) {
	m := NewWelcome(NewTheme(), "0.3.0-test")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	content := m.View().Content
	if !strings.Contains(content, "0.3.0-test") {
		t.Errorf("expected version in welcome content; got:\n%s", content)
	}
}

var _ = time.Second // keep time imported for future teatest use
