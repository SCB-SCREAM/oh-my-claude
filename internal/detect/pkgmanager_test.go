package detect

import (
	"testing"
	"testing/fstest"
)

func TestDetectPackageManagers_Lockfiles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		lockfile string
		wantName string
	}{
		{"package-lock.json", "npm"},
		{"yarn.lock", "yarn"},
		{"pnpm-lock.yaml", "pnpm"},
		{"bun.lockb", "bun"},
		{"uv.lock", "uv"},
	}

	for _, tc := range cases {
		t.Run(tc.lockfile, func(t *testing.T) {
			snap, _ := NewSnapshot(fstest.MapFS{
				tc.lockfile: &fstest.MapFile{Data: []byte("")},
			})
			got := RunWith(registry, snap)
			found := false
			for _, s := range got {
				if s.Name == tc.wantName {
					if s.Confidence != ConfLockfile {
						t.Errorf("%s: confidence %v want %v", tc.wantName, s.Confidence, ConfLockfile)
					}
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s signal from %s, got %+v", tc.wantName, tc.lockfile, got)
			}
		})
	}
}

func TestDetectPip_SuppressedByPoetry(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{
		"requirements.txt": &fstest.MapFile{Data: []byte("flask\n")},
		"poetry.lock":      &fstest.MapFile{Data: []byte("")},
		"pyproject.toml":   &fstest.MapFile{Data: []byte("[tool.poetry]\nname = \"x\"\n")},
	})
	got := RunWith([]Detector{detectorFunc{id: "pip", fn: detectPip}}, snap)
	if len(got) != 0 {
		t.Errorf("pip should be suppressed when poetry.lock present, got %+v", got)
	}
}

func TestDetectPoetry(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{
		"pyproject.toml": &fstest.MapFile{Data: []byte(`[tool.poetry]
name = "demo"

[tool.poetry.dependencies]
python = "^3.11"
`)},
	})
	got := RunWith([]Detector{detectorFunc{id: "poetry", fn: detectPoetry}}, snap)
	if len(got) != 1 || got[0].Name != "poetry" {
		t.Fatalf("want poetry signal, got %+v", got)
	}
}
