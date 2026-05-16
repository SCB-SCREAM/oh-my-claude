package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
	"github.com/SCB-SCREAM/oh-my-claude/internal/profile"
)

func TestDone_EnterQuits(t *testing.T) {
	m := NewDone(NewTheme()).WithResults(profile.Recommended, []apply.WriteResult{{Path: "a"}}, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit tea.Quit")
	}
	// tea.Quit() returns a tea.QuitMsg
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected QuitMsg; got %T", msg)
	}
}

func TestDone_ViewMentionsPreviewBanner(t *testing.T) {
	m := NewDone(NewTheme()).WithResults(profile.Recommended, []apply.WriteResult{{Path: "a"}, {Path: "b"}}, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	out := m.View().Content
	if !contains(out, "M4 preview build") {
		t.Errorf("expected M4 preview banner in done view; got:\n%s", out)
	}
	if !contains(out, "no files were written") {
		t.Errorf("expected 'no files were written' in done view")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
