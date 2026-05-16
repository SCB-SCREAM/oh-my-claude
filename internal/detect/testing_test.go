package detect

import (
	"context"
	"errors"
	"strings"
)

// fakeRunner is the shared test seam — captures every arg list passed to
// the underlying [Runner] and replays canned responses keyed on the
// first arg ("--version", "auth", "-p"). Used by every test that needs
// to drive the LLM detection path without a real `claude` binary.
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
	case "-p":
		if f.runErr != nil {
			return nil, f.runErr
		}
		return f.response, nil
	}
	return nil, errors.New("unexpected args: " + strings.Join(args, " "))
}

// makeFakeSuccess wraps a JSON result body in the claudeResult envelope
// the production code parses out of `claude -p --output-format json`.
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

// countSubprocessCalls counts only the `-p` invocations in a captured
// call list — the auth/version preflights don't burn budget so they're
// excluded.
func countSubprocessCalls(calls []fakeCall) int {
	n := 0
	for _, c := range calls {
		if len(c.args) == 0 {
			continue
		}
		if c.args[0] == "-p" {
			n++
		}
	}
	return n
}
