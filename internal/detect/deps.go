package detect

import (
	"bufio"
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// DepIndex is a flat, ecosystem-keyed view of every dependency declared in
// every manifest a [Snapshot] indexed. Built lazily on first access.
//
// Ecosystem keys: "npm" (Node packages), "python" (PyPI), "go" (Go module
// paths).
type DepIndex struct {
	// deps[ecosystem][name] = raw constraint/version string. Names are
	// normalized to lower-case for ecosystems where the registry is
	// case-insensitive (npm, PyPI); Go module paths preserve case.
	deps map[string]map[string]string

	// manifests records, per ecosystem, the manifest file paths that
	// contributed deps. Used as evidence by detectors that just need to
	// say "this looks like an X project".
	manifests map[string][]string
}

// Lookup returns (constraint, true) if name was declared in the given
// ecosystem; otherwise ("", false). Names are case-insensitive for npm
// and python.
func (d *DepIndex) Lookup(ecosystem, name string) (string, bool) {
	if d == nil {
		return "", false
	}
	m, ok := d.deps[ecosystem]
	if !ok {
		return "", false
	}
	if normalizeDepName(ecosystem, name) != name {
		name = normalizeDepName(ecosystem, name)
	}
	v, ok := m[name]
	return v, ok
}

// Manifests returns the list of manifest file paths the index parsed for
// the given ecosystem. Mostly useful as evidence in detectors that gate on
// "any manifest present".
func (d *DepIndex) Manifests(ecosystem string) []string {
	if d == nil {
		return nil
	}
	return append([]string{}, d.manifests[ecosystem]...)
}

// buildDepIndex scans the snapshot for known manifest types and parses
// each. Per-file errors are non-fatal: a malformed package.json should not
// cause a total detection failure.
func buildDepIndex(s *Snapshot) (*DepIndex, error) {
	idx := &DepIndex{
		deps:      map[string]map[string]string{},
		manifests: map[string][]string{},
	}

	for _, p := range s.ByName["package.json"] {
		if data, err := s.Read(p); err == nil {
			parsePackageJSON(idx, p, data)
		}
	}
	for _, p := range s.ByName["pyproject.toml"] {
		if data, err := s.Read(p); err == nil {
			parsePyprojectTOML(idx, p, data)
		}
	}
	for _, p := range s.ByName["setup.py"] {
		if data, err := s.Read(p); err == nil {
			parseSetupPy(idx, p, data)
		}
	}
	for _, p := range s.Files {
		base := pathBase(p)
		if base == "requirements.txt" || strings.HasPrefix(base, "requirements-") && strings.HasSuffix(base, ".txt") {
			if data, err := s.Read(p); err == nil {
				parseRequirementsTxt(idx, p, data)
			}
		}
	}
	for _, p := range s.ByName["go.mod"] {
		if data, err := s.Read(p); err == nil {
			parseGoMod(idx, p, data)
		}
	}
	return idx, nil
}

func pathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func recordDep(idx *DepIndex, ecosystem, name, constraint string) {
	if name == "" {
		return
	}
	name = normalizeDepName(ecosystem, name)
	m, ok := idx.deps[ecosystem]
	if !ok {
		m = map[string]string{}
		idx.deps[ecosystem] = m
	}
	if existing, ok := m[name]; ok && existing != "" && constraint == "" {
		return
	}
	m[name] = constraint
}

func recordManifest(idx *DepIndex, ecosystem, path string) {
	for _, p := range idx.manifests[ecosystem] {
		if p == path {
			return
		}
	}
	idx.manifests[ecosystem] = append(idx.manifests[ecosystem], path)
}

func normalizeDepName(ecosystem, name string) string {
	switch ecosystem {
	case "npm", "python":
		return strings.ToLower(strings.TrimSpace(name))
	default:
		return strings.TrimSpace(name)
	}
}

// ── package.json ────────────────────────────────────────────────────────

type packageJSON struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

func parsePackageJSON(idx *DepIndex, path string, data []byte) {
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return
	}
	recordManifest(idx, "npm", path)
	for _, m := range []map[string]string{
		pkg.Dependencies, pkg.DevDependencies,
		pkg.PeerDependencies, pkg.OptionalDependencies,
	} {
		for k, v := range m {
			recordDep(idx, "npm", k, v)
		}
	}
}

// ── pyproject.toml ──────────────────────────────────────────────────────
//
// We don't import a full TOML parser for this — pyproject.toml dep blocks
// are highly regular. We extract:
//   - PEP 621 [project].dependencies (a TOML array of strings)
//   - [project.optional-dependencies.<group>] arrays
//   - [tool.poetry.dependencies] table
//   - [tool.poetry.group.<group>.dependencies] tables
//   - [build-system].requires
//
// Anything we miss falls through silently — the worst that happens is a
// detector loses one piece of evidence, which is recoverable from the
// manifest's mere existence.

var (
	reSection     = regexp.MustCompile(`^\[([^\]]+)\]\s*$`)
	reArrayOpen   = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_-]*)\s*=\s*\[`)
	reTableEntry  = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*)\s*=\s*(.+)$`)
	rePyDepString = regexp.MustCompile(`(?i)^([A-Za-z0-9._-]+)`)
)

func parsePyprojectTOML(idx *DepIndex, path string, data []byte) {
	recordManifest(idx, "python", path)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		section   string
		inArray   bool
		arrayName string
		arrayBuf  strings.Builder
	)

	flushArray := func() {
		if arrayName == "" {
			return
		}
		switch {
		case section == "project" && arrayName == "dependencies",
			strings.HasPrefix(section, "project.optional-dependencies"):
			for _, line := range splitTOMLArray(arrayBuf.String()) {
				if name := pyReqName(line); name != "" {
					recordDep(idx, "python", name, line)
				}
			}
		case section == "build-system" && arrayName == "requires":
			for _, line := range splitTOMLArray(arrayBuf.String()) {
				if name := pyReqName(line); name != "" {
					recordDep(idx, "python", name, line)
				}
			}
		}
		arrayName = ""
		arrayBuf.Reset()
	}

	for scanner.Scan() {
		raw := scanner.Text()
		line := stripTOMLComment(raw)
		trim := strings.TrimSpace(line)

		if inArray {
			arrayBuf.WriteString(line)
			arrayBuf.WriteByte('\n')
			if strings.Contains(trim, "]") {
				inArray = false
				flushArray()
			}
			continue
		}

		if trim == "" {
			continue
		}
		if m := reSection.FindStringSubmatch(trim); m != nil {
			section = strings.TrimSpace(m[1])
			continue
		}

		// Poetry-style table entries: each line is `name = "constraint"`
		// or `name = { version = "…" }`.
		if section == "tool.poetry.dependencies" ||
			strings.HasPrefix(section, "tool.poetry.group.") {
			if m := reTableEntry.FindStringSubmatch(trim); m != nil {
				name := m[1]
				if name == "python" {
					continue
				}
				val := strings.TrimSpace(m[2])
				recordDep(idx, "python", name, val)
				continue
			}
		}

		if m := reArrayOpen.FindStringSubmatch(trim); m != nil {
			arrayName = m[1]
			arrayBuf.Reset()
			arrayBuf.WriteString(line)
			arrayBuf.WriteByte('\n')
			if strings.Contains(trim, "]") {
				flushArray()
			} else {
				inArray = true
			}
		}
	}
}

// stripTOMLComment removes a trailing `#` comment from a line, ignoring
// `#` chars inside double-quoted strings.
func stripTOMLComment(line string) string {
	inStr := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' && (i == 0 || line[i-1] != '\\') {
			inStr = !inStr
		}
		if c == '#' && !inStr {
			return line[:i]
		}
	}
	return line
}

// splitTOMLArray pulls quoted strings out of a possibly-multi-line TOML
// array body. Returns each string with quotes removed and whitespace
// trimmed.
func splitTOMLArray(body string) []string {
	var out []string
	for {
		i := strings.IndexByte(body, '"')
		if i < 0 {
			return out
		}
		body = body[i+1:]
		j := strings.IndexByte(body, '"')
		if j < 0 {
			return out
		}
		out = append(out, strings.TrimSpace(body[:j]))
		body = body[j+1:]
	}
}

func pyReqName(req string) string {
	req = strings.TrimSpace(req)
	if req == "" {
		return ""
	}
	m := rePyDepString.FindString(req)
	return strings.ToLower(m)
}

// ── requirements.txt ────────────────────────────────────────────────────

func parseRequirementsTxt(idx *DepIndex, path string, data []byte) {
	recordManifest(idx, "python", path)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		// Strip inline comment.
		if i := strings.IndexAny(line, " \t#"); i >= 0 {
			rest := strings.TrimSpace(line[i:])
			if strings.HasPrefix(rest, "#") {
				line = strings.TrimSpace(line[:i])
			}
		}
		if name := pyReqName(line); name != "" {
			recordDep(idx, "python", name, line)
		}
	}
}

// ── setup.py ─────────────────────────────────────────────────────────────
//
// We don't run Python; we just regex over the install_requires/list-form
// arguments to setup(). Misses dynamic deps but those are rare and the
// parent detector typically also matches on pyproject.toml or a known
// framework file.

var reInstallRequires = regexp.MustCompile(`(?s)install_requires\s*=\s*\[([^\]]*)\]`)

func parseSetupPy(idx *DepIndex, path string, data []byte) {
	recordManifest(idx, "python", path)
	m := reInstallRequires.FindSubmatch(data)
	if m == nil {
		return
	}
	for _, item := range splitTOMLArray(string(m[1])) {
		if name := pyReqName(item); name != "" {
			recordDep(idx, "python", name, item)
		}
	}
}

// ── go.mod ───────────────────────────────────────────────────────────────
//
// Tiny line-oriented parser. Two forms:
//
//	require example.com/foo v1.2.3
//	require (
//	    example.com/foo v1.2.3
//	    example.com/bar v0.0.0-…
//	)
//
// We ignore replace/exclude directives — they are not "deps declared with
// real constraints" for our purposes.

func parseGoMod(idx *DepIndex, path string, data []byte) {
	recordManifest(idx, "go", path)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	inBlock := false
	for scanner.Scan() {
		line := strings.TrimSpace(stripGoComment(scanner.Text()))
		if line == "" {
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			recordGoModDep(idx, line)
			continue
		}
		if strings.HasPrefix(line, "require (") {
			inBlock = true
			continue
		}
		if strings.HasPrefix(line, "require ") {
			recordGoModDep(idx, strings.TrimPrefix(line, "require "))
		}
	}
}

func stripGoComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

func recordGoModDep(idx *DepIndex, line string) {
	line = strings.TrimSpace(line)
	if strings.HasSuffix(line, "// indirect") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "// indirect"))
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}
	recordDep(idx, "go", fields[0], fields[1])
}
