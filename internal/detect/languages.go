package detect

func init() {
	register(detectorFunc{id: "typescript", fn: detectTypeScript})
	catalog(CatalogEntry{
		ID: "typescript", Tags: []string{"language"},
		Triggers: []string{"tsconfig.json", "*.ts / *.tsx files", "typescript dep in package.json"},
		MinConf:  ConfFileConvention, MaxConf: ConfManifest,
		Summary: "TypeScript / TSX sources or a tsconfig.json",
	})

	register(detectorFunc{id: "javascript", fn: detectJavaScript})
	catalog(CatalogEntry{
		ID: "javascript", Tags: []string{"language"},
		Triggers: []string{"package.json without typescript dep", "*.js / *.jsx / *.mjs / *.cjs files"},
		MinConf:  ConfFileConvention, MaxConf: ConfManifest,
		Summary: "JavaScript sources without a TypeScript signal",
	})

	register(detectorFunc{id: "python", fn: detectPython})
	catalog(CatalogEntry{
		ID: "python", Tags: []string{"language"},
		Triggers: []string{"pyproject.toml", "setup.py", "requirements.txt", "*.py files"},
		MinConf:  ConfFileConvention, MaxConf: ConfManifest,
		Summary: "Python project with a recognised manifest or sources",
	})

	register(detectorFunc{id: "go", fn: detectGo})
	catalog(CatalogEntry{
		ID: "go", Tags: []string{"language"},
		Triggers: []string{"go.mod", "go.sum"},
		MinConf:  ConfManifest, MaxConf: ConfLockfile,
		Summary: "Go module — go.mod present",
	})
}

func detectTypeScript(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0

	if p, ok := s.Has("tsconfig.json"); ok {
		ev = append(ev, p)
		conf = max(conf, ConfFileConvention)
	}
	if _, ok := s.HasDep("npm", "typescript"); ok {
		// We have a package.json that *declares* TypeScript — that's
		// stronger than any single file convention.
		ev = append(ev, s.Deps().Manifests("npm")...)
		conf = max(conf, ConfManifest)
	}
	if hasAnyExt(s, ".ts", ".tsx") {
		// Bare *.ts files alone aren't enough to outscore a missing
		// tsconfig — they could be ambient typings inside a JS project.
		// But they make a convention-level signal stronger.
		conf = max(conf, ConfFileConvention)
		ev = append(ev, firstFew(s.ByExt[".ts"], s.ByExt[".tsx"])...)
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "typescript", Confidence: conf,
		Evidence: ev, Tags: []string{"language"},
	}}
}

func detectJavaScript(s *Snapshot) []Signal {
	// A JS signal is most useful when we can clearly say "JS, not TS".
	// We still emit it even if TS is also present (a repo can have both)
	// but we cap it at ConfFileConvention so TS comes first in the sort.
	var ev []string
	conf := 0.0

	if _, ok := s.Has("package.json"); ok {
		// package.json with no typescript dep → JavaScript signal.
		if _, hasTS := s.HasDep("npm", "typescript"); !hasTS {
			ev = append(ev, s.Deps().Manifests("npm")...)
			conf = max(conf, ConfManifest)
		}
	}
	if hasAnyExt(s, ".js", ".jsx", ".mjs", ".cjs") {
		ev = append(ev, firstFew(s.ByExt[".js"], s.ByExt[".jsx"], s.ByExt[".mjs"], s.ByExt[".cjs"])...)
		conf = max(conf, ConfFileConvention)
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "javascript", Confidence: conf,
		Evidence: ev, Tags: []string{"language"},
	}}
}

func detectPython(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0

	for _, name := range []string{"pyproject.toml", "setup.py", "setup.cfg", "Pipfile", "requirements.txt"} {
		if p, ok := s.Has(name); ok {
			ev = append(ev, p)
			conf = max(conf, ConfManifest)
		}
	}
	// Any requirements-*.txt also counts.
	for _, p := range s.ByName["requirements-dev.txt"] {
		ev = append(ev, p)
		conf = max(conf, ConfManifest)
	}
	if hasAnyExt(s, ".py") {
		ev = append(ev, firstFew(s.ByExt[".py"])...)
		conf = max(conf, ConfFileConvention)
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "python", Confidence: conf,
		Evidence: ev, Tags: []string{"language"},
	}}
}

func detectGo(s *Snapshot) []Signal {
	gomod, ok := s.Has("go.mod")
	if !ok {
		return nil
	}
	ev := []string{gomod}
	conf := ConfManifest
	if p, ok := s.Has("go.sum"); ok {
		ev = append(ev, p)
		conf = ConfLockfile
	}
	return []Signal{{
		Name: "go", Confidence: conf,
		Evidence: ev, Tags: []string{"language"},
	}}
}

// hasAnyExt reports whether the snapshot indexes any file with one of the
// given lower-cased extensions.
func hasAnyExt(s *Snapshot, exts ...string) bool {
	for _, e := range exts {
		if len(s.ByExt[e]) > 0 {
			return true
		}
	}
	return false
}

// firstFew returns up to 3 entries combined from the supplied slices, in
// order. Used so language detectors don't emit thousands of *.py paths
// as evidence.
func firstFew(slices ...[]string) []string {
	const limit = 3
	var out []string
	for _, s := range slices {
		for _, p := range s {
			if len(out) >= limit {
				return out
			}
			out = append(out, p)
		}
	}
	return out
}
