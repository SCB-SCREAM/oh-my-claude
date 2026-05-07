package detect

func init() {
	register(detectorFunc{id: "pnpm-workspace", fn: detectPnpmWorkspace})
	catalog(CatalogEntry{
		ID: "pnpm-workspace", Tags: []string{"monorepo"},
		Triggers: []string{"pnpm-workspace.yaml"},
		MinConf:  ConfFileConvention, MaxConf: ConfFileConvention,
		Summary: "pnpm workspace at the repo root",
	})

	register(detectorFunc{id: "turborepo", fn: detectTurborepo})
	catalog(CatalogEntry{
		ID: "turborepo", Tags: []string{"monorepo"},
		Triggers: []string{"turbo.json"},
		MinConf:  ConfFileConvention, MaxConf: ConfFileConvention,
		Summary: "Turborepo task pipeline",
	})

	register(detectorFunc{id: "nx", fn: detectNx})
	catalog(CatalogEntry{
		ID: "nx", Tags: []string{"monorepo"},
		Triggers: []string{"nx.json"},
		MinConf:  ConfFileConvention, MaxConf: ConfFileConvention,
		Summary: "Nx workspace",
	})

	register(detectorFunc{id: "lerna", fn: detectLerna})
	catalog(CatalogEntry{
		ID: "lerna", Tags: []string{"monorepo"},
		Triggers: []string{"lerna.json"},
		MinConf:  ConfFileConvention, MaxConf: ConfFileConvention,
		Summary: "Lerna monorepo (legacy/maintenance mode)",
	})

	register(detectorFunc{id: "go-workspace", fn: detectGoWorkspace})
	catalog(CatalogEntry{
		ID: "go-workspace", Tags: []string{"monorepo"},
		Triggers: []string{"go.work"},
		MinConf:  ConfManifest, MaxConf: ConfManifest,
		Summary: "Go multi-module workspace (go.work)",
	})
}

func detectPnpmWorkspace(s *Snapshot) []Signal {
	if p, ok := s.Has("pnpm-workspace.yaml"); ok {
		return []Signal{{
			Name: "pnpm-workspace", Confidence: ConfFileConvention,
			Evidence: []string{p}, Tags: []string{"monorepo"},
		}}
	}
	return nil
}

func detectTurborepo(s *Snapshot) []Signal {
	if p, ok := s.Has("turbo.json"); ok {
		return []Signal{{
			Name: "turborepo", Confidence: ConfFileConvention,
			Evidence: []string{p}, Tags: []string{"monorepo"},
		}}
	}
	return nil
}

func detectNx(s *Snapshot) []Signal {
	if p, ok := s.Has("nx.json"); ok {
		return []Signal{{
			Name: "nx", Confidence: ConfFileConvention,
			Evidence: []string{p}, Tags: []string{"monorepo"},
		}}
	}
	return nil
}

func detectLerna(s *Snapshot) []Signal {
	if p, ok := s.Has("lerna.json"); ok {
		return []Signal{{
			Name: "lerna", Confidence: ConfFileConvention,
			Evidence: []string{p}, Tags: []string{"monorepo"},
		}}
	}
	return nil
}

func detectGoWorkspace(s *Snapshot) []Signal {
	if p, ok := s.Has("go.work"); ok {
		return []Signal{{
			Name: "go-workspace", Confidence: ConfManifest,
			Evidence: []string{p}, Tags: []string{"monorepo"},
		}}
	}
	return nil
}
