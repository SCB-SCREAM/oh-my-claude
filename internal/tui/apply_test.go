package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/SCB-SCREAM/oh-my-claude/internal/apply"
)

func TestApply_WithPlanResetsState(t *testing.T) {
	m := NewApply(NewTheme())
	m.results = []apply.WriteResult{{Path: "old"}}
	m.done = true

	plan := &apply.Plan{Stub: true, Writes: []apply.FileWrite{{Path: "a"}}}
	m = m.WithPlan(plan)

	if m.done {
		t.Error("done should reset to false on new plan")
	}
	if len(m.results) != 0 {
		t.Errorf("results should reset; got %v", m.results)
	}
	if m.plan != plan {
		t.Errorf("plan not stored")
	}
}

func TestApply_WriteDoneAppends(t *testing.T) {
	m := NewApply(NewTheme())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m.WithPlan(&apply.Plan{Stub: true, Writes: []apply.FileWrite{{Path: "a"}, {Path: "b"}}})

	m, _ = m.Update(writeDoneMsg{
		Result: apply.WriteResult{Path: "a", Status: "would-write", Bytes: 5},
		Index:  0,
		Total:  2,
	})
	if len(m.results) != 1 || m.results[0].Path != "a" {
		t.Errorf("expected result a; got %v", m.results)
	}
}

func TestApply_EnterAfterDoneAdvances(t *testing.T) {
	m := NewApply(NewTheme())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m.WithPlan(&apply.Plan{Stub: true})
	m, _ = m.Update(applyDoneMsg{})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter after done to emit cmd")
	}
	if sw, ok := cmd().(switchScreenMsg); !ok || sw.To != doneScreen {
		t.Errorf("expected switch to doneScreen; got %T %+v", cmd(), cmd())
	}
}

func TestApply_EnterBeforeDoneIsNoop(t *testing.T) {
	m := NewApply(NewTheme())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = m.WithPlan(&apply.Plan{Stub: true})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("expected nil cmd before done; got %T", cmd())
	}
}
