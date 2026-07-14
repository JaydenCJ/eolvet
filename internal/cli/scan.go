// The scan subcommand: walk, detect, judge, render, gate.
package cli

import (
	"fmt"
	"io"

	"github.com/JaydenCJ/eolvet/internal/detect"
	"github.com/JaydenCJ/eolvet/internal/report"
	"github.com/JaydenCJ/eolvet/internal/scan"
)

func runScan(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("scan", stderr)
	format := fs.String("format", "text", "")
	asOfFlag := fs.String("as-of", "", "")
	warnDays := fs.Int("warn-within", 90, "")
	failOn := fs.String("fail-on", "eol", "")
	strict := fs.Bool("strict", false, "")
	dataPath := fs.String("data", "", "")
	maxFileSize := fs.Int64("max-file-size", scan.DefaultMaxFileSize, "")
	var excludes multiFlag
	fs.Var(&excludes, "exclude", "")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(positional) > 1 {
		fmt.Fprintf(stderr, "eolvet: scan takes at most one path, got %d\n", len(positional))
		return exitUsage
	}
	path := "."
	if len(positional) == 1 {
		path = positional[0]
	}
	switch *format {
	case "text", "json", "markdown":
	default:
		fmt.Fprintf(stderr, "eolvet: unknown --format %q (want text, json, or markdown)\n", *format)
		return exitUsage
	}
	switch *failOn {
	case "eol", "eol-soon", "none":
	default:
		fmt.Fprintf(stderr, "eolvet: unknown --fail-on %q (want eol, eol-soon, or none)\n", *failOn)
		return exitUsage
	}
	if *warnDays < 0 {
		fmt.Fprintln(stderr, "eolvet: --warn-within must be >= 0")
		return exitUsage
	}
	for _, pat := range excludes {
		if !scan.ValidGlob(pat) {
			fmt.Fprintf(stderr, "eolvet: invalid --exclude pattern %q\n", pat)
			return exitUsage
		}
	}
	asOf, err := parseAsOf(*asOfFlag)
	if err != nil {
		fmt.Fprintf(stderr, "eolvet: %v\n", err)
		return exitUsage
	}
	snap, err := loadSnapshot(*dataPath)
	if err != nil {
		fmt.Fprintf(stderr, "eolvet: %v\n", err)
		return exitRuntime
	}

	decls, err := scan.Walk(path, detect.New(snap), scan.Options{
		Excludes:    excludes,
		MaxFileSize: *maxFileSize,
	})
	if err != nil {
		fmt.Fprintf(stderr, "eolvet: %v\n", err)
		return exitRuntime
	}
	findings := report.Evaluate(decls, snap, asOf, *warnDays)
	report.SortBySeverity(findings)
	r := &report.Report{
		Path:         path,
		SnapshotDate: snap.SnapshotDate,
		AsOf:         asOf.Format("2006-01-02"),
		WarnDays:     *warnDays,
		Findings:     findings,
		Summary:      report.Summarize(findings),
	}

	switch *format {
	case "json":
		out, err := report.RenderJSON(r)
		if err != nil {
			fmt.Fprintf(stderr, "eolvet: %v\n", err)
			return exitRuntime
		}
		fmt.Fprint(stdout, out)
	case "markdown":
		fmt.Fprint(stdout, report.RenderMarkdown(r))
	default:
		fmt.Fprint(stdout, report.RenderText(r))
	}

	if breached(r.Summary, *failOn, *strict) {
		return exitBreach
	}
	return exitOK
}

// breached applies the policy gate: --fail-on picks the severity floor,
// --strict adds unknowns (a compliance stance: what you cannot date,
// you cannot pass).
func breached(s report.Summary, failOn string, strict bool) bool {
	if strict && s.Unknown > 0 {
		return true
	}
	switch failOn {
	case "eol":
		return s.EOL > 0
	case "eol-soon":
		return s.EOL > 0 || s.EOLSoon > 0
	}
	return false
}
