package templates

import "testing"

func TestEmbedHasStacksDir(t *testing.T) {
	t.Parallel()
	entries, err := FS.ReadDir("stacks")
	if err != nil {
		t.Fatalf("ReadDir(stacks): %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("stacks/ embedded but empty — placeholder README.md should be present")
	}
}
