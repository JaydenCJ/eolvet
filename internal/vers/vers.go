// Package vers parses version strings and constraint expressions and
// matches concrete versions against release-cycle names.
//
// eolvet never needs full semver arithmetic: for an end-of-life audit the
// only question is "what is the oldest version this declaration allows?"
// (the floor), because that is the version whose support window bounds the
// repository's exposure. Everything here is pure and allocation-light.
package vers

import (
	"strings"
)

// Components splits a version string into its numeric components.
// Non-numeric suffixes are tolerated on the last parsed component
// ("1.20rc1" -> [1, 20]); a component with no leading digit ends the
// parse ("3.8.dev0" -> [3, 8]). Returns nil when nothing numeric leads
// the string ("latest", "stable", "*").
func Components(v string) []int {
	v = Normalize(v)
	if v == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(v, ".") {
		n, ok := leadingInt(part)
		if !ok {
			break
		}
		out = append(out, n)
		// A trailing suffix such as "10rc1" ends the version: "3.8rc1.2"
		// must not parse the ".2" as a third component.
		if !allDigits(part) {
			break
		}
	}
	return out
}

// Normalize trims whitespace, surrounding quotes, and a leading v/V
// ("v3.8.10" and `"18.x"` both normalize cleanly).
func Normalize(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`)
	if len(s) > 1 && (s[0] == 'v' || s[0] == 'V') && s[1] >= '0' && s[1] <= '9' {
		s = s[1:]
	}
	return s
}

// Compare orders two version strings by numeric components. Missing
// components compare as zero, so "18" == "18.0.0". Returns -1, 0, or +1.
func Compare(a, b string) int {
	ca, cb := Components(a), Components(b)
	n := len(ca)
	if len(cb) > n {
		n = len(cb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(ca) {
			va = ca[i]
		}
		if i < len(cb) {
			vb = cb[i]
		}
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Floor extracts the effective minimum version a constraint expression
// allows. It understands the operators found in real manifests — exact
// pins, =, >=, >, ^, ~, ~>, wildcard suffixes (18.x, 18.*) — with comma
// or space separated conjunctions and || alternatives. Upper bounds
// (<, <=) and exclusions (!=) never lower the floor and are skipped.
//
// The floor of a conjunction is its highest lower bound; the floor of a
// set of alternatives is the lowest alternative floor (any branch could
// be installed). Returns ok=false when no branch pins a lower bound
// ("*", "latest", "<19").
func Floor(constraint string) (string, bool) {
	best := ""
	for _, alt := range strings.Split(constraint, "||") {
		floor, ok := conjunctionFloor(alt)
		if !ok {
			return "", false // one unbounded branch unbounds the whole set
		}
		if best == "" || Compare(floor, best) < 0 {
			best = floor
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// MatchCycle resolves a concrete version onto a release-cycle name: the
// cycle's numeric components must be an exact prefix of the version's
// ("3.8" matches "3.8.10"; "18" matches "18.16.0"). The longest matching
// cycle wins, so MariaDB "10.11.4" lands on cycle "10.11" and never on
// "10.1". A version with fewer components than a cycle is ambiguous and
// does not match ("3" cannot pick between "3.8" and "3.9").
func MatchCycle(version string, cycles []string) (string, bool) {
	vc := Components(version)
	if len(vc) == 0 {
		return "", false
	}
	best, bestLen := "", 0
	for _, cycle := range cycles {
		cc := Components(cycle)
		if len(cc) == 0 || len(cc) > len(vc) {
			continue
		}
		match := true
		for i := range cc {
			if cc[i] != vc[i] {
				match = false
				break
			}
		}
		if match && len(cc) > bestLen {
			best, bestLen = cycle, len(cc)
		}
	}
	return best, bestLen > 0
}

// conjunctionFloor computes the highest lower bound among the comma- or
// whitespace-separated tokens of one constraint branch.
func conjunctionFloor(expr string) (string, bool) {
	fields := strings.FieldsFunc(expr, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	best := ""
	for i := 0; i < len(fields); i++ {
		tok := fields[i]
		// "~> 3.1" and ">= 3.9" may split the operator from its operand.
		if isBareOperator(tok) && i+1 < len(fields) {
			tok += fields[i+1]
			i++
		}
		lower, ok := tokenLowerBound(tok)
		if !ok {
			continue
		}
		if best == "" || Compare(lower, best) > 0 {
			best = lower
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// tokenLowerBound extracts the lower bound a single constraint token
// implies, if any.
func tokenLowerBound(tok string) (string, bool) {
	tok = strings.TrimSpace(tok)
	switch {
	case tok == "":
		return "", false
	case strings.HasPrefix(tok, "<") || strings.HasPrefix(tok, "!="):
		return "", false // pure upper bound / exclusion
	}
	for _, op := range []string{">=", "~>", ">", "^", "~", "==", "="} {
		if strings.HasPrefix(tok, op) {
			tok = strings.TrimSpace(strings.TrimPrefix(tok, op))
			break
		}
	}
	tok = strings.TrimSuffix(tok, ".x")
	tok = strings.TrimSuffix(tok, ".X")
	tok = strings.TrimSuffix(tok, ".*")
	tok = strings.TrimRight(tok, ".") // sloppy trailing dot: "3.9."
	if len(Components(tok)) == 0 {
		return "", false
	}
	return Normalize(tok), true
}

func isBareOperator(tok string) bool {
	switch tok {
	case ">=", ">", "<", "<=", "=", "==", "^", "~", "~>", "!=":
		return true
	}
	return false
}

func leadingInt(s string) (int, bool) {
	n, seen := 0, false
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		seen = true
		n = n*10 + int(r-'0')
		if n > 1<<30 {
			return 0, false // absurd component; treat as non-numeric
		}
	}
	return n, seen
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
