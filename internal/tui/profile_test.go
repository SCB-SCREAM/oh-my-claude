package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
	"github.com/SCB-SCREAM/oh-my-claude/internal/profile"
)

func TestProfile_EnterEmitsChoice(t *testing.T) {
	m := NewProfile(NewTheme()).WithStack(detect.Stack{Type: detect.TypeCLI, LanguagePrimary: "go"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Default cursor lands on Recommended (1).
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to produce a cmd")
	}
	msg := cmd()
	chosen, ok := msg.(profileChosenMsg)
	if !ok {
		t.Fatalf("expected profileChosenMsg, got %T", msg)
	}
	if chosen.Profile != string(profile.Recommended) {
		t.Errorf("default chosen profile = %q, want %q", chosen.Profile, profile.Recommended)
	}
}

func TestProfile_NumericQuickPick(t *testing.T) {
	m := NewProfile(NewTheme()).WithStack(detect.Stack{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	chosen := cmd().(profileChosenMsg)
	if chosen.Profile != string(profile.Full) {
		t.Errorf("after `3`+enter, profile = %q, want %q", chosen.Profile, profile.Full)
	}
}

func TestProfile_JKMovesCursor(t *testing.T) {
	m := NewProfile(NewTheme()).WithStack(detect.Stack{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	chosen := cmd().(profileChosenMsg)
	if chosen.Profile != string(profile.Full) {
		t.Errorf("after `j`+enter from Recommended, profile = %q, want %q", chosen.Profile, profile.Full)
	}
}
