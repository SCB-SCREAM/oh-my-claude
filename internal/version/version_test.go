package version

import (
	"strings"
	"testing"
)

func TestStringIncludesAllFields(t *testing.T) {
	t.Parallel()
	got := String()
	for _, want := range []string{Version, Commit, Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}

// TestBuildInfoFallback exercises the BuildInfo hydration path. Under
// `go test`, debug.ReadBuildInfo always succeeds and Main.Version is
// "(devel)", so hydration must NOT clobber the package-level sentinels.
func TestBuildInfoFallback(t *testing.T) {
	t.Parallel()

	if Version == "" || Commit == "" || Date == "" {
		t.Fatalf("vars empty after init: V=%q C=%q D=%q", Version, Commit, Date)
	}

	if Version == "(devel)" {
		t.Errorf("Version was clobbered with %q — hydration should reject sentinel", Version)
	}
}
