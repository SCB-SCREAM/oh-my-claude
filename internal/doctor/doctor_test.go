package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findingByName(fs []Finding, name string) Finding {
	for _, f := range fs {
		if f.Name == name {
			return f
		}
	}
	return Finding{}
}

func TestRun_EmptyDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got := Run(root)

	if f := findingByName(got, "git-repo"); f.Severity != SeverityWarn {
		t.Errorf("git-repo: want SeverityWarn, got %v (%s)", f.Severity, f.Message)
	}
	if f := findingByName(got, "claude-dir"); f.Severity != SeverityInfo {
		t.Errorf("claude-dir: want SeverityInfo, got %v", f.Severity)
	}
	if f := findingByName(got, "settings-local-gitignored"); f.Severity != SeverityInfo {
		t.Errorf("settings-local-gitignored: want SeverityInfo (no file present), got %v", f.Severity)
	}
}

func TestRun_SettingsLocalUnignored(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.local.json"), "{}")
	writeFile(t, filepath.Join(root, ".gitignore"), "node_modules\n")

	got := findingByName(Run(root), "settings-local-gitignored")
	if got.Severity != SeverityError {
		t.Errorf("want SeverityError when settings.local.json present and unignored, got %v (%s)", got.Severity, got.Message)
	}
}

func TestRun_SettingsLocalIgnored(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.local.json"), "{}")
	writeFile(t, filepath.Join(root, ".gitignore"), ".claude/settings.local.json\n")

	got := findingByName(Run(root), "settings-local-gitignored")
	if got.Severity != SeverityOK {
		t.Errorf("want SeverityOK when gitignored, got %v (%s)", got.Severity, got.Message)
	}
}

func TestRun_GitRepoOK(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := findingByName(Run(root), "git-repo")
	if got.Severity != SeverityOK {
		t.Errorf("want SeverityOK when .git/ present, got %v", got.Severity)
	}
}
