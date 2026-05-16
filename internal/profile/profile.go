// Package profile names the curated component sets omc ships out of the
// box and the table mapping each profile to its baseline component IDs.
//
// profile is deliberately a leaf package: it owns the names, the human
// blurbs, and the per-profile ID list but knows nothing about the
// Component type itself. Resolution against an actual catalog lives in
// internal/session so that profile and internal/component never need to
// import one another.
package profile

import "errors"

// Name is one of the canonical profile names. Use the typed constants
// below rather than free-form strings.
type Name string

const (
	Minimal     Name = "minimal"
	Recommended Name = "recommended"
	Full        Name = "full"
)

// ErrUnknown is wrapped by Validate / IDs when an unrecognised profile
// is supplied.
var ErrUnknown = errors.New("unknown profile")

// All returns the canonical, ordered list of valid profile names. The
// order drives the TUI profile screen and shell-completion output.
func All() []Name {
	return []Name{Minimal, Recommended, Full}
}

// Validate returns nil if p is one of the known profile names, or
// ErrUnknown otherwise.
func Validate(p Name) error {
	for _, k := range All() {
		if k == p {
			return nil
		}
	}
	return ErrUnknown
}

// Describe returns a one-paragraph human blurb shown in the TUI profile
// screen's right pane and in `omc init --help`. Wording is part of the
// public surface — golden tests assert on it.
func Describe(p Name) string {
	switch p {
	case Minimal:
		return "Minimal — only CLAUDE.md and a permissive settings.json. " +
			"Pick this if you already manage Claude Code by hand and just want a stack-aware starting point."
	case Recommended:
		return "Recommended — minimal plus the most-used slash commands (/test, /lint) and a .gitignore nudge. " +
			"The right default for most teams; everything else is opt-in."
	case Full:
		return "Full — recommended plus /format and a PostToolUse format-on-save hook. " +
			"Hooks execute on your machine, so review .claude/hooks/* before approving."
	}
	return ""
}

// IDs returns the baseline component IDs that the profile pre-selects.
// The returned slice is in stable, deterministic order — it drives the
// pre-checked state of the components screen.
//
// IDs returns nil + ErrUnknown for an unknown profile name. It does NOT
// filter against any real catalog; that's session.ResolveAndMaterialize's
// job, which intersects this list with component.Materialize(signals).
func IDs(p Name) ([]string, error) {
	if err := Validate(p); err != nil {
		return nil, err
	}
	out := make([]string, len(profileMatrix[p]))
	copy(out, profileMatrix[p])
	return out, nil
}

// profileMatrix is the canonical per-profile component-id list. The
// strings here are component IDs; the component package owns the type.
// Adding or removing an entry here flips a checkbox by default — keep
// it small and conservative.
var profileMatrix = map[Name][]string{
	Minimal: {
		"claude-md",
		"settings",
	},
	Recommended: {
		"claude-md",
		"settings",
		"gitignore",
		"cmd.test",
		"cmd.lint",
	},
	Full: {
		"claude-md",
		"settings",
		"gitignore",
		"cmd.test",
		"cmd.lint",
		"cmd.format",
		"hook.format-on-save",
	},
}
