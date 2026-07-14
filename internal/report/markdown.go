// Markdown renderer: a PR-comment-ready table.
package report

import (
	"fmt"
	"strings"
)

// RenderMarkdown renders the report as a Markdown document suitable for
// pasting into a pull request or an audit ticket.
func RenderMarkdown(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## eolvet report — %s\n\n", r.Path)
	window := fmt.Sprintf("%d days", r.WarnDays)
	if r.WarnDays == 1 {
		window = "1 day"
	}
	fmt.Fprintf(&b, "Snapshot %s · as of %s · warn window %s\n\n", r.SnapshotDate, r.AsOf, window)
	if len(r.Findings) == 0 {
		b.WriteString("No version declarations found.\n")
		return b.String()
	}
	b.WriteString("| Status | Product | Cycle | EOL | Days | Where | Declared |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, f := range r.Findings {
		status := statusText(f.Status)
		if f.Status == StatusEOL || f.Status == StatusEOLSoon {
			status = "**" + status + "**"
		}
		days := "—"
		if f.EOLDate != "" {
			days = fmt.Sprintf("%+d", f.DaysLeft)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | `%s:%d` | %s |\n",
			status, f.Label, dash(f.Cycle), dash(f.EOLDate), days,
			f.File, f.Line, mdDeclared(f))
	}
	fmt.Fprintf(&b, "\n**%s**\n", summaryLine(r.Summary))
	return b.String()
}

func mdDeclared(f Finding) string {
	out := "`" + escapePipes(f.Raw) + "`"
	if f.Note != "" {
		out += " — " + escapePipes(f.Note)
	}
	return out
}

// escapePipes keeps declarations containing | from breaking table cells.
func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
