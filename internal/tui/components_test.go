package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
)

func TestComponents_PrefillsFromSelection(t *testing.T) {
	preset := map[component.ID]bool{"claude-md": true, "settings": true}
	m := NewComponents(NewTheme()).WithSelection(preset)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	catalog := []component.Component{
		{ID: "claude-md", Title: "CLAUDE.md", Category: component.CategoryClaudeMD},
		{ID: "settings", Title: "settings", Category: component.CategorySettings},
		{ID: "extra", Title: "Extra", Category: component.CategoryCommand},
	}
	m, _ = m.Update(catalogReadyMsg{Catalog: catalog, Selected: preset})

	if !m.selected["claude-md"] || !m.selected["settings"] {
		t.Errorf("preselection lost; have %v", m.selected)
	}
	if m.selected["extra"] {
		t.Errorf("extra should not be preselected")
	}
}

func TestComponents_SpaceToggles(t *testing.T) {
	preset := map[component.ID]bool{"a": true}
	m := NewComponents(NewTheme()).WithSelection(preset)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	catalog := []component.Component{
		{ID: "a", Title: "A", Category: component.CategoryCommand},
		{ID: "b", Title: "B", Category: component.CategoryCommand},
	}
	m, _ = m.Update(catalogReadyMsg{Catalog: catalog, Selected: preset})

	// Cursor starts on the first selectable row ("a"). Space → deselect a.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if m.selected["a"] {
		t.Errorf("expected 'a' to be deselected after space; have %v", m.selected)
	}
}

func TestComponents_EnterCommitsSelection(t *testing.T) {
	m := NewComponents(NewTheme()).WithSelection(map[component.ID]bool{"a": true})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	catalog := []component.Component{{ID: "a", Title: "A", Category: component.CategoryCommand}}
	m, _ = m.Update(catalogReadyMsg{Catalog: catalog, Selected: map[component.ID]bool{"a": true}})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should produce a cmd")
	}
	committed, ok := cmd().(componentsCommittedMsg)
	if !ok {
		t.Fatalf("expected componentsCommittedMsg, got %T", cmd())
	}
	if !committed.Selected["a"] {
		t.Errorf("committed selection lost 'a'; have %v", committed.Selected)
	}
}
