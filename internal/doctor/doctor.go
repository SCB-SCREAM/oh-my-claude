// Package doctor runs read-only diagnostics against the current project +
// environment to flag common Claude Code setup problems. It never writes,
// shells out to user code, or fails the process — findings are returned as
// a slice of [Finding]s the caller (typically `omc doctor`) renders.
package doctor

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Severity ranks a finding from informational to actionable.
type Severity int

const (
	// SeverityOK indicates a successful check; rendered with a checkmark.
	SeverityOK Severity = iota
	// SeverityInfo is a neutral observation — no action implied.
	SeverityInfo
	// SeverityWarn means the user should probably fix it but nothing's broken.
	SeverityWarn
	// SeverityError is a definite problem the user should address.
	SeverityError
)

// Finding is a single diagnostic result from a check.
type Finding struct {
	Name     string   // short check name, e.g. "claude-cli-on-path"
	Severity Severity //
	Message  string   // human-readable summary
	Hint     string   // optional remediation hint, shown indented under Message
}

// Run executes every check and returns the collected findings. `repoRoot` is
// the directory to evaluate (typically the cwd).
func Run(repoRoot string) []Finding {
	checks := []func(string) Finding{
		checkGitRepo,
		checkClaudeCLI,
		checkClaudeAuth,
		checkSettingsLocalGitignored,
		checkClaudeDir,
	}
	out := make([]Finding, 0, len(checks))
	for _, c := range checks {
		out = append(out, c(repoRoot))
	}
	return out
}

func checkGitRepo(root string) Finding {
	gitDir := filepath.Join(root, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		return Finding{Name: "git-repo", Severity: SeverityOK, Message: "in a git repository"}
	}
	return Finding{
		Name:     "git-repo",
		Severity: SeverityWarn,
		Message:  "not in a git repository",
		Hint:     "run `git init` so omc's backups and .gitignore writes have a target",
	}
}

func checkClaudeCLI(_ string) Finding {
	path, err := exec.LookPath("claude")
	if err == nil {
		return Finding{
			Name:     "claude-cli-on-path",
			Severity: SeverityOK,
			Message:  "claude CLI found at " + path,
		}
	}
	// claude is a HARD dependency for omc — detection is LLM-backed, so
	// without claude on $PATH `omc init` cannot do its primary job.
	return Finding{
		Name:     "claude-cli-on-path",
		Severity: SeverityError,
		Message:  "claude CLI not found on $PATH",
		Hint:     "install from https://docs.claude.com/en/docs/claude-code; omc requires claude for project detection",
	}
}

// checkClaudeAuth runs `claude auth status` to verify the user has an
// active subscription session. omc deliberately does not fall back to
// ANTHROPIC_API_KEY (would bill per call), so unauthenticated is a hard
// error: `omc init` will refuse to run.
func checkClaudeAuth(_ string) Finding {
	if _, err := exec.LookPath("claude"); err != nil {
		// Already covered by checkClaudeCLI; skip to avoid duplicate noise.
		return Finding{
			Name:     "claude-authenticated",
			Severity: SeverityInfo,
			Message:  "skipped (claude CLI not installed)",
		}
	}
	// #nosec G204 -- fixed args, no user input
	cmd := exec.Command("claude", "auth", "status")
	if err := cmd.Run(); err != nil {
		return Finding{
			Name:     "claude-authenticated",
			Severity: SeverityError,
			Message:  "claude is installed but no active subscription session",
			Hint:     "run `claude auth login` (omc never falls back to ANTHROPIC_API_KEY billing)",
		}
	}
	return Finding{
		Name:     "claude-authenticated",
		Severity: SeverityOK,
		Message:  "claude subscription session is active",
	}
}

func checkSettingsLocalGitignored(root string) Finding {
	settingsLocal := filepath.Join(root, ".claude", "settings.local.json")
	if _, err := os.Stat(settingsLocal); errors.Is(err, fs.ErrNotExist) {
		return Finding{
			Name:     "settings-local-gitignored",
			Severity: SeverityInfo,
			Message:  "no .claude/settings.local.json present (nothing to gitignore yet)",
		}
	}

	gitignore := filepath.Join(root, ".gitignore")
	f, err := os.Open(gitignore) //#nosec G304 -- repo-relative .gitignore
	if err != nil {
		return Finding{
			Name:     "settings-local-gitignored",
			Severity: SeverityWarn,
			Message:  ".claude/settings.local.json exists but no .gitignore found",
			Hint:     "create .gitignore and add `.claude/settings.local.json`",
		}
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == ".claude/settings.local.json" || line == "**/.claude/settings.local.json" {
			return Finding{
				Name:     "settings-local-gitignored",
				Severity: SeverityOK,
				Message:  ".claude/settings.local.json is gitignored",
			}
		}
	}
	return Finding{
		Name:     "settings-local-gitignored",
		Severity: SeverityError,
		Message:  ".claude/settings.local.json is NOT gitignored — it may contain secrets",
		Hint:     "add `.claude/settings.local.json` to .gitignore",
	}
}

func checkClaudeDir(root string) Finding {
	claudeDir := filepath.Join(root, ".claude")
	info, err := os.Stat(claudeDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Finding{
			Name:     "claude-dir",
			Severity: SeverityInfo,
			Message:  "no .claude/ directory yet — run `omc init` to bootstrap",
		}
	case err != nil:
		return Finding{
			Name:     "claude-dir",
			Severity: SeverityError,
			Message:  "could not read .claude/: " + err.Error(),
		}
	case !info.IsDir():
		return Finding{
			Name:     "claude-dir",
			Severity: SeverityError,
			Message:  ".claude exists but is not a directory",
		}
	default:
		return Finding{
			Name:     "claude-dir",
			Severity: SeverityOK,
			Message:  ".claude/ directory present",
		}
	}
}

// Symbol returns the leading glyph used to render a finding in the CLI.
func (s Severity) Symbol() string {
	switch s {
	case SeverityOK:
		return "✓"
	case SeverityInfo:
		return "·"
	case SeverityWarn:
		return "!"
	case SeverityError:
		return "✗"
	default:
		return "?"
	}
}
