// Tests for language-manifest detectors: go.mod, package.json,
// pyproject.toml, Gemfile, composer.json.
package detect

import (
	"testing"
)

func TestGoModDirective(t *testing.T) {
	d := one(t, testEngine(t).DetectFile("go.mod", []byte("module example.test/app\n\ngo 1.21\n")))
	if d.Product != "go" || d.Version != "1.21" || d.Line != 3 || d.Source != "go-mod" {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestGoModToolchainWinsOverDirective(t *testing.T) {
	// The toolchain directive names the compiler that actually builds
	// the module — that is the release whose support window matters.
	content := "module example.test/app\n\ngo 1.19\n\ntoolchain go1.22.3\n"
	d := one(t, testEngine(t).DetectFile("go.mod", []byte(content)))
	if d.Version != "1.22.3" || d.Note != "toolchain directive" {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestGoModWithoutDirectiveOrWithBadContent(t *testing.T) {
	e := testEngine(t)
	if decls := e.DetectFile("go.mod", []byte("module example.test/app\n")); len(decls) != 0 {
		t.Fatalf("expected no declarations, got %+v", decls)
	}
	if decls := e.DetectFile("go.mod", []byte("go here\n")); len(decls) != 0 {
		t.Fatalf("non-version go directive should be skipped, got %+v", decls)
	}
}

func TestPackageJSONEnginesNode(t *testing.T) {
	content := `{
  "name": "app",
  "dependencies": { "node-fetch": "^3.0.0" },
  "engines": { "node": ">=18.17 <21" }
}`
	d := one(t, testEngine(t).DetectFile("package.json", []byte(content)))
	if d.Product != "node" || d.Version != "18.17" || d.Source != "package-json" {
		t.Fatalf("unexpected decl: %+v", d)
	}
	if d.Line != 4 {
		t.Fatalf("line should anchor at the engines block, got %d", d.Line)
	}
	if d.Raw != ">=18.17 <21" {
		t.Fatalf("Raw should keep the constraint, got %q", d.Raw)
	}
}

func TestPackageJSONEdgeCases(t *testing.T) {
	e := testEngine(t)
	if decls := e.DetectFile("package.json", []byte(`{"name": "app"}`)); len(decls) != 0 {
		t.Fatalf("expected no declarations, got %+v", decls)
	}
	// Broken JSON is someone else's lint problem, not a crash.
	if decls := e.DetectFile("package.json", []byte(`{not json`)); len(decls) != 0 {
		t.Fatalf("expected no declarations for broken JSON, got %+v", decls)
	}
	// An unbounded engines range cannot be judged; it must surface as
	// an explained unknown, not a guess.
	d := one(t, e.DetectFile("package.json", []byte(`{"engines": {"node": "*"}}`)))
	if d.Version != "" || d.Note == "" {
		t.Fatalf("unbounded constraint should be an explained unknown: %+v", d)
	}
}

func TestPyprojectRequiresPython(t *testing.T) {
	content := "[project]\nname = \"app\"\nrequires-python = \">=3.9\"\n"
	d := one(t, testEngine(t).DetectFile("pyproject.toml", []byte(content)))
	if d.Product != "python" || d.Version != "3.9" || d.Line != 3 {
		t.Fatalf("unexpected decl: %+v", d)
	}
	if d.Note != "floor of constraint >=3.9" {
		t.Fatalf("note = %q", d.Note)
	}
	// A requires-python-looking key outside [project] must not count.
	other := "[tool.other]\nrequires-python = \">=2.7\"\n"
	if decls := testEngine(t).DetectFile("pyproject.toml", []byte(other)); len(decls) != 0 {
		t.Fatalf("expected no declarations, got %+v", decls)
	}
}

func TestPyprojectPoetryPythonConstraint(t *testing.T) {
	content := "[tool.poetry]\nname = \"app\"\n\n[tool.poetry.dependencies]\npython = \"^3.10\"\nrequests = \"*\"\n"
	d := one(t, testEngine(t).DetectFile("pyproject.toml", []byte(content)))
	if d.Product != "python" || d.Version != "3.10" || d.Line != 5 {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestGemfileRubyPin(t *testing.T) {
	content := "source \"https://rubygems.example.test\"\n\nruby \"3.1.4\"\n\ngem \"rails\"\n"
	d := one(t, testEngine(t).DetectFile("Gemfile", []byte(content)))
	if d.Product != "ruby" || d.Version != "3.1.4" || d.Line != 3 || d.Source != "gemfile" {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestGemfileConstraintAndFileIndirection(t *testing.T) {
	d := one(t, testEngine(t).DetectFile("Gemfile", []byte("ruby '~> 3.2'\n")))
	if d.Version != "3.2" {
		t.Fatalf("unexpected decl: %+v", d)
	}
	// `ruby file: ".ruby-version"` delegates to a file the version-file
	// detector already reads — double-reporting would inflate counts.
	if decls := testEngine(t).DetectFile("Gemfile", []byte("ruby file: \".ruby-version\"\n")); len(decls) != 0 {
		t.Fatalf("expected no declarations, got %+v", decls)
	}
}

func TestComposerJSONPhpConstraint(t *testing.T) {
	content := `{
  "require": {
    "php": ">=8.1",
    "monolog/monolog": "^3.0"
  }
}`
	d := one(t, testEngine(t).DetectFile("composer.json", []byte(content)))
	if d.Product != "php" || d.Version != "8.1" || d.Source != "composer-json" {
		t.Fatalf("unexpected decl: %+v", d)
	}
	if d.Line != 3 {
		t.Fatalf("line = %d, want 3", d.Line)
	}
}
