package detect

// Confidence tiers for [Signal.Confidence]. Detectors must pick a tier
// rather than invent a value — see the stack-detection skill for rules.
const (
	// ConfLockfile: a lockfile or definitive marker is present
	// (pnpm-lock.yaml, go.sum, uv.lock, *.tf).
	ConfLockfile = 1.00
	// ConfManifest: canonical manifest present
	// (package.json, pyproject.toml, go.mod, Cargo.toml).
	ConfManifest = 0.95
	// ConfDepDeclared: dependency listed in a manifest with a real constraint.
	ConfDepDeclared = 0.85
	// ConfFileConvention: stack-specific file/dir present
	// (next.config.js, manage.py, prisma/schema.prisma).
	ConfFileConvention = 0.70
	// ConfHeuristic: weaker filename match (e.g. any *.tf file → terraform).
	ConfHeuristic = 0.50
	// ConfWeak: last-resort signal; almost never appropriate alone.
	ConfWeak = 0.30
)
