// Minimal glob matcher with ** support for --exclude patterns.
package scan

import (
	"path"
	"strings"
)

// MatchGlob reports whether a slash-separated relative path matches a
// glob pattern. Within a segment, *, ?, and [class] behave as in
// path.Match; a bare ** segment spans any number of path segments, so
// "vendor/**" matches everything under vendor and "**/Dockerfile"
// matches a Dockerfile at any depth (including the root).
func MatchGlob(pattern, rel string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

// ValidGlob reports whether a pattern is syntactically usable, so the
// CLI can reject typos ("[oops") at startup instead of silently never
// matching during the walk.
func ValidGlob(pattern string) bool {
	for _, seg := range strings.Split(pattern, "/") {
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, "probe"); err != nil {
			return false
		}
	}
	return true
}

func matchSegments(pat, parts []string) bool {
	if len(pat) == 0 {
		return len(parts) == 0
	}
	if pat[0] == "**" {
		// ** may swallow zero or more leading path segments.
		for skip := 0; skip <= len(parts); skip++ {
			if matchSegments(pat[1:], parts[skip:]) {
				return true
			}
		}
		return false
	}
	if len(parts) == 0 {
		return false
	}
	ok, err := path.Match(pat[0], parts[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pat[1:], parts[1:])
}
