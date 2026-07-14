// Tests for the repository walker: dispatch, skip rules, excludes,
// deterministic ordering, and single-file scans.
package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JaydenCJ/eolvet/internal/detect"
	"github.com/JaydenCJ/eolvet/internal/eoldata"
)

// writeTree materializes a map of relative path -> content in a temp dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func testWalk(t *testing.T, files map[string]string, opts Options) []detect.Decl {
	t.Helper()
	snap, err := eoldata.Load()
	if err != nil {
		t.Fatal(err)
	}
	root := writeTree(t, files)
	decls, err := Walk(root, detect.New(snap), opts)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return decls
}

func TestWalkFindsDeclarationsAcrossDetectors(t *testing.T) {
	decls := testWalk(t, map[string]string{
		"Dockerfile":       "FROM python:3.8\n",
		"web/.nvmrc":       "18\n",
		"svc/go.mod":       "module svc\n\ngo 1.21\n",
		"README.md":        "# not a version file\n",
		"docs/runtime.txt": "python-3.9.1\n",
	}, Options{})
	if len(decls) != 4 {
		t.Fatalf("expected 4 declarations, got %+v", decls)
	}
}

func TestWalkOrderIsDeterministic(t *testing.T) {
	files := map[string]string{
		"b/Dockerfile": "FROM node:16\n",
		"a/Dockerfile": "FROM node:18\n",
		"Dockerfile":   "FROM node:20\n",
	}
	first := testWalk(t, files, Options{})
	second := testWalk(t, files, Options{})
	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("expected 3 declarations, got %d/%d", len(first), len(second))
	}
	for i := range first {
		if first[i].File != second[i].File || first[i].Raw != second[i].Raw {
			t.Fatalf("runs disagree at %d: %+v vs %+v", i, first[i], second[i])
		}
	}
	// Sorted by file path: Dockerfile, a/…, b/….
	if first[0].File != "Dockerfile" || first[1].File != "a/Dockerfile" || first[2].File != "b/Dockerfile" {
		t.Fatalf("unexpected order: %+v", first)
	}
}

func TestWalkSkipsVendoredAndHiddenDirs(t *testing.T) {
	decls := testWalk(t, map[string]string{
		"Dockerfile":              "FROM python:3.12\n",
		"node_modules/x/.nvmrc":   "14\n",
		"vendor/y/go.mod":         "module y\n\ngo 1.19\n",
		".git/objects/Dockerfile": "FROM node:12\n",
		"build/Dockerfile":        "FROM node:12\n",
	}, Options{})
	if len(decls) != 1 || decls[0].File != "Dockerfile" {
		t.Fatalf("vendored/hidden dirs must be skipped, got %+v", decls)
	}
	// Hidden *directories* are skipped, but .nvmrc and .python-version
	// are hidden *files* — exactly the ones we want.
	decls = testWalk(t, map[string]string{".python-version": "3.9\n"}, Options{})
	if len(decls) != 1 || decls[0].Product != "python" {
		t.Fatalf("hidden pin files must be scanned, got %+v", decls)
	}
}

func TestWalkExcludeGlobs(t *testing.T) {
	files := map[string]string{
		"Dockerfile":        "FROM python:3.12\n",
		"legacy/Dockerfile": "FROM python:2.7\n",
	}
	decls := testWalk(t, files, Options{Excludes: []string{"legacy/**"}})
	if len(decls) != 1 || decls[0].File != "Dockerfile" {
		t.Fatalf("exclude glob not applied, got %+v", decls)
	}
	// A directory name alone also excludes its subtree.
	decls = testWalk(t, files, Options{Excludes: []string{"legacy"}})
	if len(decls) != 1 {
		t.Fatalf("bare directory exclude not applied, got %+v", decls)
	}
}

func TestWalkMaxFileSizeSkipsLargeFiles(t *testing.T) {
	big := "# padding\n"
	for len(big) < 128 {
		big += "# padding\n"
	}
	decls := testWalk(t, map[string]string{
		"Dockerfile": big + "FROM python:3.8\n",
		".nvmrc":     "18\n",
	}, Options{MaxFileSize: 64})
	if len(decls) != 1 || decls[0].Product != "node" {
		t.Fatalf("oversized file should be skipped, got %+v", decls)
	}
}

func TestWalkSingleFileArgument(t *testing.T) {
	snap, err := eoldata.Load()
	if err != nil {
		t.Fatal(err)
	}
	root := writeTree(t, map[string]string{"deploy/Dockerfile": "FROM ubuntu:20.04\n"})
	decls, err := Walk(filepath.Join(root, "deploy", "Dockerfile"), detect.New(snap), Options{})
	if err != nil {
		t.Fatalf("Walk(file): %v", err)
	}
	if len(decls) != 1 || decls[0].Product != "ubuntu" || decls[0].File != "Dockerfile" {
		t.Fatalf("unexpected decls: %+v", decls)
	}
	// A missing path is an error, never an empty (falsely green) report.
	if _, err := Walk(filepath.Join(root, "absent"), detect.New(snap), Options{}); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}
