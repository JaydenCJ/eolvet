// Language-manifest detectors: go.mod, package.json engines,
// pyproject.toml, Gemfile, and composer.json.
package detect

import (
	"encoding/json"
	"strings"

	"github.com/JaydenCJ/eolvet/internal/vers"
)

// goMod reads the `go` directive — and prefers the `toolchain`
// directive when present, because that is the compiler that actually
// builds the module and therefore the release whose support window
// matters.
func (e *Engine) goMod(content []byte) []Decl {
	var goDecl, toolDecl *Decl
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if fields := strings.Fields(trimmed); len(fields) == 2 {
			switch fields[0] {
			case "go":
				if goDecl == nil && len(vers.Components(fields[1])) > 0 {
					goDecl = &Decl{Product: "go", Version: fields[1], Raw: trimmed, Line: i + 1, Source: "go-mod", Note: "go directive"}
				}
			case "toolchain":
				v := strings.TrimPrefix(fields[1], "go")
				if toolDecl == nil && len(vers.Components(v)) > 0 {
					toolDecl = &Decl{Product: "go", Version: v, Raw: trimmed, Line: i + 1, Source: "go-mod", Note: "toolchain directive"}
				}
			}
		}
	}
	if toolDecl != nil {
		return []Decl{*toolDecl}
	}
	if goDecl != nil {
		return []Decl{*goDecl}
	}
	return nil
}

// packageJSON reads engines.node. The declared value is almost always a
// range; the floor is what bounds the repo's exposure.
func (e *Engine) packageJSON(content []byte) []Decl {
	var doc struct {
		Engines map[string]string `json:"engines"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil // not this detector's job to lint broken JSON
	}
	constraint, ok := doc.Engines["node"]
	if !ok {
		return nil
	}
	return constraintDecl("node", constraint, lineOfAfter(content, `"engines"`, `"node"`), "package-json")
}

// pyprojectTOML reads requires-python from [project] and the python
// constraint from [tool.poetry.dependencies], covering both PEP 621 and
// Poetry projects with a section-aware line scan.
func (e *Engine) pyprojectTOML(content []byte) []Decl {
	section := ""
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.Trim(trimmed, "[]")
			continue
		}
		key, val, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		switch {
		case section == "project" && key == "requires-python":
			return constraintDecl("python", val, i+1, "pyproject")
		case section == "tool.poetry.dependencies" && key == "python":
			return constraintDecl("python", val, i+1, "pyproject")
		}
	}
	return nil
}

// gemfile reads the `ruby "3.1.2"` pin. Indirections such as
// `ruby file: ".ruby-version"` are skipped — the version-file detector
// already covers the referenced file.
func (e *Engine) gemfile(content []byte) []Decl {
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		rest, ok := strings.CutPrefix(trimmed, "ruby ")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, `"`) && !strings.HasPrefix(rest, `'`) {
			return nil // `ruby file:` / `ruby Something` — no literal pin
		}
		quote := rest[0]
		end := strings.IndexByte(rest[1:], quote)
		if end < 0 {
			return nil
		}
		return constraintDecl("ruby", rest[1:1+end], i+1, "gemfile")
	}
	return nil
}

// composerJSON reads the php constraint from require.
func (e *Engine) composerJSON(content []byte) []Decl {
	var doc struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil
	}
	constraint, ok := doc.Require["php"]
	if !ok {
		return nil
	}
	return constraintDecl("php", constraint, lineOfAfter(content, `"require"`, `"php"`), "composer-json")
}

// constraintDecl builds a declaration from a constraint expression,
// recording the floor as the effective version. An unbounded constraint
// ("*") yields an unresolved declaration rather than a guess.
func constraintDecl(product, constraint string, line int, source string) []Decl {
	d := Decl{Product: product, Raw: constraint, Line: line, Source: source}
	if floor, ok := vers.Floor(constraint); ok {
		d.Version = floor
		if floor != vers.Normalize(constraint) {
			d.Note = "floor of constraint " + constraint
		}
	} else {
		d.Note = "constraint " + constraint + " has no lower bound"
	}
	return []Decl{d}
}

// lineOfAfter finds the 1-based line of the first `key` occurring at or
// after `anchor`, giving JSON-derived findings a usable location without
// a position-tracking parser (a "node" dependency elsewhere in the file
// must not steal the engines block's line number).
func lineOfAfter(content []byte, anchor, key string) int {
	s := string(content)
	base := strings.Index(s, anchor)
	if base < 0 {
		base = 0
	}
	idx := strings.Index(s[base:], key)
	if idx < 0 {
		return 1
	}
	return 1 + strings.Count(s[:base+idx], "\n")
}
