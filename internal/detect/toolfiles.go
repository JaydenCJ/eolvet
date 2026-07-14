// Version-pin file detectors: .python-version/.nvmrc-style single-value
// files, asdf/mise .tool-versions, and Heroku-style runtime.txt.
package detect

import (
	"path"
	"strings"

	"github.com/JaydenCJ/eolvet/internal/vers"
)

// toolProducts maps asdf/mise plugin names (and runtime.txt prefixes)
// to snapshot product keys. Tools eolvet has no lifecycle data for are
// skipped silently — reporting them would only be noise.
var toolProducts = map[string]string{
	"python":   "python",
	"node":     "node",
	"nodejs":   "node",
	"golang":   "go",
	"go":       "go",
	"ruby":     "ruby",
	"java":     "java",
	"php":      "php",
	"postgres": "postgres",
	"mysql":    "mysql",
	"mariadb":  "mariadb",
	"mongodb":  "mongodb",
	"redis":    "redis",
	"nginx":    "nginx",
	"haproxy":  "haproxy",
	"dotnet":   "dotnet",
}

// versionFile reads a single-value pin file such as .python-version or
// .nvmrc: the first non-empty, non-comment line is the version. Aliases
// that name no concrete release ("lts/hydrogen", "node", "system") and
// alternative interpreters ("pypy3.9-7.3.9") are skipped — they do not
// pin a release of the product this file is named after.
func (e *Engine) versionFile(name string, content []byte) []Decl {
	product := versionFiles[path.Base(name)]
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		version := vers.Normalize(trimmed)
		if len(vers.Components(version)) == 0 {
			return nil
		}
		return []Decl{{
			Product: product,
			Version: version,
			Raw:     trimmed,
			Line:    i + 1,
			Source:  "version-file",
		}}
	}
	return nil
}

// toolVersions reads asdf/mise .tool-versions files: one "tool version
// [fallback…]" per line; only the first (winning) version counts. Java
// pins carry a distribution prefix ("temurin-21.0.2+13.0.LTS") that is
// stripped down to the version.
func (e *Engine) toolVersions(content []byte) []Decl {
	var decls []Decl
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		product, ok := toolProducts[strings.ToLower(fields[0])]
		if !ok {
			continue
		}
		version := vers.Normalize(fields[1])
		if len(vers.Components(version)) == 0 {
			// "temurin-21.0.2" / "ref:master" — keep only pins whose tail
			// is a concrete version.
			if _, tail, found := strings.Cut(version, "-"); found && len(vers.Components(tail)) > 0 {
				version = tail
			} else {
				continue
			}
		}
		decls = append(decls, Decl{
			Product: product,
			Version: version,
			Raw:     trimmed,
			Line:    i + 1,
			Source:  "tool-versions",
		})
	}
	return decls
}

// runtimeTxt reads Heroku-style runtime.txt files ("python-3.8.10").
func (e *Engine) runtimeTxt(content []byte) []Decl {
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, version, ok := strings.Cut(trimmed, "-")
		if !ok {
			return nil
		}
		product, known := toolProducts[strings.ToLower(name)]
		if !known || len(vers.Components(version)) == 0 {
			return nil
		}
		return []Decl{{
			Product: product,
			Version: vers.Normalize(version),
			Raw:     trimmed,
			Line:    i + 1,
			Source:  "runtime-txt",
		}}
	}
	return nil
}
