// Tests for image reference parsing and the image → product mapping.
package detect

import (
	"testing"
)

func TestParseImageRefTagAndDigest(t *testing.T) {
	ref := ParseImageRef("python:3.9-slim")
	if ref.Repo != "python" || ref.Tag != "3.9-slim" || ref.Digest != "" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	ref = ParseImageRef("python@sha256:abc123")
	if ref.Repo != "python" || ref.Tag != "" || ref.Digest != "sha256:abc123" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	ref = ParseImageRef("python:3.9@sha256:abc123")
	if ref.Repo != "python" || ref.Tag != "3.9" || ref.Digest != "sha256:abc123" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestParseImageRefRegistryPortIsNotATag(t *testing.T) {
	// The ":5000" is the registry port; the reference has no tag.
	ref := ParseImageRef("registry.example.test:5000/python")
	if ref.Repo != "python" || ref.Tag != "" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	ref = ParseImageRef("registry.example.test:5000/python:3.9")
	if ref.Repo != "python" || ref.Tag != "3.9" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestParseImageRefNormalizesNamespaces(t *testing.T) {
	for _, raw := range []string{
		"python",
		"library/python",
		"docker.io/library/python",
		"public.ecr.aws/docker/library/python",
		"localhost/python",
	} {
		if got := ParseImageRef(raw).Repo; got != "python" {
			t.Errorf("ParseImageRef(%q).Repo = %q, want python", raw, got)
		}
	}
}

func TestImageProductMappings(t *testing.T) {
	for raw, want := range map[string]string{
		"golang:1.22":                     "go",
		"eclipse-temurin:17":              "java",
		"mongo:6.0":                       "mongodb",
		"mcr.microsoft.com/dotnet/sdk":    "dotnet",
		"mcr.microsoft.com/dotnet/aspnet": "dotnet",
		"rockylinux:9":                    "rockylinux",
		"mirror.example.test/python":      "python",
	} {
		p, ok := ParseImageRef(raw).Product()
		if !ok || p != want {
			t.Errorf("Product(%q) = %q, %v; want %q", raw, p, ok, want)
		}
	}
	// Images without lifecycle data must map to nothing, not to a guess.
	for _, raw := range []string{"examplecorp/api", "traefik:v2.10", "busybox"} {
		if p, ok := ParseImageRef(raw).Product(); ok {
			t.Errorf("Product(%q) = %q, want unmapped", raw, p)
		}
	}
}

func TestTagVersionExtraction(t *testing.T) {
	for tag, want := range map[string]string{
		"3.8-slim-bullseye": "3.8",
		"18-alpine3.17":     "18",
		"22.04":             "22.04",
		"latest":            "",
		"bullseye-20230522": "",
		"":                  "",
	} {
		if got := tagVersion(tag); got != want {
			t.Errorf("tagVersion(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestTagBaseHints(t *testing.T) {
	isCodename := func(product, token string) bool {
		return (product == "debian" && token == "bullseye") ||
			(product == "ubuntu" && token == "jammy")
	}
	hints := tagBaseHints("3.8-slim-bullseye", isCodename)
	if len(hints) != 1 || hints[0].product != "debian" || hints[0].version != "bullseye" {
		t.Fatalf("hints = %+v", hints)
	}
	hints = tagBaseHints("17-jdk-jammy", isCodename)
	if len(hints) != 1 || hints[0].product != "ubuntu" || hints[0].version != "jammy" {
		t.Fatalf("hints = %+v", hints)
	}
	hints = tagBaseHints("1.20-alpine3.17", isCodename)
	if len(hints) != 1 || hints[0].product != "alpine" || hints[0].version != "3.17" {
		t.Fatalf("hints = %+v", hints)
	}
	// The version token itself must never be scanned as a hint, and
	// bare "-alpine" pins nothing.
	if hints = tagBaseHints("18-alpine", isCodename); len(hints) != 0 {
		t.Fatalf("bare -alpine should hint nothing, got %+v", hints)
	}
}
