// Tests for version parsing, constraint floors, and cycle matching —
// the arithmetic every EOL judgment rests on.
package vers

import (
	"reflect"
	"testing"
)

func TestComponentsParsesVersionsAndRejectsNonVersions(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []int
	}{
		{"3.8.10", []int{3, 8, 10}},
		{"18", []int{18}},
		{"22.04", []int{22, 4}},
		{"2023", []int{2023}},
		{"v18.16.0", []int{18, 16, 0}},
		// "3.8rc1.2" must not read the trailing ".2" as a patch
		// component: the rc suffix ends the version.
		{"3.8rc1.2", []int{3, 8}},
		{"1.20rc1", []int{1, 20}},
		// Nothing numeric leads these — they are not versions at all.
		{"latest", nil},
		{"*", nil},
		{"", nil},
		{"lts/hydrogen", nil},
		{"bullseye", nil},
	} {
		if got := Components(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Components(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeTrimsQuotesAndV(t *testing.T) {
	for in, want := range map[string]string{
		`"3.9"`:    "3.9",
		"'18'":     "18",
		" v1.2.3 ": "1.2.3",
		"V2.0":     "2.0",
		"vendor":   "vendor", // a lone leading v before a letter is not a version prefix
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompareOrdersNumerically(t *testing.T) {
	// String comparison would put 3.9 above 3.10 — numeric must not.
	if Compare("3.9", "3.10") != -1 {
		t.Fatal("3.9 should sort below 3.10")
	}
	if Compare("18", "18.0.0") != 0 {
		t.Fatal("18 should equal 18.0.0 (missing components are zero)")
	}
	if Compare("2.7", "2.6.9") != 1 {
		t.Fatal("2.7 should sort above 2.6.9")
	}
}

func TestFloorOfPinsOperatorsAndWildcards(t *testing.T) {
	for in, want := range map[string]string{
		"3.8.10":   "3.8.10",
		">=3.9":    "3.9",
		">= 3.9":   "3.9",
		"^18.2.0":  "18.2.0",
		"~3.9.1":   "3.9.1",
		"~> 3.1":   "3.1",
		"=1.22":    "1.22",
		"==3.11.4": "3.11.4",
		">16":      "16",
		"18.x":     "18",
		"18.*":     "18",
		"3.9.":     "3.9", // sloppy trailing dot still parses
	} {
		got, ok := Floor(in)
		if !ok || got != want {
			t.Errorf("Floor(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestFloorOfConjunctionTakesHighestLowerBound(t *testing.T) {
	// ">=3.9,<3.13" allows nothing below 3.9; the upper bound is
	// irrelevant to EOL exposure.
	got, ok := Floor(">=3.9,<3.13")
	if !ok || got != "3.9" {
		t.Fatalf("Floor(>=3.9,<3.13) = %q, %v", got, ok)
	}
	// Two lower bounds intersect at the higher one.
	got, ok = Floor(">=14 >=18")
	if !ok || got != "18" {
		t.Fatalf("Floor(>=14 >=18) = %q, %v", got, ok)
	}
}

func TestFloorOfAlternativesTakesLowest(t *testing.T) {
	// "^18.17 || ^20.3" may install either branch, so the audit must
	// assume the older one.
	got, ok := Floor("^18.17.0 || ^20.3.0")
	if !ok || got != "18.17.0" {
		t.Fatalf("Floor(alternatives) = %q, %v", got, ok)
	}
}

func TestFloorUnboundedConstraints(t *testing.T) {
	// A constraint that never pins a lower bound cannot be judged; a
	// silent guess would produce a false EOL verdict.
	for _, in := range []string{"*", "latest", "<19", "!=3.8", ""} {
		if got, ok := Floor(in); ok {
			t.Errorf("Floor(%q) = %q, want unbounded", in, got)
		}
	}
	// One unbounded alternative unbounds the whole set: the * branch
	// allows anything.
	if got, ok := Floor(">=18 || *"); ok {
		t.Errorf("Floor(>=18 || *) = %q, want unbounded", got)
	}
}

func TestMatchCyclePicksTheMostSpecificPrefix(t *testing.T) {
	// Plain prefix match.
	got, ok := MatchCycle("3.8.10", []string{"3.8", "3.9", "3.10"})
	if !ok || got != "3.8" {
		t.Fatalf("MatchCycle(3.8.10) = %q, %v", got, ok)
	}
	// Components compare numerically: 3.10.2 lands on 3.10, never 3.1.
	got, ok = MatchCycle("3.10.2", []string{"3.1", "3.8", "3.10"})
	if !ok || got != "3.10" {
		t.Fatalf("MatchCycle(3.10.2) = %q, %v", got, ok)
	}
	// MariaDB-style tables mix specificity; the longest match wins.
	got, ok = MatchCycle("10.11.4", []string{"10.5", "10.6", "10.11"})
	if !ok || got != "10.11" {
		t.Fatalf("MatchCycle(10.11.4) = %q, %v", got, ok)
	}
	// Node-style single-component cycles match on the major.
	got, ok = MatchCycle("18.16.0", []string{"16", "18", "20"})
	if !ok || got != "18" {
		t.Fatalf("MatchCycle(18.16.0) = %q, %v", got, ok)
	}
}

func TestMatchCycleRefusesAmbiguityAndNonVersions(t *testing.T) {
	// Bare "3" cannot choose between 3.8 and 3.9 — guessing either way
	// could flip an EOL verdict.
	if got, ok := MatchCycle("3", []string{"3.8", "3.9"}); ok {
		t.Fatalf("MatchCycle(3) = %q, want no match", got)
	}
	if _, ok := MatchCycle("latest", []string{"18"}); ok {
		t.Fatal("MatchCycle(latest) should not match")
	}
	if _, ok := MatchCycle("18.1", nil); ok {
		t.Fatal("MatchCycle with no cycles should not match")
	}
}
