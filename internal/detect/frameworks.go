package detect

func init() {
	register(detectorFunc{id: "next.js", fn: detectNext})
	catalog(CatalogEntry{
		ID: "next.js", Tags: []string{"framework"},
		Triggers: []string{"next dep in package.json", "next.config.{js,ts,mjs}"},
		MinConf:  ConfFileConvention, MaxConf: ConfManifest,
		Summary: "Next.js — also emits a free react signal",
	})

	register(detectorFunc{id: "react", fn: detectReact})
	catalog(CatalogEntry{
		ID: "react", Tags: []string{"framework"},
		Triggers: []string{"react dep in package.json"},
		MinConf:  ConfDepDeclared, MaxConf: ConfDepDeclared,
		Summary: "React — only via direct dep declaration",
	})

	register(detectorFunc{id: "vite", fn: detectVite})
	catalog(CatalogEntry{
		ID: "vite", Tags: []string{"framework"},
		Triggers: []string{"vite dep in package.json", "vite.config.{js,ts,mjs}"},
		MinConf:  ConfFileConvention, MaxConf: ConfManifest,
		Summary: "Vite build tooling",
	})

	register(detectorFunc{id: "express", fn: detectExpress})
	catalog(CatalogEntry{
		ID: "express", Tags: []string{"framework"},
		Triggers: []string{"express dep in package.json"},
		MinConf:  ConfDepDeclared, MaxConf: ConfDepDeclared,
		Summary: "Express HTTP server",
	})

	register(detectorFunc{id: "nestjs", fn: detectNestJS})
	catalog(CatalogEntry{
		ID: "nestjs", Tags: []string{"framework"},
		Triggers: []string{"@nestjs/core dep", "nest-cli.json"},
		MinConf:  ConfDepDeclared, MaxConf: ConfManifest,
		Summary: "NestJS framework",
	})

	register(detectorFunc{id: "django", fn: detectDjango})
	catalog(CatalogEntry{
		ID: "django", Tags: []string{"framework"},
		Triggers: []string{"django dep in pyproject/requirements", "manage.py"},
		MinConf:  ConfDepDeclared, MaxConf: ConfManifest,
		Summary: "Django web framework",
	})

	register(detectorFunc{id: "fastapi", fn: detectFastAPI})
	catalog(CatalogEntry{
		ID: "fastapi", Tags: []string{"framework"},
		Triggers: []string{"fastapi dep in pyproject/requirements"},
		MinConf:  ConfDepDeclared, MaxConf: ConfDepDeclared,
		Summary: "FastAPI Python framework",
	})

	register(detectorFunc{id: "flask", fn: detectFlask})
	catalog(CatalogEntry{
		ID: "flask", Tags: []string{"framework"},
		Triggers: []string{"flask dep in pyproject/requirements"},
		MinConf:  ConfDepDeclared, MaxConf: ConfDepDeclared,
		Summary: "Flask Python framework",
	})
}

func detectNext(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0

	if _, ok := s.HasDep("npm", "next"); ok {
		ev = append(ev, s.Deps().Manifests("npm")...)
		conf = max(conf, ConfManifest)
	}
	for _, name := range []string{"next.config.js", "next.config.ts", "next.config.mjs", "next.config.cjs"} {
		if p, ok := s.Has(name); ok {
			ev = append(ev, p)
			conf = max(conf, ConfFileConvention)
		}
	}
	if len(ev) == 0 {
		return nil
	}
	out := []Signal{{
		Name: "next.js", Confidence: conf,
		Evidence: ev, Tags: []string{"framework"},
	}}
	// Companion signal: every Next.js install pulls React. Emit a
	// react signal so downstream components keyed on React still apply.
	if _, ok := s.HasDep("npm", "react"); ok {
		out = append(out, Signal{
			Name: "react", Confidence: ConfDepDeclared,
			Evidence: s.Deps().Manifests("npm"), Tags: []string{"framework"},
		})
	}
	return out
}

func detectReact(s *Snapshot) []Signal {
	if _, ok := s.HasDep("npm", "react"); !ok {
		return nil
	}
	return []Signal{{
		Name: "react", Confidence: ConfDepDeclared,
		Evidence: s.Deps().Manifests("npm"), Tags: []string{"framework"},
	}}
}

func detectVite(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0
	if _, ok := s.HasDep("npm", "vite"); ok {
		ev = append(ev, s.Deps().Manifests("npm")...)
		conf = max(conf, ConfManifest)
	}
	for _, name := range []string{"vite.config.js", "vite.config.ts", "vite.config.mjs"} {
		if p, ok := s.Has(name); ok {
			ev = append(ev, p)
			conf = max(conf, ConfFileConvention)
		}
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "vite", Confidence: conf,
		Evidence: ev, Tags: []string{"framework"},
	}}
}

func detectExpress(s *Snapshot) []Signal {
	if _, ok := s.HasDep("npm", "express"); !ok {
		return nil
	}
	return []Signal{{
		Name: "express", Confidence: ConfDepDeclared,
		Evidence: s.Deps().Manifests("npm"), Tags: []string{"framework"},
	}}
}

func detectNestJS(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0
	if _, ok := s.HasDep("npm", "@nestjs/core"); ok {
		ev = append(ev, s.Deps().Manifests("npm")...)
		conf = max(conf, ConfDepDeclared)
	}
	if p, ok := s.Has("nest-cli.json"); ok {
		ev = append(ev, p)
		conf = max(conf, ConfManifest)
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "nestjs", Confidence: conf,
		Evidence: ev, Tags: []string{"framework"},
	}}
}

func detectDjango(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0
	if _, ok := s.HasDep("python", "django"); ok {
		ev = append(ev, s.Deps().Manifests("python")...)
		conf = max(conf, ConfDepDeclared)
	}
	if p, ok := s.Has("manage.py"); ok {
		ev = append(ev, p)
		conf = max(conf, ConfManifest)
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "django", Confidence: conf,
		Evidence: ev, Tags: []string{"framework"},
	}}
}

func detectFastAPI(s *Snapshot) []Signal {
	if _, ok := s.HasDep("python", "fastapi"); !ok {
		return nil
	}
	return []Signal{{
		Name: "fastapi", Confidence: ConfDepDeclared,
		Evidence: s.Deps().Manifests("python"), Tags: []string{"framework"},
	}}
}

func detectFlask(s *Snapshot) []Signal {
	if _, ok := s.HasDep("python", "flask"); !ok {
		return nil
	}
	return []Signal{{
		Name: "flask", Confidence: ConfDepDeclared,
		Evidence: s.Deps().Manifests("python"), Tags: []string{"framework"},
	}}
}
