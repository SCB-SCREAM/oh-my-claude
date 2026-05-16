package profile

import (
	"errors"
	"strings"
	"testing"
)

func TestAll_Order(t *testing.T) {
	got := All()
	want := []Name{Minimal, Recommended, Full}
	if len(got) != len(want) {
		t.Fatalf("All() length = %d, want %d", len(got), len(want))
	}
	for i, n := range want {
		if got[i] != n {
			t.Errorf("All()[%d] = %q, want %q", i, got[i], n)
		}
	}
}

func TestValidate(t *testing.T) {
	for _, p := range All() {
		if err := Validate(p); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", p, err)
		}
	}
	if err := Validate("nope"); !errors.Is(err, ErrUnknown) {
		t.Errorf("Validate(\"nope\") = %v, want ErrUnknown", err)
	}
}

func TestDescribe_NonEmptyForKnown(t *testing.T) {
	for _, p := range All() {
		if d := Describe(p); d == "" {
			t.Errorf("Describe(%q) = empty, want non-empty blurb", p)
		}
	}
	if d := Describe("nope"); d != "" {
		t.Errorf("Describe(\"nope\") = %q, want empty", d)
	}
}

func TestIDs(t *testing.T) {
	tests := []struct {
		profile   Name
		wantFirst string // first entry should always be claude-md
		minLen    int
	}{
		{Minimal, "claude-md", 2},
		{Recommended, "claude-md", 5},
		{Full, "claude-md", 7},
	}
	for _, tc := range tests {
		t.Run(string(tc.profile), func(t *testing.T) {
			ids, err := IDs(tc.profile)
			if err != nil {
				t.Fatalf("IDs(%q): %v", tc.profile, err)
			}
			if len(ids) < tc.minLen {
				t.Fatalf("IDs(%q) length = %d, want >= %d", tc.profile, len(ids), tc.minLen)
			}
			if ids[0] != tc.wantFirst {
				t.Errorf("IDs(%q)[0] = %q, want %q", tc.profile, ids[0], tc.wantFirst)
			}
		})
	}

	if _, err := IDs("nope"); !errors.Is(err, ErrUnknown) {
		t.Errorf("IDs(\"nope\") = %v, want ErrUnknown", err)
	}
}

func TestIDs_Monotonic(t *testing.T) {
	// Each successive profile should be a superset of the previous one
	// (minimal ⊆ recommended ⊆ full). This is a stable invariant the
	// components screen and the docs both rely on.
	minIDs, _ := IDs(Minimal)
	rec, _ := IDs(Recommended)
	full, _ := IDs(Full)

	assertSubset(t, "minimal ⊆ recommended", minIDs, rec)
	assertSubset(t, "recommended ⊆ full", rec, full)
}

func TestIDs_DefensiveCopy(t *testing.T) {
	// Callers must not be able to mutate the underlying matrix via the
	// returned slice — caching depends on the matrix staying stable.
	ids, _ := IDs(Minimal)
	if len(ids) == 0 {
		t.Fatal("expected non-empty IDs")
	}
	ids[0] = "tampered"

	ids2, _ := IDs(Minimal)
	if ids2[0] == "tampered" {
		t.Errorf("IDs() returns a shared slice; expected defensive copy")
	}
}

func assertSubset(t *testing.T, label string, sub, sup []string) {
	t.Helper()
	supSet := make(map[string]bool, len(sup))
	for _, s := range sup {
		supSet[s] = true
	}
	for _, s := range sub {
		if !supSet[s] {
			t.Errorf("%s: %q in subset but not in superset (superset = %s)", label, s, strings.Join(sup, ","))
		}
	}
}
