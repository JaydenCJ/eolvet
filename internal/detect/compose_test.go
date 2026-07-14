// Tests for the compose-file detector.
package detect

import (
	"testing"
)

func detectCompose(t *testing.T, content string) []Decl {
	t.Helper()
	return testEngine(t).DetectFile("docker-compose.yml", []byte(content))
}

func TestComposeImageLines(t *testing.T) {
	content := `services:
  db:
    image: postgres:12
  cache:
    image: "redis:7.2"
`
	decls := detectCompose(t, content)
	if len(decls) != 2 {
		t.Fatalf("expected 2 declarations, got %+v", decls)
	}
	if decls[0].Product != "postgres" || decls[0].Version != "12" || decls[0].Line != 3 {
		t.Fatalf("db decl: %+v", decls[0])
	}
	if decls[1].Product != "redis" || decls[1].Version != "7.2" || decls[1].Line != 5 {
		t.Fatalf("cache decl (quotes must strip): %+v", decls[1])
	}
}

func TestComposeCommentsAndUnmappedImages(t *testing.T) {
	content := "# image: postgres:9.6\nservices:\n  db:\n    image: mysql:5.7 # legacy\n"
	d := one(t, detectCompose(t, content))
	if d.Product != "mysql" || d.Version != "5.7" {
		t.Fatalf("unexpected decl: %+v", d)
	}
	if decls := detectCompose(t, "    image: examplecorp/worker:1.0\n"); len(decls) != 0 {
		t.Fatalf("unmappable image should yield nothing, got %+v", decls)
	}
}

func TestComposeVariableDefaultsResolve(t *testing.T) {
	// ${TAG:-16} carries an auditable default; plain ${TAG} does not.
	decls := detectCompose(t, "    image: postgres:${TAG:-16}\n")
	d := one(t, decls)
	if d.Version != "16" {
		t.Fatalf("default should resolve: %+v", d)
	}
	decls = detectCompose(t, "    image: postgres:${TAG}\n")
	d = one(t, decls)
	if d.Version != "" || d.Note == "" {
		t.Fatalf("unresolved variable should be an explained unknown: %+v", d)
	}
}

func TestComposeFilenames(t *testing.T) {
	e := testEngine(t)
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		if !e.Matches(name) {
			t.Errorf("Matches(%q) = false", name)
		}
	}
	if e.Matches("docker-compose.override.example") {
		t.Error("unexpected match")
	}
}
