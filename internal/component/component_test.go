package component

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
)

func TestCatalog_Deterministic(t *testing.T) {
	a := Catalog()
	b := Catalog()
	if !reflect.DeepEqual(idList(a), idList(b)) {
		t.Errorf("Catalog() is not deterministic across calls")
	}
}

func TestCatalog_HasBaselines(t *testing.T) {
	have := map[ID]bool{}
	for _, c := range Catalog() {
		have[c.ID] = true
	}
	for _, id := range []ID{"claude-md", "settings", "gitignore"} {
		if !have[id] {
			t.Errorf("Catalog() missing baseline component %q", id)
		}
	}
}

func TestMaterialize_EmptyStackKeepsBaselinesDropsGated(t *testing.T) {
	mat := Materialize(detect.Stack{})
	for _, c := range mat {
		if c.AppliesTo != nil {
			t.Errorf("Materialize(empty Stack) returned gated component %q", c.ID)
		}
	}
}

func TestMaterialize_TestCommandRequiresTestCmd(t *testing.T) {
	withCmd := Materialize(detect.Stack{Type: detect.TypeCLI, TestCmd: "go test ./..."})
	if !idSet(withCmd)["cmd.test"] {
		t.Errorf("Materialize: cmd.test should apply when Stack.TestCmd is set")
	}
	withoutCmd := Materialize(detect.Stack{Type: detect.TypeCLI})
	if idSet(withoutCmd)["cmd.test"] {
		t.Errorf("Materialize: cmd.test should NOT apply when Stack.TestCmd is empty")
	}
}

// TestFormatHook_FiresWheneverFormatterPresent is the regression test for
// the "no hardcoded stacks" rule. The hook gates on Stack.Formatter being
// non-empty — never on a language name list — so adding Rust support
// (or any new language) is zero work in this package.
func TestFormatHook_FiresWheneverFormatterPresent(t *testing.T) {
	rust := detect.Stack{
		Type:            detect.TypeCLI,
		LanguagePrimary: "rust",
		Formatter:       "cargo fmt",
	}
	if !idSet(Materialize(rust))["hook.format-on-save"] {
		t.Errorf("format-on-save should fire for any Stack with Formatter set; rust example failed")
	}
}

func TestFormatHook_DoesNotFireWithoutFormatter(t *testing.T) {
	noFmt := detect.Stack{Type: detect.TypeInfra} // infra stacks rarely declare a formatter
	if idSet(Materialize(noFmt))["hook.format-on-save"] {
		t.Errorf("format-on-save should not fire when Stack.Formatter is empty")
	}
}

func TestByID(t *testing.T) {
	if _, ok := ByID("claude-md"); !ok {
		t.Errorf("ByID(\"claude-md\") = !ok; expected the baseline component")
	}
	if c, ok := ByID("does-not-exist"); ok {
		t.Errorf("ByID(\"does-not-exist\") = %+v, true; want zero, false", c)
	}
}

func TestAllIDs_Sorted(t *testing.T) {
	ids := AllIDs()
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("AllIDs() not sorted: %q before %q", ids[i-1], ids[i])
		}
	}
}

func TestPlaceholderBody_HasBanner(t *testing.T) {
	for _, c := range Catalog() {
		for _, f := range c.Files {
			if !bytes.HasPrefix(f.Body, []byte("# omc preview")) {
				t.Errorf("component %q file %q: body missing preview banner", c.ID, f.Path)
			}
		}
	}
}

// --- helpers ---

func idList(cs []Component) []ID {
	out := make([]ID, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func idSet(cs []Component) map[ID]bool {
	out := map[ID]bool{}
	for _, c := range cs {
		out[c.ID] = true
	}
	return out
}
