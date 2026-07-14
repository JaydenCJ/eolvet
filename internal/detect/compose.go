// Compose-file detector: image: lines in docker-compose.yml and friends.
package detect

import (
	"strings"
)

// compose extracts declarations from `image:` keys. Compose files are
// YAML, but the one key this audit needs is a scalar on its own line in
// every real-world file, so a line scan keeps eolvet dependency-free
// while handling quotes, comments, and unresolved ${VARS} correctly.
func (e *Engine) compose(content []byte) []Decl {
	var decls []Decl
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		val, ok := strings.CutPrefix(trimmed, "image:")
		if !ok {
			continue
		}
		// Strip a trailing comment, then surrounding quotes.
		if j := strings.Index(val, " #"); j >= 0 {
			val = val[:j]
		}
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if val == "" {
			continue
		}
		raw := val
		if strings.Contains(val, "$") {
			// ${REGISTRY:-docker.io}/python:${TAG:-3.9} — compose default
			// syntax mirrors Dockerfile ARG syntax closely enough to reuse
			// the same expander with no declared variables.
			val = expandArgs(val, nil)
		}
		decls = append(decls, e.resolveImage(ParseImageRef(val), raw, i+1, "compose")...)
	}
	return decls
}
