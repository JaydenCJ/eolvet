// Tests for the bundled snapshot and its loader: the embedded table
// must be internally consistent, and a user-supplied table must be
// validated just as strictly.
package eoldata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustLoad(t *testing.T) *Snapshot {
	t.Helper()
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func TestEmbeddedSnapshotLoadsAndValidates(t *testing.T) {
	s := mustLoad(t)
	if s.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d", s.SchemaVersion)
	}
	if s.SnapshotDate != "2026-06-15" {
		t.Fatalf("snapshot_date = %q", s.SnapshotDate)
	}
	if len(s.Products) < 15 {
		t.Fatalf("expected a substantial product table, got %d", len(s.Products))
	}
	names := s.ProductNames()
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("ProductNames not strictly sorted at %q >= %q", names[i-1], names[i])
		}
	}
}

func TestEmbeddedSnapshotDatesAreCoherent(t *testing.T) {
	// Release dates must not decrease within a product (products,
	// check output, and the OLDEST/NEWEST columns all rely on it), and
	// no cycle may go EOL before it is released.
	s := mustLoad(t)
	for _, name := range s.ProductNames() {
		p, _ := s.Product(name)
		prev := time.Time{}
		for _, c := range p.Cycles {
			rel, err := ParseDate(c.Release)
			if err != nil {
				t.Fatalf("%s %s: %v", name, c.Cycle, err)
			}
			if rel.Before(prev) {
				t.Errorf("%s: cycle %s released %s before its predecessor", name, c.Cycle, c.Release)
			}
			prev = rel
			eol, _ := ParseDate(c.EOL)
			if eol.Before(rel) {
				t.Errorf("%s %s: eol %s precedes release %s", name, c.Cycle, c.EOL, c.Release)
			}
		}
	}
}

func TestResolveConcreteVersionToCycle(t *testing.T) {
	s := mustLoad(t)
	p, _ := s.Product("python")
	c, ok := p.Resolve("3.8.10")
	if !ok || c.Cycle != "3.8" || c.EOL != "2024-10-07" {
		t.Fatalf("Resolve(3.8.10) = %+v, %v", c, ok)
	}
}

func TestResolveCodenameToCycle(t *testing.T) {
	s := mustLoad(t)
	p, _ := s.Product("debian")
	c, ok := p.Resolve("bullseye")
	if !ok || c.Cycle != "11" {
		t.Fatalf("Resolve(bullseye) = %+v, %v", c, ok)
	}
	// Codenames are case-insensitive: FROM debian:Bullseye is legal.
	if c, ok = p.Resolve("Bullseye"); !ok || c.Cycle != "11" {
		t.Fatalf("Resolve(Bullseye) = %+v, %v", c, ok)
	}
}

func TestResolveUnknownVersionFails(t *testing.T) {
	s := mustLoad(t)
	p, _ := s.Product("python")
	if c, ok := p.Resolve("1.5"); ok {
		t.Fatalf("Resolve(1.5) = %+v, want no match", c)
	}
	if c, ok := p.Resolve("latest"); ok {
		t.Fatalf("Resolve(latest) = %+v, want no match", c)
	}
}

func TestIsCodename(t *testing.T) {
	s := mustLoad(t)
	deb, _ := s.Product("debian")
	if !deb.IsCodename("bookworm") || deb.IsCodename("hydrogen") {
		t.Fatal("IsCodename misclassifies")
	}
	ub, _ := s.Product("ubuntu")
	if !ub.IsCodename("jammy") {
		t.Fatal("jammy should be an ubuntu codename")
	}
}

func TestDaysUntil(t *testing.T) {
	a, _ := ParseDate("2026-07-13")
	b, _ := ParseDate("2026-08-31")
	if got := DaysUntil(a, b); got != 49 {
		t.Fatalf("DaysUntil = %d, want 49", got)
	}
	if got := DaysUntil(b, a); got != -49 {
		t.Fatalf("DaysUntil reverse = %d, want -49", got)
	}
	if got := DaysUntil(a, a); got != 0 {
		t.Fatalf("DaysUntil same day = %d, want 0", got)
	}
}

func TestLoadFileAcceptsValidCustomSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.json")
	doc := `{
	  "schema_version": 1,
	  "snapshot_date": "2026-01-01",
	  "source": "org policy",
	  "products": {
	    "python": {"label": "Python", "cycles": [
	      {"cycle": "3.11", "release": "2022-10-24", "eol": "2027-10-31"}
	    ]}
	  }
	}`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if s.SnapshotDate != "2026-01-01" || len(s.Products) != 1 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
}

func TestLoadFileRejectsBadDocuments(t *testing.T) {
	// Every rejection here would otherwise surface as a silent
	// misclassification during a scan.
	cases := map[string]string{
		"bad schema version": `{"schema_version": 2, "snapshot_date": "2026-01-01",
			"products": {"p": {"label": "P", "cycles": [{"cycle": "1", "release": "2020-01-01", "eol": "2021-01-01"}]}}}`,
		"bad snapshot date": `{"schema_version": 1, "snapshot_date": "someday",
			"products": {"p": {"label": "P", "cycles": [{"cycle": "1", "release": "2020-01-01", "eol": "2021-01-01"}]}}}`,
		"no products": `{"schema_version": 1, "snapshot_date": "2026-01-01", "products": {}}`,
		"missing label": `{"schema_version": 1, "snapshot_date": "2026-01-01",
			"products": {"p": {"cycles": [{"cycle": "1", "release": "2020-01-01", "eol": "2021-01-01"}]}}}`,
		"non-numeric cycle": `{"schema_version": 1, "snapshot_date": "2026-01-01",
			"products": {"p": {"label": "P", "cycles": [{"cycle": "stable", "release": "2020-01-01", "eol": "2021-01-01"}]}}}`,
		"duplicate cycle": `{"schema_version": 1, "snapshot_date": "2026-01-01",
			"products": {"p": {"label": "P", "cycles": [
				{"cycle": "1", "release": "2020-01-01", "eol": "2021-01-01"},
				{"cycle": "1", "release": "2020-06-01", "eol": "2022-01-01"}]}}}`,
		"invalid eol date": `{"schema_version": 1, "snapshot_date": "2026-01-01",
			"products": {"p": {"label": "P", "cycles": [{"cycle": "1", "release": "2020-01-01", "eol": "TBD"}]}}}`,
		"dangling codename": `{"schema_version": 1, "snapshot_date": "2026-01-01",
			"products": {"p": {"label": "P", "codenames": {"zesty": "99"},
				"cycles": [{"cycle": "1", "release": "2020-01-01", "eol": "2021-01-01"}]}}}`,
		"not json": `plainly not json`,
	}
	dir := t.TempDir()
	for name, doc := range cases {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
		if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFile(path); err == nil {
			t.Errorf("%s: LoadFile accepted an invalid snapshot", name)
		}
	}
	if _, err := LoadFile(filepath.Join(dir, "absent.json")); err == nil {
		t.Error("expected an error for a missing snapshot file")
	}
}
