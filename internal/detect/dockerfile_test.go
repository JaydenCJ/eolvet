// Tests for the Dockerfile detector: FROM parsing, ARG substitution,
// multi-stage handling, and tag decomposition into base-OS findings.
package detect

import (
	"testing"

	"github.com/JaydenCJ/eolvet/internal/eoldata"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	snap, err := eoldata.Load()
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	return New(snap)
}

func detectDockerfile(t *testing.T, content string) []Decl {
	t.Helper()
	return testEngine(t).DetectFile("Dockerfile", []byte(content))
}

func one(t *testing.T, decls []Decl) Decl {
	t.Helper()
	if len(decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d: %+v", len(decls), decls)
	}
	return decls[0]
}

func TestDockerfileSimpleFrom(t *testing.T) {
	d := one(t, detectDockerfile(t, "FROM python:3.8\n"))
	if d.Product != "python" || d.Version != "3.8" || d.Line != 1 || d.Source != "dockerfile" {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestDockerfileTagDecomposition(t *testing.T) {
	// python:3.8-slim-bullseye is *two* exposures: an EOL Python and an
	// aging Debian base. Both must be reported.
	decls := detectDockerfile(t, "FROM python:3.8-slim-bullseye\n")
	if len(decls) != 2 {
		t.Fatalf("expected 2 declarations, got %+v", decls)
	}
	if decls[0].Product != "python" || decls[0].Version != "3.8" {
		t.Fatalf("main decl: %+v", decls[0])
	}
	if decls[1].Product != "debian" || decls[1].Version != "bullseye" {
		t.Fatalf("base decl: %+v", decls[1])
	}
	// "alpine3.17" carries a pinned Alpine release…
	decls = detectDockerfile(t, "FROM golang:1.20-alpine3.17\n")
	if len(decls) != 2 || decls[0].Version != "1.20" ||
		decls[1].Product != "alpine" || decls[1].Version != "3.17" {
		t.Fatalf("alpine suffix: %+v", decls)
	}
	// …while bare "-alpine" floats with the image — reporting a version
	// we do not know would be a fabricated finding.
	d := one(t, detectDockerfile(t, "FROM node:18-alpine\n"))
	if d.Product != "node" || d.Version != "18" {
		t.Fatalf("bare -alpine: %+v", d)
	}
}

func TestDockerfileArgSubstitution(t *testing.T) {
	// ARG default in ${VAR} form.
	d := one(t, detectDockerfile(t, "ARG PY=3.9\nFROM python:${PY}-slim\n"))
	if d.Version != "3.9" || d.Line != 2 {
		t.Fatalf("ARG default not substituted: %+v", d)
	}
	if d.Raw != "python:${PY}-slim" {
		t.Fatalf("Raw must show the file as written, got %q", d.Raw)
	}
	// ${VAR:-default} fallback syntax.
	d = one(t, detectDockerfile(t, "FROM python:${PY:-3.10}\n"))
	if d.Version != "3.10" {
		t.Fatalf("${VAR:-default} not applied: %+v", d)
	}
	// "ARG PY" without a default has no offline-knowable value; the
	// finding must say so instead of guessing.
	d = one(t, detectDockerfile(t, "ARG PY\nFROM python:$PY\n"))
	if d.Version != "" || d.Note == "" {
		t.Fatalf("unresolved variable should yield an explained unknown: %+v", d)
	}
}

func TestDockerfileMultiStageAndFlags(t *testing.T) {
	// References to earlier stages and FROM scratch are not images.
	content := "FROM golang:1.22 AS build\nFROM build\nFROM scratch\n"
	d := one(t, detectDockerfile(t, content))
	if d.Product != "go" || d.Version != "1.22" {
		t.Fatalf("unexpected decl: %+v", d)
	}
	// --platform (and future flags) sit between FROM and the image.
	d = one(t, detectDockerfile(t, "FROM --platform=$BUILDPLATFORM node:20\n"))
	if d.Product != "node" || d.Version != "20" {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestDockerfileLineContinuationsAndComments(t *testing.T) {
	content := "# base image\nFROM \\\n  ubuntu:22.04\nRUN echo hi\n"
	d := one(t, detectDockerfile(t, content))
	if d.Product != "ubuntu" || d.Version != "22.04" {
		t.Fatalf("unexpected decl: %+v", d)
	}
	if d.Line != 2 {
		t.Fatalf("continued FROM should anchor at its first line, got %d", d.Line)
	}
}

func TestDockerfileUnpinnedTagsAreExplainedUnknowns(t *testing.T) {
	d := one(t, detectDockerfile(t, "FROM redis:latest\n"))
	if d.Product != "redis" || d.Version != "" || d.Note == "" {
		t.Fatalf("latest tag should be an explained unknown: %+v", d)
	}
	d = one(t, detectDockerfile(t, "FROM python@sha256:0000000000000000000000000000000000000000000000000000000000000000\n"))
	if d.Product != "python" || d.Version != "" || d.Note == "" {
		t.Fatalf("digest pin should be an explained unknown: %+v", d)
	}
}

func TestDockerfileDistroCodenameTag(t *testing.T) {
	d := one(t, detectDockerfile(t, "FROM debian:bullseye-slim\n"))
	if d.Product != "debian" || d.Version != "bullseye" {
		t.Fatalf("codename tag should resolve on the main product: %+v", d)
	}
}

func TestDockerfileUnknownImageSkipped(t *testing.T) {
	if decls := detectDockerfile(t, "FROM examplecorp/internal-runner:4.2\n"); len(decls) != 0 {
		t.Fatalf("unmappable image should yield nothing, got %+v", decls)
	}
}

func TestDockerfileMultipleStagesAllReported(t *testing.T) {
	content := "FROM node:16 AS assets\nFROM golang:1.21 AS build\nFROM alpine:3.18\n"
	decls := detectDockerfile(t, content)
	if len(decls) != 3 {
		t.Fatalf("expected 3 declarations, got %+v", decls)
	}
	for i, want := range []string{"node", "go", "alpine"} {
		if decls[i].Product != want || decls[i].Line != i+1 {
			t.Fatalf("decl %d = %+v, want product %s at line %d", i, decls[i], want, i+1)
		}
	}
}

func TestDockerfileRecognizedFilenames(t *testing.T) {
	e := testEngine(t)
	for _, name := range []string{"Dockerfile", "Dockerfile.prod", "api.dockerfile", "deploy/Dockerfile"} {
		if !e.Matches(name) {
			t.Errorf("Matches(%q) = false", name)
		}
	}
	for _, name := range []string{"readme.md", "dockerfile-notes.txt", "go.sum"} {
		if e.Matches(name) {
			t.Errorf("Matches(%q) = true", name)
		}
	}
}
