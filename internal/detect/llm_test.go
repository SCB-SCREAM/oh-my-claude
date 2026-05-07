package detect

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// fakeRunner is the test seam — captures the args passed to each call
// and replays canned responses keyed on the first arg ("--version",
// "auth", "-p").
type fakeRunner struct {
	versionOK bool
	authOK    bool
	response  []byte
	runErr    error
	calls     []fakeCall
}

type fakeCall struct {
	args  []string
	stdin []byte
}

func (f *fakeRunner) Run(_ context.Context, args []string, stdin []byte) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{args: append([]string(nil), args...), stdin: stdin})
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
	case "-p", "--bare":
		if f.runErr != nil {
			return nil, f.runErr
		}
		return f.response, nil
	}
	return nil, errors.New("unexpected args: " + strings.Join(args, " "))
}

func makeFakeSuccess(resultJSON string) []byte {
	return []byte(`{
  "type": "result",
  "subtype": "success",
  "is_error": false,
  "result": ` + jsonString(resultJSON) + `,
  "stop_reason": "end_turn",
  "session_id": "test",
  "total_cost_usd": 0.0001
}`)
}

// jsonString returns s as a JSON-quoted string literal.
func jsonString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 {
				continue
			}
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func TestRunViaLLM_Success(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{
		"build.gradle":    &fstest.MapFile{Data: []byte("plugins {}")},
		"settings.gradle": &fstest.MapFile{Data: []byte("rootProject.name = 'demo'")},
		"src/Main.java":   &fstest.MapFile{Data: []byte("class Main {}")},
	})
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response: makeFakeSuccess(`{"signals":[
  {"name":"java","category":"language","evidence":["src/Main.java","build.gradle"]},
  {"name":"gradle","category":"package-manager","evidence":["build.gradle","settings.gradle"]}
]}`),
	}

	got := RunViaLLM(context.Background(), snap, nil, LLMOptions{
		Runner:   runner,
		CacheDir: t.TempDir(),
	})

	if len(got) != 2 {
		t.Fatalf("want 2 LLM signals, got %d: %+v", len(got), got)
	}
	for _, s := range got {
		if s.Confidence != ConfHeuristic {
			t.Errorf("%s: confidence %v, want ConfHeuristic", s.Name, s.Confidence)
		}
		var sawLLMTag bool
		for _, tag := range s.Tags {
			if tag == "llm-inferred" {
				sawLLMTag = true
			}
		}
		if !sawLLMTag {
			t.Errorf("%s: missing llm-inferred tag, got %v", s.Name, s.Tags)
		}
	}
}

func TestRunViaLLM_RuleSignalsWinOnCollision(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{
		"go.mod":       &fstest.MapFile{Data: []byte("module x\n")},
		"build.gradle": &fstest.MapFile{Data: []byte("")},
	})
	existing := []Signal{
		{Name: "go", Confidence: ConfManifest, Evidence: []string{"go.mod"}, Tags: []string{"language"}},
	}
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		// LLM also tries to emit `go` — should be discarded.
		response: makeFakeSuccess(`{"signals":[
  {"name":"go","category":"language","evidence":["go.mod"]},
  {"name":"java","category":"language","evidence":["build.gradle"]}
]}`),
	}

	got := RunViaLLM(context.Background(), snap, existing, LLMOptions{
		Runner:   runner,
		CacheDir: t.TempDir(),
	})
	if len(got) != 1 || got[0].Name != "java" {
		t.Fatalf("want only `java` (rule-go wins), got %+v", got)
	}
}

func TestRunViaLLM_GracefulDegradation(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x\n")}})

	cases := []struct {
		name   string
		runner *fakeRunner
		want   string
	}{
		{
			"claude not on PATH",
			&fakeRunner{},
			"claude` not on PATH",
		},
		{
			"claude not authenticated",
			&fakeRunner{versionOK: true},
			"claude not authenticated",
		},
		{
			"subprocess error",
			&fakeRunner{versionOK: true, authOK: true, runErr: errors.New("rate-limited")},
			"rate-limited",
		},
		{
			"is_error=true",
			&fakeRunner{versionOK: true, authOK: true, response: []byte(`{"type":"result","is_error":true,"subtype":"error_max_budget_usd","result":""}`)},
			"error_max_budget_usd",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs strings.Builder
			got := RunViaLLM(context.Background(), snap, nil, LLMOptions{
				Runner:   tc.runner,
				CacheDir: t.TempDir(),
				Verbose:  func(f string, a ...any) { fmt.Fprintf(&logs, f+"\n", a...) },
			})
			if len(got) != 0 {
				t.Errorf("want zero signals on degradation, got %+v", got)
			}
			if !strings.Contains(logs.String(), tc.want) {
				t.Errorf("want log to mention %q, got %q", tc.want, logs.String())
			}
		})
	}
}

func TestRunViaLLM_PostParseValidation(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x")}})
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response: makeFakeSuccess(`{"signals":[
  {"name":"java","category":"language","evidence":["src/Main.java"]},
  {"name":"BAD NAME","category":"language","evidence":["x"]},
  {"name":"py","category":"unknown-category","evidence":["x"]},
  {"name":"flag-injected","category":"language","evidence":["-rm -rf /","ok.py"]}
]}`),
	}

	got := RunViaLLM(context.Background(), snap, nil, LLMOptions{
		Runner:   runner,
		CacheDir: t.TempDir(),
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 valid signals after post-parse filter, got %+v", got)
	}
	for _, s := range got {
		switch s.Name {
		case "java":
			// fine
		case "flag-injected":
			// `name` itself is valid; the leading-dash evidence entry must be dropped.
			for _, e := range s.Evidence {
				if strings.HasPrefix(e, "-") {
					t.Errorf("evidence retained leading-dash entry: %q", e)
				}
			}
		default:
			t.Errorf("unexpected signal %q in result", s.Name)
		}
	}
}

func TestRunViaLLM_CachesResults(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x\n")}})
	cacheDir := t.TempDir()
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"signals":[{"name":"java","category":"language","evidence":["x.java"]}]}`),
	}

	first := RunViaLLM(context.Background(), snap, nil, LLMOptions{Runner: runner, CacheDir: cacheDir})
	if len(first) != 1 {
		t.Fatalf("first call: want 1 signal, got %+v", first)
	}
	firstCalls := len(runner.calls)

	second := RunViaLLM(context.Background(), snap, nil, LLMOptions{Runner: runner, CacheDir: cacheDir})
	if len(second) != 1 {
		t.Fatalf("second call: want cached signal, got %+v", second)
	}
	// Cache hit must NOT call the subprocess (auth or -p) again.
	if extraSubprocess := countSubprocessCalls(runner.calls[firstCalls:]); extraSubprocess != 0 {
		t.Errorf("cache miss: %d subprocess calls after first run, want 0", extraSubprocess)
	}
}

func TestRunViaLLM_SkipCacheBypass(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x\n")}})
	cacheDir := t.TempDir()
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"signals":[{"name":"java","category":"language","evidence":["x.java"]}]}`),
	}

	_ = RunViaLLM(context.Background(), snap, nil, LLMOptions{Runner: runner, CacheDir: cacheDir})
	firstCalls := len(runner.calls)

	_ = RunViaLLM(context.Background(), snap, nil, LLMOptions{Runner: runner, CacheDir: cacheDir, SkipCache: true})
	if got := countSubprocessCalls(runner.calls[firstCalls:]); got == 0 {
		t.Errorf("--skip-cache: want a fresh subprocess call, got 0")
	}
}

func TestRunViaLLM_CacheTTLExpiry(t *testing.T) {
	t.Parallel()

	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x\n")}})
	cacheDir := t.TempDir()
	runner := &fakeRunner{
		versionOK: true,
		authOK:    true,
		response:  makeFakeSuccess(`{"signals":[{"name":"java","category":"language","evidence":["x.java"]}]}`),
	}

	_ = RunViaLLM(context.Background(), snap, nil, LLMOptions{Runner: runner, CacheDir: cacheDir})
	firstCalls := len(runner.calls)

	// Use a TTL of 1 nanosecond — the just-written cache is already stale.
	time.Sleep(2 * time.Millisecond)
	_ = RunViaLLM(context.Background(), snap, nil, LLMOptions{
		Runner: runner, CacheDir: cacheDir, CacheTTL: time.Nanosecond,
	})
	if got := countSubprocessCalls(runner.calls[firstCalls:]); got == 0 {
		t.Errorf("expired cache: want a fresh subprocess call, got 0")
	}
}

func TestRunViaLLM_BareOnlyForAPIKeyMode(t *testing.T) {
	// Cannot t.Parallel — subtests use t.Setenv.

	snap, _ := NewSnapshot(fstest.MapFS{"go.mod": &fstest.MapFile{Data: []byte("module x\n")}})

	t.Run("subscription mode does not pass --bare", func(t *testing.T) {
		runner := &fakeRunner{
			versionOK: true,
			authOK:    true,
			response:  makeFakeSuccess(`{"signals":[]}`),
		}
		_ = RunViaLLM(context.Background(), snap, nil, LLMOptions{Runner: runner, CacheDir: t.TempDir()})
		if got := firstArgOfPrintCall(runner.calls); got == "--bare" {
			t.Errorf("subscription mode: --bare must not be the first arg")
		}
	})

	t.Run("api-key mode passes --bare first", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-test")
		runner := &fakeRunner{
			versionOK: true,
			authOK:    false, // forces fallthrough to API-key mode
			response:  makeFakeSuccess(`{"signals":[]}`),
		}
		_ = RunViaLLM(context.Background(), snap, nil, LLMOptions{Runner: runner, CacheDir: t.TempDir()})
		if got := firstArgOfPrintCall(runner.calls); got != "--bare" {
			t.Errorf("api-key mode: want --bare first, got %q (calls: %+v)", got, runner.calls)
		}
	})
}

func TestParseLLMResponse_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	if got := parseLLMResponse("not json"); got != nil {
		t.Errorf("want nil on invalid JSON, got %+v", got)
	}
	if got := parseLLMResponse(`{"signals": "not-an-array"}`); got != nil {
		t.Errorf("want nil on schema-mismatched payload, got %+v", got)
	}
}

func TestHashListing_DeterministicAndScoped(t *testing.T) {
	t.Parallel()
	a := hashListing([]string{"a", "b", "c"})
	b := hashListing([]string{"a", "b", "c"})
	c := hashListing([]string{"a", "b", "d"})
	if a != b {
		t.Errorf("same input → different hash: %s vs %s", a, b)
	}
	if a == c {
		t.Errorf("different input → same hash: %s vs %s", a, c)
	}
}

func countSubprocessCalls(calls []fakeCall) int {
	n := 0
	for _, c := range calls {
		if len(c.args) == 0 {
			continue
		}
		switch c.args[0] {
		case "-p", "--bare":
			n++
		}
	}
	return n
}

func firstArgOfPrintCall(calls []fakeCall) string {
	for _, c := range calls {
		if len(c.args) == 0 {
			continue
		}
		if c.args[0] == "-p" || c.args[0] == "--bare" {
			return c.args[0]
		}
	}
	return ""
}
