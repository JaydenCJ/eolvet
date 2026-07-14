// In-process CLI integration tests: real argument parsing, real
// filesystem walks over fabricated repositories, real exit codes.
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes the CLI in-process and captures both streams.
func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// demoRepo fabricates a small polyglot repository with one EOL runtime,
// one soon-to-expire base, one supported runtime, and one unknown.
func demoRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"Dockerfile":         "ARG PY=3.8\nFROM python:${PY}-slim-bullseye AS build\nFROM build\n",
		"go.mod":             "module demo\n\ngo 1.26\n",
		"docker-compose.yml": "services:\n  cache:\n    image: redis:latest\n",
	}
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

func TestScanTextReportAndBreachExit(t *testing.T) {
	code, out, _ := run(t, "scan", "--as-of", "2026-07-13", demoRepo(t))
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (EOL findings with default --fail-on eol)", code)
	}
	for _, want := range []string{
		"snapshot 2026-06-15, as of 2026-07-13",
		"EOL", "Python", "3.8", "-644d",
		"EOL-SOON", "Debian", "11", "+49d",
		"OK", "Go", "1.26",
		"UNKNOWN", "Redis", "redis:latest",
		"4 declarations: 1 eol, 1 eol-soon, 1 supported, 1 unknown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("scan output missing %q:\n%s", want, out)
		}
	}
	// A bare path (no subcommand) implies scan.
	code, out, _ = run(t, demoRepo(t), "--as-of", "2026-07-13")
	if code != 1 || !strings.Contains(out, "eolvet scan") {
		t.Fatalf("bare path should imply scan: code=%d out=%s", code, out)
	}
}

func TestScanFailOnPolicies(t *testing.T) {
	repo := demoRepo(t)
	if code, _, _ := run(t, "scan", "--as-of", "2026-07-13", "--fail-on", "none", repo); code != 0 {
		t.Fatalf("--fail-on none should exit 0, got %d", code)
	}
	// Exclude the EOL Dockerfile; the remaining findings are OK+unknown,
	// so --fail-on eol-soon passes but --strict still trips on unknown.
	if code, _, _ := run(t, "scan", "--as-of", "2026-07-13", "--exclude", "Dockerfile", "--fail-on", "eol-soon", repo); code != 0 {
		t.Fatalf("no eol/eol-soon left after exclude, want 0, got %d", code)
	}
	if code, _, _ := run(t, "scan", "--as-of", "2026-07-13", "--exclude", "Dockerfile", "--strict", repo); code != 1 {
		t.Fatal("--strict should fail on the unknown redis:latest finding")
	}
}

func TestScanJSONOutput(t *testing.T) {
	code, out, _ := run(t, "scan", "--format", "json", "--as-of", "2026-07-13", demoRepo(t))
	if code != 1 {
		t.Fatalf("exit = %d", code)
	}
	var doc struct {
		Tool     string `json:"tool"`
		AsOf     string `json:"as_of"`
		Findings []struct {
			Status  string `json:"status"`
			Product string `json:"product"`
			File    string `json:"file"`
		} `json:"findings"`
		Summary struct {
			Total int `json:"total"`
			EOL   int `json:"eol"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if doc.Tool != "eolvet" || doc.AsOf != "2026-07-13" || doc.Summary.Total != 4 || doc.Summary.EOL != 1 {
		t.Fatalf("unexpected document: %+v", doc)
	}
	if doc.Findings[0].Status != "eol" || doc.Findings[0].Product != "python" {
		t.Fatalf("first finding: %+v", doc.Findings[0])
	}
}

func TestScanMarkdownOutput(t *testing.T) {
	code, out, _ := run(t, "scan", "--format", "markdown", "--as-of", "2026-07-13", demoRepo(t))
	if code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "| **EOL** | Python | 3.8 |") {
		t.Fatalf("markdown table missing:\n%s", out)
	}
	if !strings.Contains(out, "Snapshot 2026-06-15 · as of 2026-07-13") {
		t.Fatalf("markdown header missing:\n%s", out)
	}
}

func TestScanWarnWithinNarrowsWindow(t *testing.T) {
	// Debian 11 is 49 days out on 2026-07-13; a 30-day window keeps it OK.
	code, out, _ := run(t, "scan", "--as-of", "2026-07-13", "--warn-within", "30", demoRepo(t))
	if code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "0 eol-soon") {
		t.Fatalf("expected no eol-soon findings with 30-day window:\n%s", out)
	}
}

func TestScanCustomSnapshotViaData(t *testing.T) {
	// A policy table that omits redis entirely and marks go 1.26 EOL:
	// the scan must judge by the user's table, not the bundled one.
	repo := demoRepo(t)
	data := filepath.Join(t.TempDir(), "policy.json")
	doc := `{
	  "schema_version": 1, "snapshot_date": "2026-07-01", "source": "org policy",
	  "products": {
	    "python": {"label": "Python", "cycles": [{"cycle": "3.8", "release": "2019-10-14", "eol": "2024-10-07"}]},
	    "debian": {"label": "Debian", "codenames": {"bullseye": "11"},
	      "cycles": [{"cycle": "11", "release": "2021-08-14", "eol": "2026-08-31"}]},
	    "go": {"label": "Go", "cycles": [{"cycle": "1.26", "release": "2026-02-10", "eol": "2026-07-01"}]}
	  }
	}`
	if err := os.WriteFile(data, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(t, "scan", "--as-of", "2026-07-13", "--data", data, repo)
	if code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "snapshot 2026-07-01") {
		t.Fatalf("custom snapshot date missing:\n%s", out)
	}
	if !strings.Contains(out, "2 eol") {
		t.Fatalf("go 1.26 should be EOL under the custom table:\n%s", out)
	}
	if !strings.Contains(out, "product not in snapshot") {
		t.Fatalf("redis should be unknown under the custom table:\n%s", out)
	}
}

func TestScanUsageErrors(t *testing.T) {
	repo := demoRepo(t)
	for _, args := range [][]string{
		{"scan", "--format", "yaml", repo},
		{"scan", "--fail-on", "always", repo},
		{"scan", "--as-of", "someday", repo},
		{"scan", "--warn-within", "-3", repo},
		{"scan", "--exclude", "[oops", repo},
		{"scan", repo, "extra-path"},
		{"check", "python"},
		{"products", "extra"},
	} {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("%v: exit = %d, want 2", args, code)
		}
	}
	// A missing path is a runtime error (3), distinct from bad usage (2).
	code, _, errOut := run(t, "scan", filepath.Join(t.TempDir(), "absent"))
	if code != 3 || errOut == "" {
		t.Errorf("missing path: exit = %d, stderr = %q; want 3 with a message", code, errOut)
	}
}

func TestCheckStatusesAndExitCodes(t *testing.T) {
	code, out, _ := run(t, "check", "python", "3.8", "--as-of", "2026-07-13")
	if code != 1 || !strings.Contains(out, "Python 3.8 — EOL since 2024-10-07, 644 days ago") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out, _ = run(t, "check", "debian", "bullseye", "--as-of", "2026-07-13")
	if code != 0 || !strings.Contains(out, "EOL SOON on 2026-08-31, in 49 days") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out, _ = run(t, "check", "go", "1.26.1", "--as-of", "2026-07-13")
	if code != 0 || !strings.Contains(out, "supported until 2027-02-09") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	// --fail-on eol-soon promotes the warning to a breach.
	if code, _, _ = run(t, "check", "debian", "bullseye", "--as-of", "2026-07-13", "--fail-on", "eol-soon"); code != 1 {
		t.Fatalf("eol-soon with --fail-on eol-soon should exit 1, got %d", code)
	}
}

func TestCheckUnknownProductAndVersion(t *testing.T) {
	code, _, errOut := run(t, "check", "cobol", "85")
	if code != 2 || !strings.Contains(errOut, "unknown product") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	code, _, errOut = run(t, "check", "python", "1.5")
	if code != 2 || !strings.Contains(errOut, "no release cycle matching") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestProductsListsSnapshot(t *testing.T) {
	code, out, _ := run(t, "products")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"snapshot 2026-06-15", "python", "Python", "debian", "alpine", "products,"} {
		if !strings.Contains(out, want) {
			t.Errorf("products output missing %q:\n%s", want, out)
		}
	}
}

func TestProductsJSON(t *testing.T) {
	code, out, _ := run(t, "products", "--format", "json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var doc struct {
		Tool     string `json:"tool"`
		Products []struct {
			Product string `json:"product"`
			Cycles  int    `json:"cycles"`
		} `json:"products"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if doc.Tool != "eolvet" || len(doc.Products) < 15 {
		t.Fatalf("unexpected document: %+v", doc)
	}
}

func TestVersionAndHelp(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		code, out, _ := run(t, args...)
		if code != 0 || !strings.Contains(out, "eolvet 0.1.0 (snapshot 2026-06-15)") {
			t.Fatalf("%v: code=%d out=%q", args, code, out)
		}
	}
	code, out, _ := run(t, "help")
	if code != 0 || !strings.Contains(out, "eolvet scan [flags] [path]") {
		t.Fatalf("help: code=%d out=%q", code, out)
	}
}

func TestScanEmptyDirectory(t *testing.T) {
	code, out, _ := run(t, "scan", "--as-of", "2026-07-13", t.TempDir())
	if code != 0 || !strings.Contains(out, "no version declarations found") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestScanSingleFileArgument(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Dockerfile")
	if err := os.WriteFile(p, []byte("FROM node:16-bullseye\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(t, "scan", "--as-of", "2026-07-13", p)
	if code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "Node.js") || !strings.Contains(out, "Debian") {
		t.Fatalf("both findings expected:\n%s", out)
	}
}
