package detect

import (
	"testing"
	"testing/fstest"
)

func TestDepIndex_PackageJSON(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"package.json": &fstest.MapFile{Data: []byte(`{
  "name": "demo",
  "dependencies": { "next": "^14.0.0", "React": "18.2.0" },
  "devDependencies": { "typescript": "^5.0.0" }
}`)},
	}
	snap, _ := NewSnapshot(fsys)
	if v, ok := snap.HasDep("npm", "next"); !ok || v != "^14.0.0" {
		t.Errorf("next: got (%q,%v), want (^14.0.0,true)", v, ok)
	}
	if _, ok := snap.HasDep("npm", "react"); !ok {
		t.Error("react: case-insensitive lookup failed")
	}
	if _, ok := snap.HasDep("npm", "typescript"); !ok {
		t.Error("typescript (devDeps): not found")
	}
	if got := snap.Deps().Manifests("npm"); len(got) != 1 || got[0] != "package.json" {
		t.Errorf("manifests: got %v, want [package.json]", got)
	}
}

func TestDepIndex_PEP621Pyproject(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"pyproject.toml": &fstest.MapFile{Data: []byte(`
[project]
name = "demo"
dependencies = [
    "django>=4.2,<5",
    "psycopg[binary]==3.1.18",
]
[project.optional-dependencies]
test = ["pytest>=8"]
`)},
	}
	snap, _ := NewSnapshot(fsys)
	if _, ok := snap.HasDep("python", "django"); !ok {
		t.Error("django: not found via PEP 621 dependencies")
	}
	if _, ok := snap.HasDep("python", "psycopg"); !ok {
		t.Error("psycopg: not found")
	}
	if _, ok := snap.HasDep("python", "pytest"); !ok {
		t.Error("pytest: not found via optional-dependencies")
	}
}

func TestDepIndex_PoetryPyproject(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"pyproject.toml": &fstest.MapFile{Data: []byte(`
[tool.poetry]
name = "demo"

[tool.poetry.dependencies]
python = "^3.11"
fastapi = "^0.110"

[tool.poetry.group.dev.dependencies]
pytest = "^8.2"
`)},
	}
	snap, _ := NewSnapshot(fsys)
	if _, ok := snap.HasDep("python", "fastapi"); !ok {
		t.Error("fastapi: not found via [tool.poetry.dependencies]")
	}
	if _, ok := snap.HasDep("python", "pytest"); !ok {
		t.Error("pytest: not found via [tool.poetry.group.dev.dependencies]")
	}
	if _, ok := snap.HasDep("python", "python"); ok {
		t.Error("python language pin should not register as a dep")
	}
}

func TestDepIndex_RequirementsTxt(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"requirements.txt": &fstest.MapFile{Data: []byte(`# comment
flask==3.0.0
gunicorn # web server
-r dev-requirements.txt
`)},
	}
	snap, _ := NewSnapshot(fsys)
	if _, ok := snap.HasDep("python", "flask"); !ok {
		t.Error("flask: missing")
	}
	if _, ok := snap.HasDep("python", "gunicorn"); !ok {
		t.Error("gunicorn: missing")
	}
}

func TestDepIndex_GoMod(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"go.mod": &fstest.MapFile{Data: []byte(`module example.com/x

go 1.22

require (
    github.com/spf13/cobra v1.10.0
    github.com/charmbracelet/lipgloss v1.1.0 // indirect
)

require golang.org/x/sync v0.20.0
`)},
	}
	snap, _ := NewSnapshot(fsys)
	if v, ok := snap.HasDep("go", "github.com/spf13/cobra"); !ok || v != "v1.10.0" {
		t.Errorf("cobra: (%q,%v) want (v1.10.0,true)", v, ok)
	}
	if _, ok := snap.HasDep("go", "github.com/charmbracelet/lipgloss"); !ok {
		t.Error("lipgloss (// indirect): missing")
	}
	if _, ok := snap.HasDep("go", "golang.org/x/sync"); !ok {
		t.Error("single-line require: missing")
	}
}
