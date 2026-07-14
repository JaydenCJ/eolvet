// Tests for version-pin files: .python-version and friends,
// .tool-versions, and runtime.txt.
package detect

import (
	"testing"
)

func TestVersionFilePins(t *testing.T) {
	e := testEngine(t)
	// Plain pin.
	d := one(t, e.DetectFile(".python-version", []byte("3.9.2\n")))
	if d.Product != "python" || d.Version != "3.9.2" || d.Source != "version-file" {
		t.Fatalf("unexpected decl: %+v", d)
	}
	// .nvmrc convention includes a leading v.
	d = one(t, e.DetectFile(".nvmrc", []byte("v18.16.0\n")))
	if d.Product != "node" || d.Version != "18.16.0" {
		t.Fatalf("unexpected decl: %+v", d)
	}
	// Comments and blank lines precede the pin in tool-managed files.
	d = one(t, e.DetectFile(".ruby-version", []byte("# managed by rbenv\n\n3.1.4\n")))
	if d.Product != "ruby" || d.Version != "3.1.4" || d.Line != 3 {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestVersionFileAliasesSkipped(t *testing.T) {
	// "lts/hydrogen" and "system" name no concrete release — nothing
	// honest to date, so nothing is reported.
	e := testEngine(t)
	for name, content := range map[string]string{
		".nvmrc":          "lts/hydrogen\n",
		".python-version": "pypy3.9-7.3.9\n",
		".ruby-version":   "system\n",
	} {
		if decls := e.DetectFile(name, []byte(content)); len(decls) != 0 {
			t.Errorf("%s: expected no declarations, got %+v", name, decls)
		}
	}
}

func TestToolVersionsMultipleTools(t *testing.T) {
	content := "# team pins\nnodejs 18.16.0\npython 3.9.2 3.8.10\ngolang 1.20.1\nshellcheck 0.9.0\n"
	decls := testEngine(t).DetectFile(".tool-versions", []byte(content))
	if len(decls) != 3 {
		t.Fatalf("expected 3 declarations (shellcheck has no lifecycle data), got %+v", decls)
	}
	if decls[0].Product != "node" || decls[0].Version != "18.16.0" || decls[0].Line != 2 {
		t.Fatalf("node decl: %+v", decls[0])
	}
	// asdf uses the first version as the active one; fallbacks are noise.
	if decls[1].Product != "python" || decls[1].Version != "3.9.2" {
		t.Fatalf("python decl: %+v", decls[1])
	}
	if decls[2].Product != "go" || decls[2].Version != "1.20.1" {
		t.Fatalf("go decl: %+v", decls[2])
	}
}

func TestToolVersionsJavaDistributionPrefix(t *testing.T) {
	d := one(t, testEngine(t).DetectFile(".tool-versions", []byte("java temurin-21.0.2+13.0.LTS\n")))
	if d.Product != "java" || d.Version != "21.0.2+13.0.LTS" {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestToolVersionsRefPinsSkipped(t *testing.T) {
	if decls := testEngine(t).DetectFile(".tool-versions", []byte("nodejs ref:main\n")); len(decls) != 0 {
		t.Fatalf("ref pins should be skipped, got %+v", decls)
	}
}

func TestRuntimeTxtPython(t *testing.T) {
	d := one(t, testEngine(t).DetectFile("runtime.txt", []byte("python-3.8.10\n")))
	if d.Product != "python" || d.Version != "3.8.10" || d.Source != "runtime-txt" {
		t.Fatalf("unexpected decl: %+v", d)
	}
}

func TestRuntimeTxtUnknownRuntimeSkipped(t *testing.T) {
	e := testEngine(t)
	for _, content := range []string{"erlang-25.0\n", "notaversion\n", ""} {
		if decls := e.DetectFile("runtime.txt", []byte(content)); len(decls) != 0 {
			t.Errorf("%q: expected no declarations, got %+v", content, decls)
		}
	}
}
