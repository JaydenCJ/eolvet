// Tests for status judgment and the three renderers.
package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaydenCJ/eolvet/internal/detect"
	"github.com/JaydenCJ/eolvet/internal/eoldata"
)

func asOf(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := eoldata.ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func snapshot(t *testing.T) *eoldata.Snapshot {
	t.Helper()
	s, err := eoldata.Load()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func judgeOne(t *testing.T, d detect.Decl, day string) Finding {
	t.Helper()
	fs := Evaluate([]detect.Decl{d}, snapshot(t), asOf(t, day), 90)
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	return fs[0]
}

func TestJudgeEOL(t *testing.T) {
	f := judgeOne(t, detect.Decl{Product: "python", Version: "3.8.10", Raw: "3.8.10"}, "2026-07-13")
	if f.Status != StatusEOL || f.Cycle != "3.8" || f.EOLDate != "2024-10-07" {
		t.Fatalf("unexpected finding: %+v", f)
	}
	if f.DaysLeft != -644 {
		t.Fatalf("DaysLeft = %d, want -644", f.DaysLeft)
	}
	if f.Label != "Python" {
		t.Fatalf("Label = %q", f.Label)
	}
}

func TestJudgeEOLSoonWindow(t *testing.T) {
	// Debian 11 goes EOL 2026-08-31: 49 days out on 2026-07-13, inside
	// the default 90-day window.
	f := judgeOne(t, detect.Decl{Product: "debian", Version: "bullseye", Raw: "bullseye"}, "2026-07-13")
	if f.Status != StatusEOLSoon || f.DaysLeft != 49 {
		t.Fatalf("unexpected finding: %+v", f)
	}
	// The same declaration judged earlier is comfortably supported.
	f = judgeOne(t, detect.Decl{Product: "debian", Version: "bullseye", Raw: "bullseye"}, "2025-01-01")
	if f.Status != StatusOK {
		t.Fatalf("unexpected finding: %+v", f)
	}
}

func TestJudgeEOLDayBoundary(t *testing.T) {
	// On the EOL date itself the cycle is end-of-life — a one-day
	// off-by-one here is precisely the bug an auditor cannot accept.
	f := judgeOne(t, detect.Decl{Product: "python", Version: "3.8"}, "2024-10-07")
	if f.Status != StatusEOL || f.DaysLeft != 0 {
		t.Fatalf("on the EOL day: %+v", f)
	}
	f = judgeOne(t, detect.Decl{Product: "python", Version: "3.8"}, "2024-10-06")
	if f.Status != StatusEOLSoon || f.DaysLeft != 1 {
		t.Fatalf("day before EOL: %+v", f)
	}
}

func TestJudgeWarnWindowIsConfigurable(t *testing.T) {
	d := detect.Decl{Product: "debian", Version: "bullseye"}
	fs := Evaluate([]detect.Decl{d}, snapshot(t), asOf(t, "2026-07-13"), 30)
	if fs[0].Status != StatusOK {
		t.Fatalf("49 days out with a 30-day window should be OK: %+v", fs[0])
	}
}

func TestJudgeUnknowns(t *testing.T) {
	// Product missing from the (custom) snapshot.
	f := judgeOne(t, detect.Decl{Product: "cobol", Version: "85", Raw: "cobol-85"}, "2026-07-13")
	if f.Status != StatusUnknown || !strings.Contains(f.Note, "product not in snapshot") {
		t.Fatalf("unexpected finding: %+v", f)
	}
	// Version present but no matching cycle.
	f = judgeOne(t, detect.Decl{Product: "python", Version: "1.5", Raw: "1.5"}, "2026-07-13")
	if f.Status != StatusUnknown || !strings.Contains(f.Note, "no matching release cycle") {
		t.Fatalf("unexpected finding: %+v", f)
	}
	// Unresolved version keeps the detector's explanation.
	f = judgeOne(t, detect.Decl{Product: "redis", Version: "", Raw: "redis:latest", Note: "unpinned tag"}, "2026-07-13")
	if f.Status != StatusUnknown || f.Note != "unpinned tag" {
		t.Fatalf("unexpected finding: %+v", f)
	}
}

func TestSummarizeCounts(t *testing.T) {
	findings := []Finding{
		{Status: StatusEOL}, {Status: StatusEOL},
		{Status: StatusEOLSoon},
		{Status: StatusOK}, {Status: StatusOK}, {Status: StatusOK},
		{Status: StatusUnknown},
	}
	s := Summarize(findings)
	if s.Total != 7 || s.EOL != 2 || s.EOLSoon != 1 || s.Supported != 3 || s.Unknown != 1 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestSortBySeverityWorstFirst(t *testing.T) {
	findings := []Finding{
		{Status: StatusOK, File: "a"},
		{Status: StatusUnknown, File: "b"},
		{Status: StatusEOLSoon, File: "c"},
		{Status: StatusEOL, File: "z"},
		{Status: StatusEOL, File: "d"},
	}
	SortBySeverity(findings)
	want := []Status{StatusEOL, StatusEOL, StatusEOLSoon, StatusUnknown, StatusOK}
	for i, s := range want {
		if findings[i].Status != s {
			t.Fatalf("position %d: %+v", i, findings)
		}
	}
	// Ties break by file for reproducible reports.
	if findings[0].File != "d" || findings[1].File != "z" {
		t.Fatalf("tie-break by file failed: %+v", findings[:2])
	}
}

func demoReport(t *testing.T) *Report {
	t.Helper()
	decls := []detect.Decl{
		{Product: "python", Version: "3.8", Raw: "python:3.8-slim-bullseye", File: "Dockerfile", Line: 2, Source: "dockerfile"},
		{Product: "debian", Version: "bullseye", Raw: "python:3.8-slim-bullseye", File: "Dockerfile", Line: 2, Source: "dockerfile", Note: "base OS of image tag"},
		{Product: "go", Version: "1.26", Raw: "go 1.26", File: "go.mod", Line: 3, Source: "go-mod"},
		{Product: "redis", Version: "", Raw: "redis:latest", File: "docker-compose.yml", Line: 5, Source: "compose", Note: "unpinned tag"},
	}
	findings := Evaluate(decls, snapshot(t), asOf(t, "2026-07-13"), 90)
	SortBySeverity(findings)
	return &Report{
		Path: "demo", SnapshotDate: "2026-06-15", AsOf: "2026-07-13",
		WarnDays: 90, Findings: findings, Summary: Summarize(findings),
	}
}

func TestRenderTextTableAndSummary(t *testing.T) {
	out := RenderText(demoReport(t))
	for _, want := range []string{
		"eolvet scan — demo (snapshot 2026-06-15, as of 2026-07-13)",
		"STATUS", "EOL-SOON", "Python", "3.8", "2024-10-07", "-644d",
		"Dockerfile:2", "redis:latest", "(unpinned tag)",
		"4 declarations: 1 eol, 1 eol-soon, 1 supported, 1 unknown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q:\n%s", want, out)
		}
	}
	// Severity order: the EOL row must appear before the OK row.
	if strings.Index(out, "Python") > strings.Index(out, "go.mod") {
		t.Fatal("EOL finding should render before supported findings")
	}
	// An empty report degrades to a sentence, not an empty table.
	empty := RenderText(&Report{Path: "empty", SnapshotDate: "2026-06-15", AsOf: "2026-07-13"})
	if !strings.Contains(empty, "no version declarations found") {
		t.Fatalf("unexpected empty output: %q", empty)
	}
}

func TestRenderJSONEnvelopeAndRoundTrip(t *testing.T) {
	out, err := RenderJSON(demoReport(t))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tool          string    `json:"tool"`
		SchemaVersion int       `json:"schema_version"`
		SnapshotDate  string    `json:"snapshot_date"`
		Findings      []Finding `json:"findings"`
		Summary       Summary   `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if doc.Tool != "eolvet" || doc.SchemaVersion != 1 || doc.SnapshotDate != "2026-06-15" {
		t.Fatalf("envelope wrong: %+v", doc)
	}
	if len(doc.Findings) != 4 || doc.Summary.EOL != 1 {
		t.Fatalf("payload wrong: %+v", doc)
	}
	if doc.Findings[0].Status != StatusEOL || doc.Findings[0].DaysLeft != -644 {
		t.Fatalf("first finding: %+v", doc.Findings[0])
	}
	// No findings must serialize as [], never null — consumers index it.
	empty, err := RenderJSON(&Report{Path: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty, `"findings": []`) {
		t.Fatalf("empty findings must serialize as [], got:\n%s", empty)
	}
}

func TestRenderMarkdownTable(t *testing.T) {
	out := RenderMarkdown(demoReport(t))
	for _, want := range []string{
		"## eolvet report — demo",
		"| Status | Product | Cycle | EOL | Days | Where | Declared |",
		"| **EOL** | Python | 3.8 | 2024-10-07 | -644 | `Dockerfile:2` |",
		"`redis:latest` — unpinned tag",
		"**4 declarations: 1 eol, 1 eol-soon, 1 supported, 1 unknown**",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderMarkdownEscapesPipes(t *testing.T) {
	r := &Report{
		Path: "p", SnapshotDate: "2026-06-15", AsOf: "2026-07-13", WarnDays: 90,
		Findings: []Finding{{Status: StatusUnknown, Label: "Node.js", Raw: ">=18 || >=20", File: "package.json", Line: 1}},
	}
	r.Summary = Summarize(r.Findings)
	out := RenderMarkdown(r)
	if !strings.Contains(out, `>=18 \|\| >=20`) {
		t.Fatalf("pipes must be escaped in table cells:\n%s", out)
	}
}
