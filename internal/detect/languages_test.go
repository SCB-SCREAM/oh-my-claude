package detect

import (
	"testing"
	"testing/fstest"
)

func TestDetectGo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		fsys      fstest.MapFS
		want      bool
		wantConf  float64
		wantFiles []string
	}{
		{
			name: "no go.mod",
			fsys: fstest.MapFS{"main.go": &fstest.MapFile{Data: []byte("package main")}},
			want: false,
		},
		{
			name: "go.mod only",
			fsys: fstest.MapFS{
				"go.mod": &fstest.MapFile{Data: []byte("module x\n")},
			},
			want: true, wantConf: ConfManifest, wantFiles: []string{"go.mod"},
		},
		{
			name: "go.mod + go.sum",
			fsys: fstest.MapFS{
				"go.mod": &fstest.MapFile{Data: []byte("module x\n")},
				"go.sum": &fstest.MapFile{Data: []byte("")},
			},
			want: true, wantConf: ConfLockfile, wantFiles: []string{"go.mod", "go.sum"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap, _ := NewSnapshot(tc.fsys)
			got := RunWith([]Detector{detectorFunc{id: "go", fn: detectGo}}, snap)
			if !tc.want {
				if len(got) != 0 {
					t.Errorf("expected no signal, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Name != "go" {
				t.Fatalf("expected go signal, got %+v", got)
			}
			if got[0].Confidence != tc.wantConf {
				t.Errorf("confidence: got %v, want %v", got[0].Confidence, tc.wantConf)
			}
			if !equalSlices(got[0].Evidence, tc.wantFiles) {
				t.Errorf("evidence: got %v, want %v", got[0].Evidence, tc.wantFiles)
			}
		})
	}
}

func TestDetectTypeScript(t *testing.T) {
	t.Parallel()

	t.Run("tsconfig only", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"tsconfig.json": &fstest.MapFile{Data: []byte("{}")},
		})
		got := RunWith([]Detector{detectorFunc{id: "typescript", fn: detectTypeScript}}, snap)
		if len(got) != 1 || got[0].Name != "typescript" {
			t.Fatalf("want typescript, got %+v", got)
		}
		if got[0].Confidence != ConfFileConvention {
			t.Errorf("confidence: %v want %v", got[0].Confidence, ConfFileConvention)
		}
	})

	t.Run("package.json with typescript dep", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"package.json": &fstest.MapFile{Data: []byte(`{"devDependencies":{"typescript":"^5"}}`)},
		})
		got := RunWith([]Detector{detectorFunc{id: "typescript", fn: detectTypeScript}}, snap)
		if len(got) != 1 || got[0].Confidence != ConfManifest {
			t.Fatalf("want ts at ConfManifest, got %+v", got)
		}
	})

	t.Run("no signals", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"main.go": &fstest.MapFile{Data: []byte("package main")},
		})
		got := RunWith([]Detector{detectorFunc{id: "typescript", fn: detectTypeScript}}, snap)
		if len(got) != 0 {
			t.Errorf("unexpected: %+v", got)
		}
	})
}

func TestDetectPython(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{
		"pyproject.toml": &fstest.MapFile{Data: []byte(`[project]
name = "demo"
dependencies = []
`)},
		"app/main.py": &fstest.MapFile{Data: []byte("print('x')\n")},
	})
	got := RunWith([]Detector{detectorFunc{id: "python", fn: detectPython}}, snap)
	if len(got) != 1 || got[0].Name != "python" {
		t.Fatalf("want python signal, got %+v", got)
	}
	if got[0].Confidence != ConfManifest {
		t.Errorf("confidence: %v want %v", got[0].Confidence, ConfManifest)
	}
}

func TestDetectJavaScript(t *testing.T) {
	t.Parallel()

	t.Run("plain js project", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"package.json": &fstest.MapFile{Data: []byte(`{"name":"demo"}`)},
			"index.js":     &fstest.MapFile{Data: []byte("module.exports = {}\n")},
		})
		got := RunWith([]Detector{detectorFunc{id: "javascript", fn: detectJavaScript}}, snap)
		if len(got) != 1 || got[0].Name != "javascript" {
			t.Fatalf("want js, got %+v", got)
		}
	})

	t.Run("ts project does not also emit js manifest signal", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"package.json": &fstest.MapFile{Data: []byte(`{"devDependencies":{"typescript":"^5"}}`)},
		})
		got := RunWith([]Detector{detectorFunc{id: "javascript", fn: detectJavaScript}}, snap)
		if len(got) != 0 {
			t.Errorf("ts-only project should not emit js signal, got %+v", got)
		}
	})
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
