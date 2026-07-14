// Plain-text renderer: aligned columns, severity first, one summary line.
package report

import (
	"fmt"
	"sort"
	"strings"
)

// severityRank orders statuses worst-first for human-facing output —
// the auditor's next action is always at the top.
var severityRank = map[Status]int{
	StatusEOL:     0,
	StatusEOLSoon: 1,
	StatusUnknown: 2,
	StatusOK:      3,
}

// SortBySeverity orders findings worst-first, then by file and line, so
// every renderer presents the same stable order.
func SortBySeverity(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if severityRank[findings[i].Status] != severityRank[findings[j].Status] {
			return severityRank[findings[i].Status] < severityRank[findings[j].Status]
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
}

// RenderText renders the report as an aligned table for terminals.
func RenderText(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "eolvet scan — %s (snapshot %s, as of %s)\n\n", r.Path, r.SnapshotDate, r.AsOf)
	if len(r.Findings) == 0 {
		b.WriteString("no version declarations found\n")
		return b.String()
	}
	rows := make([][]string, 0, len(r.Findings)+1)
	rows = append(rows, []string{"STATUS", "PRODUCT", "CYCLE", "EOL", "DAYS", "WHERE", "DECLARED"})
	for _, f := range r.Findings {
		rows = append(rows, []string{
			statusText(f.Status),
			f.Label,
			dash(f.Cycle),
			dash(f.EOLDate),
			daysText(f),
			fmt.Sprintf("%s:%d", f.File, f.Line),
			declaredText(f),
		})
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if w := len([]rune(cell)); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for _, row := range rows {
		for i, cell := range row {
			if i == len(row)-1 {
				b.WriteString(cell)
				continue
			}
			fmt.Fprintf(&b, "%-*s  ", widths[i], cell)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n%s\n", summaryLine(r.Summary))
	return b.String()
}

func statusText(s Status) string {
	switch s {
	case StatusEOL:
		return "EOL"
	case StatusEOLSoon:
		return "EOL-SOON"
	case StatusOK:
		return "OK"
	default:
		return "UNKNOWN"
	}
}

func daysText(f Finding) string {
	if f.EOLDate == "" {
		return "—"
	}
	return fmt.Sprintf("%+dd", f.DaysLeft)
}

func declaredText(f Finding) string {
	if f.Note != "" {
		return fmt.Sprintf("%s  (%s)", f.Raw, f.Note)
	}
	return f.Raw
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func summaryLine(s Summary) string {
	noun := "declarations"
	if s.Total == 1 {
		noun = "declaration"
	}
	return fmt.Sprintf("%d %s: %d eol, %d eol-soon, %d supported, %d unknown",
		s.Total, noun, s.EOL, s.EOLSoon, s.Supported, s.Unknown)
}
