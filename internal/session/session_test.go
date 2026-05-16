package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/SCB-SCREAM/oh-my-claude/internal/component"
	"github.com/SCB-SCREAM/oh-my-claude/internal/detect"
	"github.com/SCB-SCREAM/oh-my-claude/internal/profile"
)

// fakeRunner implements detect.Runner with canned responses keyed on
// the first arg ("--version", "auth", "-p"). Mirrors the private
// fakeRunner in internal/detect but exposed locally so session tests
// don't need to import detect's test helpers.
type fakeRunner struct {
	versionOK bool
	authOK    bool
	response  []byte
}

func (f *fakeRunner) Run(_ context.Context, args []string, _ []byte) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("no args")
	}
	switch args[0] {
	case "--version":
		if !f.versionOK {
			return nil, errors.New("not on PATH")
		}
		return []byte("2.1.132 (Claude Code)\n"), nil
	case "auth":
		if !f.authOK {
			return nil, errors.New("not authenticated")
		}
		return []byte("logged in\n"), nil
	case "-p":
		return f.response, nil
	}
	return nil, errors.New("unexpected args: " + strings.Join(args, " "))
}

// fakeClaudeOK builds a claude-result envelope wrapping the supplied
// JSON body. Uses encoding/json so embedded whitespace/control chars in
// jsonResult are escaped correctly.
func fakeClaudeOK(jsonResult string) []byte {
	quoted, err := json.Marshal(jsonResult)
	if err != nil {
		panic(err)
	}
	return []byte(fmt.Sprintf(
		`{"type":"result","subtype":"success","is_error":false,"result":%s,"stop_reason":"end_turn","total_cost_usd":0.0001}`,
		string(quoted),
	))
}

// fsTSNextPnpm is a tiny synthetic TS/Next/pnpm tree. With the LLM-
// backed detector, the file contents don't matter — only the listing
// reaches the model. We pass a fake Runner that returns a Stack the
// downstream pipeline can act on.
func fsTSNextPnpm() fstest.MapFS {
	return fstest.MapFS{
		"package.json":     &fstest.MapFile{Data: []byte(`{"name":"x"}`)},
		"tsconfig.json":    &fstest.MapFile{Data: []byte(`{}`)},
		"pnpm-lock.yaml":   &fstest.MapFile{Data: []byte("lockfileVersion: 9.0\n")},
		"next.config.js":   &fstest.MapFile{Data: []byte("module.exports = {};\n")},
		"src/app/page.tsx": &fstest.MapFile{Data: []byte("export default () => null\n")},
	}
}

func TestRun_EmptyRepo(t *testing.T) {
	opts := Options{
		RepoFS:  fstest.MapFS{},
		Profile: profile.Recommended,
		DryRun:  true,
		Detect: detect.Options{
			Runner: &fakeRunner{versionOK: true, authOK: true},
		},
	}
	_, err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("empty repo: expected error (no files to classify)")
	}
}

func TestRun_TSNextPnpm(t *testing.T) {
	opts := Options{
		RepoFS:  fsTSNextPnpm(),
		Profile: profile.Recommended,
		DryRun:  true,
		Detect: detect.Options{
			Runner: &fakeRunner{
				versionOK: true,
				authOK:    true,
				response: fakeClaudeOK(`{
				  "type": "webapp",
				  "language_primary": "typescript",
				  "package_manager": "pnpm",
				  "build_cmd": "pnpm build",
				  "test_cmd": "pnpm test",
				  "lint_cmd": "pnpm lint",
				  "formatter": "prettier --write .",
				  "frameworks": ["next.js", "react"]
				}`),
			},
		},
	}
	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Stack.Type != detect.TypeWebApp {
		t.Errorf("expected Type=webapp, got %q", res.Stack.Type)
	}
	if res.Stack.LanguagePrimary != "typescript" {
		t.Errorf("expected typescript, got %q", res.Stack.LanguagePrimary)
	}

	// Recommended profile + a Stack with TestCmd/LintCmd → cmd.test + cmd.lint selected.
	for _, id := range []component.ID{"claude-md", "settings", "gitignore", "cmd.test", "cmd.lint"} {
		if !res.Selected[id] {
			t.Errorf("expected %q selected, got %v", id, ids(res.Selected))
		}
	}

	if res.Plan == nil || len(res.Plan.Writes) == 0 {
		t.Fatalf("expected non-empty plan; got %+v", res.Plan)
	}
	if !res.Plan.Stub {
		t.Error("plan.Stub must be true in M4")
	}
	if len(res.WriteResults) != len(res.Plan.Writes) {
		t.Errorf("WriteResults length %d != Writes length %d", len(res.WriteResults), len(res.Plan.Writes))
	}
	for _, r := range res.WriteResults {
		if r.Status != "would-write" {
			t.Errorf("result %q status %q, want would-write", r.Path, r.Status)
		}
	}
}

func TestRun_RejectsUnknownProfile(t *testing.T) {
	opts := Options{
		RepoFS:  fsTSNextPnpm(),
		Profile: "nope",
		Detect: detect.Options{
			Runner: &fakeRunner{versionOK: true, authOK: true, response: fakeClaudeOK(`{"type":"webapp"}`)},
		},
	}
	_, err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestRun_RequiresRepo(t *testing.T) {
	_, err := Run(context.Background(), Options{Profile: profile.Minimal})
	if err == nil {
		t.Error("expected error when neither RepoRoot nor RepoFS set")
	}
}

func TestResolveAndMaterialize_AllowlistConstrains(t *testing.T) {
	opts := Options{
		RepoFS:             fsTSNextPnpm(),
		Profile:            profile.Recommended,
		ComponentAllowlist: []component.ID{"claude-md", "settings"},
		Detect: detect.Options{
			Runner: &fakeRunner{
				versionOK: true,
				authOK:    true,
				response:  fakeClaudeOK(`{"type":"webapp","test_cmd":"pnpm test","lint_cmd":"pnpm lint"}`),
			},
		},
	}
	det, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	_, selected, err := ResolveAndMaterialize(opts, det.Stack)
	if err != nil {
		t.Fatal(err)
	}
	if !selected["claude-md"] || !selected["settings"] {
		t.Errorf("allowlist members missing from selection: %v", ids(selected))
	}
	if selected["cmd.test"] || selected["gitignore"] {
		t.Errorf("allowlist should have excluded non-listed IDs; got %v", ids(selected))
	}
}

func TestRun_DetectionErrorBubblesUp(t *testing.T) {
	opts := Options{
		RepoFS:  fsTSNextPnpm(),
		Profile: profile.Recommended,
		Detect: detect.Options{
			Runner: &fakeRunner{}, // versionOK=false → ErrClaudeMissing
		},
	}
	_, err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected detection failure to bubble up")
	}
}

func ids(m map[component.ID]bool) []component.ID {
	out := make([]component.ID, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
