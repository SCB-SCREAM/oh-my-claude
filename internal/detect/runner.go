package detect

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// Runner is the seam between [DetectViaLLM] and the underlying `claude`
// subprocess. Production uses [defaultRunner], which calls
// [exec.CommandContext]; tests inject a fake to drive every error path
// hermetically.
type Runner interface {
	// Run invokes `claude` with the given args. stdin is optional; if
	// non-nil it is piped to the child's stdin. Returns the child's
	// stdout. A non-nil error indicates exec failure or non-zero exit
	// code; stderr is wrapped into the error.
	Run(ctx context.Context, args []string, stdin []byte) ([]byte, error)
}

// defaultRunner is the production Runner — calls the real `claude`
// binary via [exec.CommandContext].
var defaultRunner Runner = realRunner{}

type realRunner struct{}

func (realRunner) Run(ctx context.Context, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "claude", args...) //#nosec G204 -- args are constructed from a fixed flag set; no user input
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.Bytes()
	if err != nil {
		// `claude -p --output-format json` returns exit 1 on
		// structured errors (e.g. error_max_budget_usd, error_max_turns)
		// but still writes a parseable JSON object to stdout. If we
		// have stdout, surface it; the caller's own JSON parse logic
		// will pull the `subtype` out and log a useful message.
		if len(out) > 0 {
			return out, nil
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, &runnerError{err: err, stderr: stderr.String()}
		}
		return nil, err
	}
	return out, nil
}

type runnerError struct {
	err    error
	stderr string
}

func (e *runnerError) Error() string {
	if e.stderr == "" {
		return e.err.Error()
	}
	return e.err.Error() + ": " + truncate(e.stderr, 200)
}

func (e *runnerError) Unwrap() error { return e.err }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
