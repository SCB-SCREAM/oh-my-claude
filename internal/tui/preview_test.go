package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
)

func TestPreview_PlanReadyPopulates(t *testing.T) {
	m := NewPreview(NewTheme())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	plan := &apply.Plan{
		Stub: true,
		Writes: []apply.FileWrite{
			{Path: "a.md", Body: []byte("hello\n"), Action: apply.ActionCreate},
			{Path: "b.md", Body: []byte("world\n"), Action: apply.ActionCreate},
		},
	}
	m, _ = m.Update(planReadyMsg{Plan: plan})

	if m.loading {
		t.Error("loading flag should clear after planReadyMsg")
	}
	if m.plan == nil || len(m.plan.Writes) != 2 {
		t.Fatalf("expected 2 writes in stored plan; got %+v", m.plan)
	}
}

func TestPreview_ToggleSkip(t *testing.T) {
	m := NewPreview(NewTheme())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	plan := &apply.Plan{
		Stub: true,
		Writes: []apply.FileWrite{
			{Path: "a.md", Body: []byte("x"), Action: apply.ActionCreate},
		},
	}
	m, _ = m.Update(planReadyMsg{Plan: plan})

	m, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if !m.skipFiles["a.md"] {
		t.Errorf("expected a.md in skipFiles after 's'; have %v", m.skipFiles)
	}
}

func TestPreview_EnterCommits(t *testing.T) {
	m := NewPreview(NewTheme())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	plan := &apply.Plan{
		Stub: true,
		Writes: []apply.FileWrite{
			{Path: "a.md", Body: []byte("x"), Action: apply.ActionCreate},
		},
	}
	m, _ = m.Update(planReadyMsg{Plan: plan})
	m, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"}) // skip a.md

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	committed, ok := cmd().(previewCommittedMsg)
	if !ok {
		t.Fatalf("expected previewCommittedMsg, got %T", cmd())
	}
	if !committed.SkipFiles["a.md"] {
		t.Errorf("expected a.md in committed skips; have %v", committed.SkipFiles)
	}
}
