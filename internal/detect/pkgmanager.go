package detect

func init() {
	register(detectorFunc{id: "npm", fn: detectNpm})
	catalog(CatalogEntry{
		ID: "npm", Tags: []string{"package-manager"},
		Triggers: []string{"package-lock.json"},
		MinConf:  ConfLockfile, MaxConf: ConfLockfile,
		Summary: "npm package manager (package-lock.json present)",
	})

	register(detectorFunc{id: "yarn", fn: detectYarn})
	catalog(CatalogEntry{
		ID: "yarn", Tags: []string{"package-manager"},
		Triggers: []string{"yarn.lock"},
		MinConf:  ConfLockfile, MaxConf: ConfLockfile,
		Summary: "Yarn package manager (yarn.lock present)",
	})

	register(detectorFunc{id: "pnpm", fn: detectPnpm})
	catalog(CatalogEntry{
		ID: "pnpm", Tags: []string{"package-manager"},
		Triggers: []string{"pnpm-lock.yaml"},
		MinConf:  ConfLockfile, MaxConf: ConfLockfile,
		Summary: "pnpm package manager (pnpm-lock.yaml present)",
	})

	register(detectorFunc{id: "bun", fn: detectBun})
	catalog(CatalogEntry{
		ID: "bun", Tags: []string{"package-manager"},
		Triggers: []string{"bun.lockb", "bun.lock"},
		MinConf:  ConfLockfile, MaxConf: ConfLockfile,
		Summary: "Bun package manager / runtime",
	})

	register(detectorFunc{id: "pip", fn: detectPip})
	catalog(CatalogEntry{
		ID: "pip", Tags: []string{"package-manager"},
		Triggers: []string{"requirements*.txt", "no poetry/uv lockfile"},
		MinConf:  ConfFileConvention, MaxConf: ConfManifest,
		Summary: "pip / venv-style Python project",
	})

	register(detectorFunc{id: "poetry", fn: detectPoetry})
	catalog(CatalogEntry{
		ID: "poetry", Tags: []string{"package-manager"},
		Triggers: []string{"poetry.lock", "[tool.poetry] section in pyproject.toml"},
		MinConf:  ConfManifest, MaxConf: ConfLockfile,
		Summary: "Poetry-managed Python project",
	})

	register(detectorFunc{id: "uv", fn: detectUv})
	catalog(CatalogEntry{
		ID: "uv", Tags: []string{"package-manager"},
		Triggers: []string{"uv.lock"},
		MinConf:  ConfLockfile, MaxConf: ConfLockfile,
		Summary: "uv-managed Python project (uv.lock present)",
	})
}

func detectNpm(s *Snapshot) []Signal {
	if p, ok := s.Has("package-lock.json"); ok {
		return []Signal{{
			Name: "npm", Confidence: ConfLockfile,
			Evidence: []string{p}, Tags: []string{"package-manager"},
		}}
	}
	return nil
}

func detectYarn(s *Snapshot) []Signal {
	if p, ok := s.Has("yarn.lock"); ok {
		return []Signal{{
			Name: "yarn", Confidence: ConfLockfile,
			Evidence: []string{p}, Tags: []string{"package-manager"},
		}}
	}
	return nil
}

func detectPnpm(s *Snapshot) []Signal {
	if p, ok := s.Has("pnpm-lock.yaml"); ok {
		return []Signal{{
			Name: "pnpm", Confidence: ConfLockfile,
			Evidence: []string{p}, Tags: []string{"package-manager"},
		}}
	}
	return nil
}

func detectBun(s *Snapshot) []Signal {
	for _, name := range []string{"bun.lockb", "bun.lock"} {
		if p, ok := s.Has(name); ok {
			return []Signal{{
				Name: "bun", Confidence: ConfLockfile,
				Evidence: []string{p}, Tags: []string{"package-manager"},
			}}
		}
	}
	return nil
}

func detectPip(s *Snapshot) []Signal {
	// Pip is the fallback when we see Python deps but no Poetry/uv
	// markers. We don't emit pip alongside Poetry/uv to keep the TUI
	// from showing two competing package managers.
	if _, ok := s.Has("poetry.lock"); ok {
		return nil
	}
	if _, ok := s.Has("uv.lock"); ok {
		return nil
	}

	var ev []string
	conf := 0.0
	for _, p := range s.ByName["requirements.txt"] {
		ev = append(ev, p)
		conf = max(conf, ConfManifest)
	}
	for _, p := range s.Files {
		base := pathBase(p)
		if base != "requirements.txt" && len(base) > len("requirements-") &&
			base[:13] == "requirements-" && base[len(base)-4:] == ".txt" {
			ev = append(ev, p)
			conf = max(conf, ConfManifest)
		}
	}
	if p, ok := s.Has("Pipfile"); ok {
		ev = append(ev, p)
		conf = max(conf, ConfManifest)
	}
	if p, ok := s.Has("Pipfile.lock"); ok {
		ev = append(ev, p)
		conf = ConfLockfile
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "pip", Confidence: conf,
		Evidence: ev, Tags: []string{"package-manager"},
	}}
}

func detectPoetry(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0
	if p, ok := s.Has("poetry.lock"); ok {
		ev = append(ev, p)
		conf = ConfLockfile
	}
	// Look for [tool.poetry] in pyproject.toml — the dep index already
	// parsed it; we can detect Poetry via any dep recorded *and* the
	// "tool.poetry" section having been hit, but to keep this cheap we
	// just re-scan the manifest text.
	for _, p := range s.ByName["pyproject.toml"] {
		if data, err := s.Read(p); err == nil {
			if containsLine(data, "[tool.poetry]") ||
				containsLine(data, "[tool.poetry.dependencies]") {
				ev = append(ev, p)
				conf = max(conf, ConfManifest)
			}
		}
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "poetry", Confidence: conf,
		Evidence: ev, Tags: []string{"package-manager"},
	}}
}

func detectUv(s *Snapshot) []Signal {
	if p, ok := s.Has("uv.lock"); ok {
		return []Signal{{
			Name: "uv", Confidence: ConfLockfile,
			Evidence: []string{p}, Tags: []string{"package-manager"},
		}}
	}
	return nil
}

// containsLine reports whether data contains the given line as a complete
// line (after trimming whitespace). Used by the Poetry detector to spot
// `[tool.poetry]` without doing a full TOML parse.
func containsLine(data []byte, want string) bool {
	for i := 0; i < len(data); {
		end := i
		for end < len(data) && data[end] != '\n' {
			end++
		}
		line := string(data[i:end])
		// Trim CR for CRLF files.
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		// Trim leading/trailing spaces.
		j := 0
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		k := len(line)
		for k > j && (line[k-1] == ' ' || line[k-1] == '\t') {
			k--
		}
		if line[j:k] == want {
			return true
		}
		i = end + 1
	}
	return false
}
