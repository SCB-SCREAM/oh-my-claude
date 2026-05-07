package detect

import (
	"sort"
	"strings"
	"sync"
)

// CatalogEntry describes one detector for documentation purposes — it is
// what `omc stacks` lists. Triggers should be human-readable hints
// ("package.json", "next.config.{js,ts}", "any *.tf file"), not
// regular expressions.
type CatalogEntry struct {
	ID       string   // matches Detector.ID(): "next.js", "github-actions"
	Tags     []string // same vocabulary as Signal.Tags
	Triggers []string // human-readable evidence patterns
	MinConf  float64  // weakest confidence the detector can emit
	MaxConf  float64  // strongest confidence with all evidence stacked
	Summary  string   // one-line description
}

var (
	catalogMu      sync.Mutex
	catalogEntries []CatalogEntry
)

// catalog registers a detector's documentation entry. Called from the
// same init() as register() so the two stay in lockstep.
func catalog(e CatalogEntry) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	for _, existing := range catalogEntries {
		if existing.ID == e.ID {
			return
		}
	}
	catalogEntries = append(catalogEntries, e)
}

// Catalog returns a stable, sorted snapshot of every catalog entry. Sorted
// by primary tag, then ID, so `omc stacks` output is deterministic.
func Catalog() []CatalogEntry {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	out := make([]CatalogEntry, len(catalogEntries))
	copy(out, catalogEntries)
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := primaryTag(out[i]), primaryTag(out[j])
		if ti != tj {
			return tagOrder(ti) < tagOrder(tj)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// CatalogGroup is one tag-keyed bucket of catalog entries returned by
// [GroupedCatalog]. Tags are pre-sorted by display priority so callers
// can render section headers without re-sorting.
type CatalogGroup struct {
	Tag     string
	Entries []CatalogEntry
}

// GroupedCatalog returns catalog entries grouped by primary tag, in the
// order: language, package-manager, framework, orm, db, infra, ci,
// monorepo. Entries within a group are alphabetical by ID.
func GroupedCatalog() []CatalogGroup {
	all := Catalog()
	byTag := map[string][]CatalogEntry{}
	for _, e := range all {
		t := primaryTag(e)
		byTag[t] = append(byTag[t], e)
	}
	var tags []string
	for t := range byTag {
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tagOrder(tags[i]) < tagOrder(tags[j])
	})
	out := make([]CatalogGroup, 0, len(tags))
	for _, t := range tags {
		out = append(out, CatalogGroup{Tag: t, Entries: byTag[t]})
	}
	return out
}

func primaryTag(e CatalogEntry) string {
	if len(e.Tags) == 0 {
		return "other"
	}
	return e.Tags[0]
}

func tagOrder(tag string) int {
	switch strings.ToLower(tag) {
	case "language":
		return 0
	case "package-manager":
		return 1
	case "framework":
		return 2
	case "orm":
		return 3
	case "db":
		return 4
	case "infra":
		return 5
	case "ci":
		return 6
	case "monorepo":
		return 7
	default:
		return 100
	}
}
