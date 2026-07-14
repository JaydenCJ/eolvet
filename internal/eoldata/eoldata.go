// Package eoldata loads and queries the versioned end-of-life snapshot.
//
// The snapshot is a dated, curated table of release cycles per product,
// embedded into the binary at build time so a scan needs no network and
// gives the same answer on every machine. Organizations can substitute
// their own table (same schema) via Snapshot loading from a file.
package eoldata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/JaydenCJ/eolvet/internal/vers"
)

//go:embed snapshot.json
var embedded []byte

// Cycle is one release line of a product with its lifecycle dates.
type Cycle struct {
	Cycle   string `json:"cycle"`
	Release string `json:"release"`
	EOL     string `json:"eol"`
}

// Product is a named runtime, distro, or server with its release cycles.
type Product struct {
	Label     string            `json:"label"`
	Codenames map[string]string `json:"codenames,omitempty"`
	Cycles    []Cycle           `json:"cycles"`
}

// Snapshot is the full EOL table plus its provenance metadata.
type Snapshot struct {
	SchemaVersion int                `json:"schema_version"`
	SnapshotDate  string             `json:"snapshot_date"`
	Source        string             `json:"source"`
	Products      map[string]Product `json:"products"`
}

// Load parses and validates the snapshot bundled into the binary.
// The embedded table is validated by tests, so failure here means a
// corrupted build and is worth a loud error.
func Load() (*Snapshot, error) {
	return parse(embedded)
}

// LoadFile parses and validates a user-supplied snapshot (the --data
// flag), letting organizations pin their own lifecycle policy.
func LoadFile(path string) (*Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	s, err := parse(raw)
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", path, err)
	}
	return s, nil
}

// Product looks up a product by its snapshot key.
func (s *Snapshot) Product(name string) (Product, bool) {
	p, ok := s.Products[name]
	return p, ok
}

// ProductNames returns all product keys in sorted order, for stable
// listings and deterministic reports.
func (s *Snapshot) ProductNames() []string {
	names := make([]string, 0, len(s.Products))
	for name := range s.Products {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CycleCount reports the total number of cycles across all products.
func (s *Snapshot) CycleCount() int {
	n := 0
	for _, p := range s.Products {
		n += len(p.Cycles)
	}
	return n
}

// Resolve maps a concrete version string onto one of the product's
// cycles ("3.8.10" -> "3.8"). Codenames resolve first ("bullseye" -> 11),
// so distro tags work whether numeric or named.
func (p Product) Resolve(version string) (Cycle, bool) {
	if p.Codenames != nil {
		if cycle, ok := p.Codenames[normalizeCodename(version)]; ok {
			version = cycle
		}
	}
	names := make([]string, len(p.Cycles))
	for i, c := range p.Cycles {
		names[i] = c.Cycle
	}
	name, ok := vers.MatchCycle(version, names)
	if !ok {
		return Cycle{}, false
	}
	for _, c := range p.Cycles {
		if c.Cycle == name {
			return c, true
		}
	}
	return Cycle{}, false
}

// IsCodename reports whether the token is a known codename for this
// product ("jammy", "bookworm"). Detectors use it to split base-image
// suffixes like "3.9-slim-bullseye" into a second finding.
func (p Product) IsCodename(token string) bool {
	if p.Codenames == nil {
		return false
	}
	_, ok := p.Codenames[normalizeCodename(token)]
	return ok
}

// ParseDate parses a snapshot date (YYYY-MM-DD) as midnight UTC.
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// DaysUntil returns the whole days from `from` to `to`; negative when
// `to` is in the past. Both are date-granular, so no DST surprises.
func DaysUntil(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}

func normalizeCodename(s string) string {
	// Codenames are stored lowercase; tags occasionally carry point
	// releases ("bullseye-20230522") — the date suffix is stripped by
	// the detector, not here, to keep this a pure map lookup.
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// parse decodes and validates a snapshot document. Every date must
// parse, every cycle name must be numeric and unique per product, and
// every codename must point at a declared cycle — a bad table should
// fail loudly at load time, not silently misclassify at scan time.
func parse(raw []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}
	if s.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported snapshot schema_version %d (want 1)", s.SchemaVersion)
	}
	if _, err := ParseDate(s.SnapshotDate); err != nil {
		return nil, fmt.Errorf("invalid snapshot_date %q", s.SnapshotDate)
	}
	if len(s.Products) == 0 {
		return nil, fmt.Errorf("snapshot has no products")
	}
	for name, p := range s.Products {
		if p.Label == "" {
			return nil, fmt.Errorf("product %s: missing label", name)
		}
		if len(p.Cycles) == 0 {
			return nil, fmt.Errorf("product %s: no cycles", name)
		}
		seen := map[string]bool{}
		for _, c := range p.Cycles {
			if len(vers.Components(c.Cycle)) == 0 {
				return nil, fmt.Errorf("product %s: non-numeric cycle %q", name, c.Cycle)
			}
			if seen[c.Cycle] {
				return nil, fmt.Errorf("product %s: duplicate cycle %q", name, c.Cycle)
			}
			seen[c.Cycle] = true
			for field, date := range map[string]string{"release": c.Release, "eol": c.EOL} {
				if _, err := ParseDate(date); err != nil {
					return nil, fmt.Errorf("product %s cycle %s: invalid %s date %q", name, c.Cycle, field, date)
				}
			}
		}
		for code, cycle := range p.Codenames {
			if !seen[cycle] {
				return nil, fmt.Errorf("product %s: codename %q points at unknown cycle %q", name, code, cycle)
			}
		}
	}
	return &s, nil
}
