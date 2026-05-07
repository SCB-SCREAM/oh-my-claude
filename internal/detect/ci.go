package detect

func init() {
	register(detectorFunc{id: "github-actions", fn: detectGitHubActions})
	catalog(CatalogEntry{
		ID: "github-actions", Tags: []string{"ci"},
		Triggers: []string{".github/workflows/*.{yml,yaml}"},
		MinConf:  ConfFileConvention, MaxConf: ConfFileConvention,
		Summary: "GitHub Actions workflows",
	})

	register(detectorFunc{id: "gitlab-ci", fn: detectGitLabCI})
	catalog(CatalogEntry{
		ID: "gitlab-ci", Tags: []string{"ci"},
		Triggers: []string{".gitlab-ci.yml"},
		MinConf:  ConfFileConvention, MaxConf: ConfFileConvention,
		Summary: "GitLab CI pipeline config",
	})

	register(detectorFunc{id: "circleci", fn: detectCircleCI})
	catalog(CatalogEntry{
		ID: "circleci", Tags: []string{"ci"},
		Triggers: []string{".circleci/config.yml"},
		MinConf:  ConfFileConvention, MaxConf: ConfFileConvention,
		Summary: "CircleCI pipeline config",
	})
}

func detectGitHubActions(s *Snapshot) []Signal {
	ev := s.Glob(".github/workflows/*.yml")
	ev = append(ev, s.Glob(".github/workflows/*.yaml")...)
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "github-actions", Confidence: ConfFileConvention,
		Evidence: ev, Tags: []string{"ci"},
	}}
}

func detectGitLabCI(s *Snapshot) []Signal {
	if p, ok := s.Has(".gitlab-ci.yml"); ok {
		return []Signal{{
			Name: "gitlab-ci", Confidence: ConfFileConvention,
			Evidence: []string{p}, Tags: []string{"ci"},
		}}
	}
	return nil
}

func detectCircleCI(s *Snapshot) []Signal {
	if s.HasAt(".circleci/config.yml") {
		return []Signal{{
			Name: "circleci", Confidence: ConfFileConvention,
			Evidence: []string{".circleci/config.yml"}, Tags: []string{"ci"},
		}}
	}
	if s.HasAt(".circleci/config.yaml") {
		return []Signal{{
			Name: "circleci", Confidence: ConfFileConvention,
			Evidence: []string{".circleci/config.yaml"}, Tags: []string{"ci"},
		}}
	}
	return nil
}
