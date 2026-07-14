// Package detect turns files found in a repository into version
// declarations: "this file says this product runs at this version".
//
// Detectors are pure functions over file content — they never touch the
// filesystem or the network — and they only report what a file actually
// declares. Deciding whether a declaration is end-of-life is the report
// package's job; finding files is the scan package's job.
package detect

import (
	"path"
	"strings"

	"github.com/JaydenCJ/eolvet/internal/eoldata"
)

// Decl is one version declaration extracted from a file.
type Decl struct {
	Product string // snapshot product key, e.g. "python"
	Version string // effective concrete version or codename; "" when unresolved
	Raw     string // the declaration as written, e.g. "python:3.8-slim-bullseye"
	File    string // repo-relative path (filled in by the scanner)
	Line    int    // 1-based line of the declaration
	Source  string // detector id: "dockerfile", "compose", "tool-versions", …
	Note    string // human context: "floor of constraint >=3.9", "unpinned tag", …
}

// Engine holds the snapshot the detectors consult for codename lookups.
// Detectors never judge EOL status; the snapshot is only needed to
// recognize distro codenames inside image tags.
type Engine struct {
	snap *eoldata.Snapshot
}

// New builds a detection engine over the given snapshot.
func New(snap *eoldata.Snapshot) *Engine {
	return &Engine{snap: snap}
}

// Matches reports whether a file basename is one the engine knows how to
// read. The scanner uses it to avoid opening irrelevant files.
func (e *Engine) Matches(name string) bool {
	return kindOf(name) != kindNone
}

// DetectFile extracts every declaration from one file's content. The
// returned Decls carry Line/Source/Raw; the caller fills in File.
func (e *Engine) DetectFile(name string, content []byte) []Decl {
	switch kindOf(name) {
	case kindDockerfile:
		return e.dockerfile(content)
	case kindCompose:
		return e.compose(content)
	case kindVersionFile:
		return e.versionFile(name, content)
	case kindToolVersions:
		return e.toolVersions(content)
	case kindRuntimeTxt:
		return e.runtimeTxt(content)
	case kindGoMod:
		return e.goMod(content)
	case kindPackageJSON:
		return e.packageJSON(content)
	case kindPyprojectTOML:
		return e.pyprojectTOML(content)
	case kindGemfile:
		return e.gemfile(content)
	case kindComposerJSON:
		return e.composerJSON(content)
	}
	return nil
}

type fileKind int

const (
	kindNone fileKind = iota
	kindDockerfile
	kindCompose
	kindVersionFile
	kindToolVersions
	kindRuntimeTxt
	kindGoMod
	kindPackageJSON
	kindPyprojectTOML
	kindGemfile
	kindComposerJSON
)

// versionFiles maps single-value version pin files to their product.
var versionFiles = map[string]string{
	".python-version": "python",
	".nvmrc":          "node",
	".node-version":   "node",
	".ruby-version":   "ruby",
	".go-version":     "go",
	".java-version":   "java",
}

func kindOf(name string) fileKind {
	base := path.Base(name)
	lower := strings.ToLower(base)
	switch {
	case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."),
		strings.HasSuffix(lower, ".dockerfile"):
		return kindDockerfile
	case lower == "docker-compose.yml" || lower == "docker-compose.yaml",
		lower == "compose.yml" || lower == "compose.yaml":
		return kindCompose
	case base == ".tool-versions":
		return kindToolVersions
	case lower == "runtime.txt":
		return kindRuntimeTxt
	case base == "go.mod":
		return kindGoMod
	case base == "package.json":
		return kindPackageJSON
	case base == "pyproject.toml":
		return kindPyprojectTOML
	case base == "Gemfile":
		return kindGemfile
	case base == "composer.json":
		return kindComposerJSON
	}
	if _, ok := versionFiles[base]; ok {
		return kindVersionFile
	}
	return kindNone
}

// isCodename adapts the snapshot's per-product codename tables for the
// tag decomposition helpers.
func (e *Engine) isCodename(product, token string) bool {
	p, ok := e.snap.Product(product)
	return ok && p.IsCodename(token)
}

// resolveImage turns a parsed image reference into declarations: one for
// the image's own product and one per base-OS hint in the tag suffix
// ("python:3.8-slim-bullseye" yields python 3.8 and debian bullseye).
func (e *Engine) resolveImage(ref ImageRef, raw string, line int, source string) []Decl {
	product, ok := ref.Product()
	if !ok {
		return nil // image we cannot map — nothing honest to report
	}
	var decls []Decl
	main := Decl{Product: product, Raw: raw, Line: line, Source: source}
	switch {
	case ref.Tag == "" && ref.Digest != "":
		main.Note = "pinned by digest only; release version not stated"
	case ref.Tag == "" || ref.Tag == "latest" || ref.Tag == "stable":
		main.Note = "unpinned tag; resolves to a different release over time"
	case strings.Contains(ref.Tag, "$"):
		main.Note = "tag uses an unresolved variable"
	default:
		if v := tagVersion(ref.Tag); v != "" {
			main.Version = v
		} else if first, _, _ := strings.Cut(ref.Tag, "-"); e.isCodename(product, first) {
			// Distro images tagged by codename: debian:bullseye-slim.
			main.Version = first
			main.Note = "codename tag"
		} else {
			main.Note = "tag " + ref.Tag + " does not state a release version"
		}
	}
	decls = append(decls, main)
	if ref.Tag != "" {
		for _, hint := range tagBaseHints(ref.Tag, e.isCodename) {
			if hint.product == product {
				continue // debian:bullseye-slim must not double-report
			}
			decls = append(decls, Decl{
				Product: hint.product,
				Version: hint.version,
				Raw:     raw,
				Line:    line,
				Source:  source,
				Note:    "base OS of image tag",
			})
		}
	}
	return decls
}
