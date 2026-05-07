package detect

import (
	"bufio"
	"io/fs"
	"path"
	"strings"
)

// gitignore is a tiny, root-only .gitignore matcher. We deliberately do not
// support nested .gitignore files, .git/info/exclude, or the user's global
// gitignore — that complexity is not worth it before v1.0, and the
// hardcoded [skipDirs] catches the bulk of "ignore me" intent.
type gitignore struct {
	rules []gitignoreRule
}

type gitignoreRule struct {
	pattern string // normalized: no leading "/", trailing "/" stripped (recorded in dirOnly)
	negate  bool   // line started with "!"
	dirOnly bool   // pattern ended in "/"
	rooted  bool   // pattern contained an unescaped "/" anywhere except trailing
	hasGlob bool   // pattern contains '*', '?', '['
}

func loadRootGitignore(fsys fs.FS) *gitignore {
	g := &gitignore{}
	f, err := fsys.Open(".gitignore")
	if err != nil {
		return g
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := parseGitignoreLine(line)
		if r.pattern == "" {
			continue
		}
		g.rules = append(g.rules, r)
	}
	return g
}

func parseGitignoreLine(line string) gitignoreRule {
	r := gitignoreRule{}
	if strings.HasPrefix(line, "!") {
		r.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		r.rooted = true
		line = strings.TrimPrefix(line, "/")
	} else if strings.Contains(line, "/") {
		r.rooted = true
	}
	r.pattern = line
	r.hasGlob = strings.ContainsAny(line, "*?[")
	return r
}

// match reports whether p (repo-relative, slash-separated) is ignored. If
// isDir is false, dir-only rules are skipped.
func (g *gitignore) match(p string, isDir bool) bool {
	if g == nil || len(g.rules) == 0 {
		return false
	}
	matched := false
	for _, r := range g.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.matches(p) {
			matched = !r.negate
		}
	}
	return matched
}

func (r gitignoreRule) matches(p string) bool {
	if r.rooted {
		// Anchored to the repo root: the pattern must match the path or
		// a leading prefix that is itself a complete path component.
		if r.hasGlob {
			ok, _ := path.Match(r.pattern, p)
			if ok {
				return true
			}
			// For directory-only rules we also accept any nested path.
			if r.dirOnly {
				if dir, ok := matchPrefix(r.pattern, p); ok && dir {
					return true
				}
			}
			return false
		}
		if p == r.pattern || strings.HasPrefix(p, r.pattern+"/") {
			return true
		}
		return false
	}

	// Unrooted: match against any path component suffix.
	rest := p
	for rest != "" {
		if r.hasGlob {
			if ok, _ := path.Match(r.pattern, rest); ok {
				return true
			}
		} else if rest == r.pattern || strings.HasPrefix(rest, r.pattern+"/") {
			return true
		}
		slash := strings.IndexByte(rest, '/')
		if slash < 0 {
			return false
		}
		rest = rest[slash+1:]
	}
	return false
}

// matchPrefix reports whether pattern matches a leading directory of p.
// Used for dir-only rules so that gitignoring "build/" also drops
// "build/output/x.so".
func matchPrefix(pattern, p string) (bool, bool) {
	for {
		if ok, _ := path.Match(pattern, p); ok {
			return true, true
		}
		slash := strings.LastIndexByte(p, '/')
		if slash < 0 {
			return false, false
		}
		p = p[:slash]
	}
}
