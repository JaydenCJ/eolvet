// Package report judges declarations against the EOL snapshot and
// renders the result as text, JSON, or Markdown.
package report

import (
	"time"

	"github.com/JaydenCJ/eolvet/internal/detect"
	"github.com/JaydenCJ/eolvet/internal/eoldata"
)

// Status classifies one finding relative to the as-of date.
type Status string

const (
	StatusEOL     Status = "eol"      // past its end-of-life date
	StatusEOLSoon Status = "eol-soon" // EOL within the warn window
	StatusOK      Status = "supported"
	StatusUnknown Status = "unknown" // version or cycle unresolvable
)

// Finding is one judged declaration — everything a reviewer needs to
// act: what, where, how it was declared, and when support ends.
type Finding struct {
	Status   Status `json:"status"`
	Product  string `json:"product"`
	Label    string `json:"label"`
	Cycle    string `json:"cycle,omitempty"`
	Version  string `json:"version,omitempty"`
	EOLDate  string `json:"eol_date,omitempty"`
	DaysLeft int    `json:"days_left"` // negative when past EOL; 0 for unknown
	File     string `json:"file"`
	Line     int    `json:"line"`
	Source   string `json:"source"`
	Raw      string `json:"declared"`
	Note     string `json:"note,omitempty"`
}

// Summary counts findings by status.
type Summary struct {
	Total     int `json:"total"`
	EOL       int `json:"eol"`
	EOLSoon   int `json:"eol_soon"`
	Supported int `json:"supported"`
	Unknown   int `json:"unknown"`
}

// Report is the full result of one scan.
type Report struct {
	Path         string    `json:"path"`
	SnapshotDate string    `json:"snapshot_date"`
	AsOf         string    `json:"as_of"`
	WarnDays     int       `json:"warn_within_days"`
	Findings     []Finding `json:"findings"`
	Summary      Summary   `json:"summary"`
}

// Evaluate judges every declaration against the snapshot. asOf is date-
// granular; warnDays is the eol-soon window. The declaration order is
// preserved, so reports stay deterministic.
func Evaluate(decls []detect.Decl, snap *eoldata.Snapshot, asOf time.Time, warnDays int) []Finding {
	findings := make([]Finding, 0, len(decls))
	for _, d := range decls {
		findings = append(findings, judge(d, snap, asOf, warnDays))
	}
	return findings
}

// Summarize tallies findings by status.
func Summarize(findings []Finding) Summary {
	s := Summary{Total: len(findings)}
	for _, f := range findings {
		switch f.Status {
		case StatusEOL:
			s.EOL++
		case StatusEOLSoon:
			s.EOLSoon++
		case StatusOK:
			s.Supported++
		default:
			s.Unknown++
		}
	}
	return s
}

func judge(d detect.Decl, snap *eoldata.Snapshot, asOf time.Time, warnDays int) Finding {
	f := Finding{
		Status:  StatusUnknown,
		Product: d.Product,
		Label:   d.Product,
		Version: d.Version,
		File:    d.File,
		Line:    d.Line,
		Source:  d.Source,
		Raw:     d.Raw,
		Note:    d.Note,
	}
	product, ok := snap.Product(d.Product)
	if !ok {
		f.Note = joinNote(f.Note, "product not in snapshot")
		return f
	}
	f.Label = product.Label
	if d.Version == "" {
		return f // detector already explained why in Note
	}
	cycle, ok := product.Resolve(d.Version)
	if !ok {
		f.Note = joinNote(f.Note, "no matching release cycle in snapshot")
		return f
	}
	f.Cycle = cycle.Cycle
	f.EOLDate = cycle.EOL
	eol, err := eoldata.ParseDate(cycle.EOL)
	if err != nil {
		return f // validated at load; unreachable in practice
	}
	f.DaysLeft = eoldata.DaysUntil(asOf, eol)
	switch {
	case f.DaysLeft <= 0:
		f.Status = StatusEOL
	case f.DaysLeft <= warnDays:
		f.Status = StatusEOLSoon
	default:
		f.Status = StatusOK
	}
	return f
}

func joinNote(existing, extra string) string {
	if existing == "" {
		return extra
	}
	return existing + "; " + extra
}
