package detect

import (
	"testing"
	"testing/fstest"
)

func TestDetectDocker(t *testing.T) {
	t.Parallel()
	snap, _ := NewSnapshot(fstest.MapFS{
		"Dockerfile":              &fstest.MapFile{Data: []byte("FROM alpine\n")},
		"Dockerfile.dev":          &fstest.MapFile{Data: []byte("FROM alpine\n")},
		"docker-compose.yml":      &fstest.MapFile{Data: []byte("services: {}\n")},
		"docker-compose.prod.yml": &fstest.MapFile{Data: []byte("services: {}\n")},
	})
	got := RunWith([]Detector{detectorFunc{id: "docker", fn: detectDocker}}, snap)
	if len(got) != 1 || got[0].Name != "docker" {
		t.Fatalf("want docker signal, got %+v", got)
	}
	if got[0].Confidence != ConfManifest {
		t.Errorf("confidence: %v want %v", got[0].Confidence, ConfManifest)
	}
	// Up to evidenceCap (5) entries.
	if len(got[0].Evidence) < 4 {
		t.Errorf("expected 4+ evidence files, got %v", got[0].Evidence)
	}
}

func TestDetectKubernetesAndTerraform(t *testing.T) {
	t.Parallel()

	t.Run("kubernetes via kustomization", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"infra/k8s/kustomization.yaml": &fstest.MapFile{Data: []byte("resources: []\n")},
		})
		got := RunWith([]Detector{detectorFunc{id: "kubernetes", fn: detectKubernetes}}, snap)
		if len(got) != 1 || got[0].Name != "kubernetes" {
			t.Fatalf("want kubernetes, got %+v", got)
		}
	})

	t.Run("terraform with lockfile", func(t *testing.T) {
		snap, _ := NewSnapshot(fstest.MapFS{
			"main.tf":             &fstest.MapFile{Data: []byte("terraform {}\n")},
			".terraform.lock.hcl": &fstest.MapFile{Data: []byte("")},
		})
		got := RunWith([]Detector{detectorFunc{id: "terraform", fn: detectTerraform}}, snap)
		if len(got) != 1 || got[0].Confidence != ConfLockfile {
			t.Fatalf("want terraform at ConfLockfile, got %+v", got)
		}
	})
}

func TestDetectCI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fsys fstest.MapFS
		want string
	}{
		{
			"github-actions",
			fstest.MapFS{
				".github/workflows/ci.yml":       &fstest.MapFile{Data: []byte("on: push\n")},
				".github/workflows/release.yaml": &fstest.MapFile{Data: []byte("on: push\n")},
			},
			"github-actions",
		},
		{
			"gitlab-ci",
			fstest.MapFS{".gitlab-ci.yml": &fstest.MapFile{Data: []byte("stages: []\n")}},
			"gitlab-ci",
		},
		{
			"circleci",
			fstest.MapFS{".circleci/config.yml": &fstest.MapFile{Data: []byte("version: 2.1\n")}},
			"circleci",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap, _ := NewSnapshot(tc.fsys)
			got := RunWith(registry, snap)
			found := false
			for _, s := range got {
				if s.Name == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s signal, got %+v", tc.want, got)
			}
		})
	}
}

func TestDetectMonorepo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		file string
		want string
	}{
		{"pnpm-workspace", "pnpm-workspace.yaml", "pnpm-workspace"},
		{"turborepo", "turbo.json", "turborepo"},
		{"nx", "nx.json", "nx"},
		{"lerna", "lerna.json", "lerna"},
		{"go-workspace", "go.work", "go-workspace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap, _ := NewSnapshot(fstest.MapFS{
				tc.file: &fstest.MapFile{Data: []byte("")},
			})
			got := RunWith(registry, snap)
			found := false
			for _, s := range got {
				if s.Name == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s signal, got %+v", tc.want, got)
			}
		})
	}
}
