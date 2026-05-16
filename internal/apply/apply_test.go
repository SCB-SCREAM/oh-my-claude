package apply

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
)

func TestBuildPlan_Empty(t *testing.T) {
	plan, err := BuildPlan("/r", nil, nil, BuildOpts{})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Writes) != 0 || len(plan.Skips) != 0 {
		t.Errorf("expected empty plan; got writes=%d skips=%d", len(plan.Writes), len(plan.Skips))
	}
	if !plan.Stub {
		t.Errorf("plan.Stub = false; M4 plans must always be stub")
	}
}

func TestBuildPlan_SelectsOnlyChecked(t *testing.T) {
	comps := []component.Component{
		{ID: "a", Title: "A", Category: component.CategoryCommand, Files: []component.TargetFile{{Path: "a.md", Body: []byte("A")}}},
		{ID: "b", Title: "B", Category: component.CategoryCommand, Files: []component.TargetFile{{Path: "b.md", Body: []byte("B")}}},
	}
	selected := map[component.ID]bool{"a": true}

	plan, err := BuildPlan("/r", comps, selected, BuildOpts{})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Writes) != 1 || plan.Writes[0].Path != "a.md" {
		t.Errorf("expected 1 write for a.md; got %+v", plan.Writes)
	}
}

func TestBuildPlan_SkipPaths(t *testing.T) {
	comps := []component.Component{
		{ID: "a", Title: "A", Category: component.CategoryCommand, Files: []component.TargetFile{
			{Path: "a.md", Body: []byte("A")},
			{Path: "b.md", Body: []byte("B")},
		}},
	}
	selected := map[component.ID]bool{"a": true}
	opts := BuildOpts{SkipPaths: map[string]bool{"b.md": true}}

	plan, err := BuildPlan("/r", comps, selected, opts)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Writes) != 1 || plan.Writes[0].Path != "a.md" {
		t.Errorf("expected only a.md in writes; got %+v", plan.Writes)
	}
	if len(plan.Skips) != 1 || plan.Skips[0].Path != "b.md" {
		t.Errorf("expected b.md in skips; got %+v", plan.Skips)
	}
}

func TestBuildPlan_RejectsUnsafePaths(t *testing.T) {
	tests := []string{"../etc/passwd", "/etc/passwd", "./bad/../escape"}
	for _, p := range tests {
		t.Run(p, func(t *testing.T) {
			comps := []component.Component{
				{ID: "a", Title: "A", Files: []component.TargetFile{{Path: p, Body: []byte("x")}}},
			}
			_, err := BuildPlan("/r", comps, map[component.ID]bool{"a": true}, BuildOpts{})
			if err == nil {
				t.Errorf("BuildPlan accepted unsafe path %q", p)
			}
		})
	}
}

func TestBuildPlan_ActionFromConflictPolicy(t *testing.T) {
	cases := []struct {
		policy component.ConflictPolicy
		want   Action
	}{
		{component.ConflictSkipIfExists, ActionCreate},
		{component.ConflictMerge, ActionMerge},
		{component.ConflictOverwritePrompt, ActionOverwrite},
	}
	for _, tc := range cases {
		comps := []component.Component{{
			ID: "a", Title: "A",
			Conflict: tc.policy,
			Files:    []component.TargetFile{{Path: "a", Body: []byte("x")}},
		}}
		plan, _ := BuildPlan("/r", comps, map[component.ID]bool{"a": true}, BuildOpts{})
		if plan.Writes[0].Action != tc.want {
			t.Errorf("policy %v → action %v, want %v", tc.policy, plan.Writes[0].Action, tc.want)
		}
	}
}

func TestExecute_RequiresStub(t *testing.T) {
	plan := &Plan{Stub: false}
	_, err := Execute(context.Background(), plan)
	if !errors.Is(err, ErrNotStub) {
		t.Errorf("Execute on non-stub plan = %v, want ErrNotStub", err)
	}
}

func TestExecute_NilPlan(t *testing.T) {
	_, err := Execute(context.Background(), nil)
	if err == nil {
		t.Errorf("Execute(nil) = nil; want error")
	}
}

func TestExecute_ProducesResults(t *testing.T) {
	plan := &Plan{
		Stub: true,
		Writes: []FileWrite{
			{Path: "a", Body: []byte("hello")},
			{Path: "b", Body: []byte("world!")},
		},
	}
	results, err := Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for i, r := range results {
		if r.Status != "would-write" {
			t.Errorf("results[%d].Status = %q, want would-write", i, r.Status)
		}
		if r.Bytes != len(plan.Writes[i].Body) {
			t.Errorf("results[%d].Bytes = %d, want %d", i, r.Bytes, len(plan.Writes[i].Body))
		}
	}
}

func TestRenderDiff_Create(t *testing.T) {
	out := RenderDiff(FileWrite{Path: "x", Body: []byte("hello\nworld\n"), Action: ActionCreate})
	if !strings.Contains(out, "+ hello") || !strings.Contains(out, "+ world") {
		t.Errorf("expected `+ ` prefix on every line; got:\n%s", out)
	}
}

func TestRenderDiff_MergeHasBanner(t *testing.T) {
	out := RenderDiff(FileWrite{Path: "x", Body: []byte("y"), Action: ActionMerge})
	if !strings.Contains(out, "M5 will diff") {
		t.Errorf("expected M5 banner for ActionMerge; got:\n%s", out)
	}
}

func TestRenderDiff_Skip(t *testing.T) {
	out := RenderDiff(FileWrite{Action: ActionSkip})
	if !strings.Contains(out, "skipped") {
		t.Errorf("expected skip explanation; got:\n%s", out)
	}
}

func TestAction_String(t *testing.T) {
	cases := map[Action]string{
		ActionCreate:    "new",
		ActionOverwrite: "overwrite",
		ActionMerge:     "merge",
		ActionSkip:      "skip",
	}
	for a, want := range cases {
		if got := a.String(); got != want {
			t.Errorf("Action(%d).String() = %q, want %q", a, got, want)
		}
	}
}
