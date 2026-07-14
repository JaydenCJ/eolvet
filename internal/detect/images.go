// Container image reference parsing and the image → product mapping.
package detect

import (
	"strings"
)

// ImageRef is a container image reference split into its parts. Repo is
// normalized: registry host and the implicit "library/" namespace are
// stripped, so "docker.io/library/python", "public.ecr.aws/docker/library/python"
// and "python" all normalize to "python".
type ImageRef struct {
	Repo   string // normalized repository path
	Tag    string // tag without the leading ":", "" when absent
	Digest string // digest without the leading "@", "" when absent
}

// imageProducts maps normalized repository paths to snapshot product
// keys. Multi-segment keys are checked before the last-segment fallback,
// so "dotnet/sdk" wins over a hypothetical bare "sdk".
var imageProducts = map[string]string{
	"python":          "python",
	"node":            "node",
	"golang":          "go",
	"ruby":            "ruby",
	"php":             "php",
	"openjdk":         "java",
	"eclipse-temurin": "java",
	"amazoncorretto":  "java",
	"postgres":        "postgres",
	"mysql":           "mysql",
	"mariadb":         "mariadb",
	"mongo":           "mongodb",
	"redis":           "redis",
	"nginx":           "nginx",
	"haproxy":         "haproxy",
	"ubuntu":          "ubuntu",
	"debian":          "debian",
	"alpine":          "alpine",
	"centos":          "centos",
	"rockylinux":      "rockylinux",
	"almalinux":       "almalinux",
	"amazonlinux":     "amazonlinux",
	"dotnet/sdk":      "dotnet",
	"dotnet/aspnet":   "dotnet",
	"dotnet/runtime":  "dotnet",
}

// ParseImageRef splits a raw image reference into repo, tag, and digest.
func ParseImageRef(raw string) ImageRef {
	ref := ImageRef{Repo: strings.TrimSpace(raw)}
	if i := strings.Index(ref.Repo, "@"); i >= 0 {
		ref.Digest = ref.Repo[i+1:]
		ref.Repo = ref.Repo[:i]
	}
	// The tag is the part after the last ":" — but only when that ":" is
	// not the port of a registry host ("registry.example.test:5000/app").
	if i := strings.LastIndex(ref.Repo, ":"); i >= 0 && !strings.Contains(ref.Repo[i:], "/") {
		ref.Tag = ref.Repo[i+1:]
		ref.Repo = ref.Repo[:i]
	}
	ref.Repo = normalizeRepo(ref.Repo)
	return ref
}

// Product maps the reference onto a snapshot product key.
func (r ImageRef) Product() (string, bool) {
	if p, ok := imageProducts[r.Repo]; ok {
		return p, true
	}
	// "dotnet/sdk"-style keys already matched above; fall back to the
	// last path segment so "myregistry-mirror/python" still resolves.
	if i := strings.LastIndex(r.Repo, "/"); i >= 0 {
		if p, ok := imageProducts[r.Repo[i+1:]]; ok {
			return p, true
		}
	}
	return "", false
}

// normalizeRepo drops the registry host (any first segment containing a
// dot, a port colon, or "localhost") and the implicit "library/" (or
// mirror "docker/library/") namespace.
func normalizeRepo(repo string) string {
	parts := strings.Split(repo, "/")
	if len(parts) > 1 {
		first := parts[0]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			parts = parts[1:]
		}
	}
	for len(parts) > 1 && (parts[0] == "library" || parts[0] == "docker") {
		parts = parts[1:]
	}
	return strings.Join(parts, "/")
}

// tagVersion extracts the leading version token from an image tag:
// "3.8-slim-bullseye" -> "3.8", "18-alpine3.17" -> "18",
// "bullseye-20230522" -> "" (codenames are not versions),
// "latest" -> "". Distro images may use a codename as the whole
// version ("debian:bullseye"), which resolveTag handles separately.
func tagVersion(tag string) string {
	tok, _, _ := strings.Cut(tag, "-")
	if tok == "" {
		return ""
	}
	if c := tok[0]; c < '0' || c > '9' {
		return ""
	}
	return tok
}

// baseHint describes a base-OS component embedded in another image's
// tag suffix, e.g. the "bullseye" in "python:3.8-slim-bullseye" or the
// "alpine3.17" in "golang:1.20-alpine3.17".
type baseHint struct {
	product string
	version string // numeric version or codename
}

// tagBaseHints scans the "-"-separated suffix tokens of a tag for base
// OS hints. isCodename answers whether a token is a known distro
// codename for the given product.
func tagBaseHints(tag string, isCodename func(product, token string) bool) []baseHint {
	tokens := strings.Split(tag, "-")
	if len(tokens) < 2 {
		return nil
	}
	var hints []baseHint
	for _, tok := range tokens[1:] {
		switch {
		case tok == "":
		case strings.HasPrefix(tok, "alpine") && len(tok) > len("alpine"):
			// "alpine3.17" pins the Alpine release; bare "-alpine"
			// floats with the image and is deliberately not reported.
			hints = append(hints, baseHint{product: "alpine", version: tok[len("alpine"):]})
		case isCodename("debian", tok):
			hints = append(hints, baseHint{product: "debian", version: tok})
		case isCodename("ubuntu", tok):
			hints = append(hints, baseHint{product: "ubuntu", version: tok})
		}
	}
	return hints
}
