package detect

import "strings"

func init() {
	register(detectorFunc{id: "docker", fn: detectDocker})
	catalog(CatalogEntry{
		ID: "docker", Tags: []string{"infra"},
		Triggers: []string{"Dockerfile", "Dockerfile.*", "docker-compose*.{yml,yaml}", ".dockerignore"},
		MinConf:  ConfFileConvention, MaxConf: ConfManifest,
		Summary: "Container build / Compose stack present",
	})

	register(detectorFunc{id: "kubernetes", fn: detectKubernetes})
	catalog(CatalogEntry{
		ID: "kubernetes", Tags: []string{"infra"},
		Triggers: []string{"kustomization.yaml", "k8s/ directory", "Helm Chart.yaml"},
		MinConf:  ConfFileConvention, MaxConf: ConfFileConvention,
		Summary: "Kubernetes manifests / Kustomize / Helm charts",
	})

	register(detectorFunc{id: "terraform", fn: detectTerraform})
	catalog(CatalogEntry{
		ID: "terraform", Tags: []string{"infra"},
		Triggers: []string{"*.tf", "*.tf.json", ".terraform.lock.hcl"},
		MinConf:  ConfFileConvention, MaxConf: ConfLockfile,
		Summary: "Terraform infrastructure-as-code",
	})
}

func detectDocker(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0
	if p, ok := s.Has("Dockerfile"); ok {
		ev = append(ev, p)
		conf = max(conf, ConfManifest)
	}
	for _, f := range s.Files {
		base := pathBase(f)
		switch {
		case strings.HasPrefix(base, "Dockerfile.") && base != "Dockerfile":
			ev = append(ev, f)
			conf = max(conf, ConfManifest)
		case strings.HasPrefix(base, "docker-compose") &&
			(strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")):
			ev = append(ev, f)
			conf = max(conf, ConfManifest)
		}
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "docker", Confidence: conf,
		Evidence: ev, Tags: []string{"infra"},
	}}
}

func detectKubernetes(s *Snapshot) []Signal {
	var ev []string
	for _, name := range []string{"kustomization.yaml", "kustomization.yml"} {
		ev = append(ev, s.ByName[name]...)
	}
	ev = append(ev, s.ByName["Chart.yaml"]...)
	// Loose heuristic: a top-level k8s/ directory with .yaml files.
	for _, f := range s.Files {
		if strings.HasPrefix(f, "k8s/") && (strings.HasSuffix(f, ".yaml") || strings.HasSuffix(f, ".yml")) {
			ev = append(ev, f)
			break
		}
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "kubernetes", Confidence: ConfFileConvention,
		Evidence: ev, Tags: []string{"infra"},
	}}
}

func detectTerraform(s *Snapshot) []Signal {
	var ev []string
	conf := 0.0
	for _, f := range s.ByExt[".tf"] {
		ev = append(ev, f)
		conf = max(conf, ConfFileConvention)
	}
	if p, ok := s.Has(".terraform.lock.hcl"); ok {
		ev = append(ev, p)
		conf = ConfLockfile
	}
	if len(ev) == 0 {
		return nil
	}
	return []Signal{{
		Name: "terraform", Confidence: conf,
		Evidence: ev, Tags: []string{"infra"},
	}}
}
