package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

func TestScan_StackMsgRendersSummary(t *testing.T) {
	m := NewScan(NewTheme(), "/tmp/test")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	m, _ = m.Update(stackMsg{
		Stack: detect.Stack{
			Type:            detect.TypeCLI,
			LanguagePrimary: "go",
			TestCmd:         "go test -race ./...",
		},
		Detection: &detect.Result{},
	})

	if m.loading {
		t.Error("loading flag should clear after stackMsg")
	}
	if m.stack.Type != detect.TypeCLI {
		t.Errorf("expected Type=cli, got %q", m.stack.Type)
	}

	view := m.View().Content
	if !strings.Contains(view, "cli") {
		t.Errorf("View should mention detected Type; got:\n%s", view)
	}
	if !strings.Contains(view, "go test -race") {
		t.Errorf("View should render Stack fields; got:\n%s", view)
	}
}

func TestScan_EnterAdvancesToProfile(t *testing.T) {
	m := NewScan(NewTheme(), "/tmp/test")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(stackMsg{Stack: detect.Stack{Type: detect.TypeCLI}, Detection: &detect.Result{}})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to emit a tea.Cmd")
	}
	msg := cmd()
	switchMsg, ok := msg.(switchScreenMsg)
	if !ok {
		t.Fatalf("expected switchScreenMsg, got %T", msg)
	}
	if switchMsg.To != profileScreen {
		t.Errorf("expected switch to profileScreen, got %v", switchMsg.To)
	}
}

func TestScan_EnterIgnoredWhileLoading(t *testing.T) {
	m := NewScan(NewTheme(), "/tmp/test")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("enter should be a no-op while detection is in flight")
	}
}

func TestScan_CacheStatusRendered(t *testing.T) {
	m := NewScan(NewTheme(), "/tmp/test")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(stackMsg{
		Stack:     detect.Stack{Type: detect.TypeCLI},
		Detection: &detect.Result{FromCache: true},
	})

	if !strings.Contains(m.View().Content, "cached") {
		t.Errorf("cached results should be flagged in the view")
	}
}
