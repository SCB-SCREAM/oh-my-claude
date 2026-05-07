// Package detect runs file-only stack detection over a project's filesystem
// and emits structured [Signal]s describing the languages, package managers,
// frameworks, infra, CI systems, and monorepo tools it found.
//
// The package is intentionally airtight: detectors never shell out, never
// read user source code (only manifests and config files), and never see
// the underlying [fs.FS] directly — they receive a shared, pre-walked
// [Snapshot]. This keeps the 5,000-file walk cap deterministic and lets
// every signal carry exact, file-grounded evidence.
package detect

import (
	"io/fs"
	"sort"
)

// Signal is a single piece of stack evidence extracted from a project.
// Detectors emit zero or more Signals; the runner deduplicates and sorts
// them before returning to callers.
type Signal struct {
	// Name is the canonical, lower-case identifier of the thing detected
	// ("typescript", "next.js", "github-actions"). Stable across versions.
	Name string

	// Confidence is one of the [Conf*] tier constants. Higher means more
	// definitive (a lockfile beats a guess from a single config file).
	Confidence float64

	// Evidence is the repo-relative, slash-separated, sorted list of files
	// that triggered the signal. Always at least one entry; capped at 5.
	Evidence []string

	// Tags classify the signal: "language", "package-manager", "framework",
	// "db", "orm", "infra", "ci", "monorepo".
	Tags []string
}

// Detector inspects a [Snapshot] and emits zero or more [Signal]s. A
// detector that cannot decide returns nil — never an error.
type Detector interface {
	ID() string
	Detect(snap *Snapshot) []Signal
}

// detectorFunc adapts a plain function to the Detector interface so each
// category file can register its detectors without boilerplate types.
type detectorFunc struct {
	id string
	fn func(*Snapshot) []Signal
}

func (d detectorFunc) ID() string                     { return d.id }
func (d detectorFunc) Detect(snap *Snapshot) []Signal { return d.fn(snap) }

// registry holds every detector registered via init() in this package.
var registry []Detector

// register appends d to the package-level detector registry. Called from
// init() in each category file. Panics if d.ID() is duplicated — the
// registry is not a multimap.
func register(d Detector) {
	for _, existing := range registry {
		if existing.ID() == d.ID() {
			panic("detect: duplicate detector ID " + d.ID())
		}
	}
	registry = append(registry, d)
}

// Run walks fsys, builds a [Snapshot], invokes every registered detector,
// and returns a sorted, deduplicated [Signal] slice. fsys must be rooted
// at the repository root; Run never escapes that root.
func Run(fsys fs.FS) ([]Signal, *Snapshot, error) {
	snap, err := NewSnapshot(fsys)
	if err != nil {
		return nil, nil, err
	}
	return RunWith(registry, snap), snap, nil
}

// RunWith executes the supplied detectors against snap. Used directly by
// tests so they can inject a controlled detector set without registration
// side-effects from other files.
func RunWith(detectors []Detector, snap *Snapshot) []Signal {
	var out []Signal
	seen := map[string]int{}
	for _, d := range detectors {
		for _, sig := range d.Detect(snap) {
			if sig.Name == "" || len(sig.Evidence) == 0 {
				continue
			}
			normalizeSignal(&sig)
			if idx, ok := seen[sig.Name]; ok {
				out[idx] = mergeSignals(out[idx], sig)
				continue
			}
			seen[sig.Name] = len(out)
			out = append(out, sig)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// normalizeSignal sorts and caps evidence and dedupes tags, in place.
func normalizeSignal(s *Signal) {
	s.Evidence = uniqueSorted(s.Evidence)
	const evidenceCap = 5
	if len(s.Evidence) > evidenceCap {
		s.Evidence = s.Evidence[:evidenceCap]
	}
	s.Tags = uniqueSorted(s.Tags)
}

// mergeSignals combines two signals with the same Name. Confidence is the
// max of the two; evidence and tags are unioned, then renormalized.
func mergeSignals(a, b Signal) Signal {
	out := Signal{
		Name:       a.Name,
		Confidence: max(a.Confidence, b.Confidence),
		Evidence:   append(append([]string{}, a.Evidence...), b.Evidence...),
		Tags:       append(append([]string{}, a.Tags...), b.Tags...),
	}
	normalizeSignal(&out)
	return out
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
