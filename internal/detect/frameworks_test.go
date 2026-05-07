package detect

import (
	"testing"
	"testing/fstest"
)

func TestDetectNext_EmitsReactCompanion(t *testing.T) {
	t.Parallel()
	snap, _ := NewSnapshot(fstest.MapFS{
		"package.json": &fstest.MapFile{Data: []byte(`{
"dependencies": {"next": "^14.0.0", "react": "^18"}
}`)},
		"next.config.js": &fstest.MapFile{Data: []byte("module.exports = {}\n")},
	})
	got := RunWith([]Detector{detectorFunc{id: "next.js", fn: detectNext}}, snap)
	var sawNext, sawReact bool
	for _, s := range got {
		if s.Name == "next.js" {
			sawNext = true
			if s.Confidence != ConfManifest {
				t.Errorf("next confidence: %v want %v", s.Confidence, ConfManifest)
			}
		}
		if s.Name == "react" {
			sawReact = true
		}
	}
	if !sawNext {
		t.Errorf("no next.js signal: %+v", got)
	}
	if !sawReact {
		t.Errorf("expected react companion signal: %+v", got)
	}
}

func TestDetectDjango(t *testing.T) {
	t.Parallel()

	t.Run("dep+manage.py stack to ConfManifest", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"requirements.txt": &fstest.MapFile{Data: []byte("django==5.0\n")},
			"manage.py":        &fstest.MapFile{Data: []byte("#!/usr/bin/env python\n")},
		})
		got := RunWith([]Detector{detectorFunc{id: "django", fn: detectDjango}}, snap)
		if len(got) != 1 || got[0].Confidence != ConfManifest {
			t.Errorf("want django at ConfManifest, got %+v", got)
		}
	})

	t.Run("dep alone is dep-declared", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"requirements.txt": &fstest.MapFile{Data: []byte("Django==5.0\n")},
		})
		got := RunWith([]Detector{detectorFunc{id: "django", fn: detectDjango}}, snap)
		if len(got) != 1 || got[0].Confidence != ConfDepDeclared {
			t.Errorf("want django at ConfDepDeclared, got %+v", got)
		}
	})
}

func TestDetectFastAPIAndFlask(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dep  string
		fn   func(*Snapshot) []Signal
		want string
	}{
		{"fastapi", "fastapi", detectFastAPI, "fastapi"},
		{"flask", "flask", detectFlask, "flask"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap, _ := NewSnapshot(fstest.MapFS{
				"requirements.txt": &fstest.MapFile{Data: []byte(tc.dep + "==1.0\n")},
			})
			got := RunWith([]Detector{detectorFunc{id: tc.name, fn: tc.fn}}, snap)
			if len(got) != 1 || got[0].Name != tc.want {
				t.Errorf("want %s signal, got %+v", tc.want, got)
			}
		})
	}
}

func TestDetectVite(t *testing.T) {
	t.Parallel()
	snap, _ := NewSnapshot(fstest.MapFS{
		"package.json":   &fstest.MapFile{Data: []byte(`{"devDependencies":{"vite":"^5"}}`)},
		"vite.config.ts": &fstest.MapFile{Data: []byte("export default {}\n")},
	})
	got := RunWith([]Detector{detectorFunc{id: "vite", fn: detectVite}}, snap)
	if len(got) != 1 || got[0].Name != "vite" {
		t.Fatalf("want vite, got %+v", got)
	}
	if got[0].Confidence != ConfManifest {
		t.Errorf("confidence: %v want %v", got[0].Confidence, ConfManifest)
	}
}
