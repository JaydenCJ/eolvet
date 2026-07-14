// Tests for the --exclude glob matcher.
package scan

import (
	"testing"
)

func TestMatchGlobPlainSegments(t *testing.T) {
	if !MatchGlob("Dockerfile", "Dockerfile") {
		t.Fatal("exact name should match")
	}
	if MatchGlob("Dockerfile", "sub/Dockerfile") {
		t.Fatal("single segment must not span directories")
	}
}

func TestMatchGlobStarWithinSegment(t *testing.T) {
	if !MatchGlob("*.dockerfile", "api.dockerfile") {
		t.Fatal("* should match within a segment")
	}
	if MatchGlob("*.dockerfile", "sub/api.dockerfile") {
		t.Fatal("* must not cross a /")
	}
}

func TestMatchGlobDoubleStarSpansSegments(t *testing.T) {
	for pattern, rel := range map[string]string{
		"legacy/**":       "legacy/deep/Dockerfile",
		"**/Dockerfile":   "a/b/c/Dockerfile",
		"**/.nvmrc":       ".nvmrc", // ** also matches zero segments
		"a/**/Dockerfile": "a/Dockerfile",
	} {
		if !MatchGlob(pattern, rel) {
			t.Errorf("MatchGlob(%q, %q) = false", pattern, rel)
		}
	}
	if MatchGlob("legacy/**", "modern/Dockerfile") {
		t.Fatal("wrong prefix should not match")
	}
	if MatchGlob("**/go.mod", "go.sum") {
		t.Fatal("different basename should not match")
	}
}

func TestValidGlob(t *testing.T) {
	for _, ok := range []string{"vendor/**", "*.dockerfile", "**/Dockerfile", "a/b"} {
		if !ValidGlob(ok) {
			t.Errorf("ValidGlob(%q) = false", ok)
		}
	}
	if ValidGlob("[oops") {
		t.Fatal("unterminated class should be invalid")
	}
}
