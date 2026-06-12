package similarity

import (
	"math"
	"testing"
)

func TestHash(t *testing.T) {
	h1 := Hash("task-a")
	h2 := Hash("task-a")
	if h1 != h2 {
		t.Errorf("Hash not stable: %s != %s", h1, h2)
	}
	if len(h1) != 12 {
		t.Errorf("Hash length should be 12, got %d (%q)", len(h1), h1)
	}
}

func TestTokenize(t *testing.T) {
	toks := tokenize("The quick brown fox jumps over the lazy dog!")
	if len(toks) == 0 {
		t.Fatal("tokenize returned no tokens")
	}
	// "the" is a stopword, should be excluded
	for _, tok := range toks {
		if tok == "the" {
			t.Errorf("stopword 'the' was not removed: %v", toks)
		}
	}
}

func TestJaccard(t *testing.T) {
	if math.Abs(jaccard([]string{"a", "b", "c"}, []string{"a", "b", "c"})) > 1e-9 {
		// jaccard returns float64, no error path
	}
	identical := jaccard([]string{"a", "b", "c"}, []string{"a", "b", "c"})
	if math.Abs(identical-1.0) > 1e-9 {
		t.Errorf("identical jaccard: got %f, want 1.0", identical)
	}
	disjoint := jaccard([]string{"a", "b"}, []string{"c", "d"})
	if disjoint != 0.0 {
		t.Errorf("disjoint jaccard: got %f, want 0.0", disjoint)
	}
}

func TestLevenshtein(t *testing.T) {
	if math.Abs(levenshtein("kitten", "kitten")-1.0) > 1e-9 {
		t.Errorf("identical: got %f, want 1.0", levenshtein("kitten", "kitten"))
	}
	// empty vs empty
	if levenshtein("", "") != 1.0 {
		t.Errorf("empty/empty: got %f, want 1.0", levenshtein("", ""))
	}
}

func TestScore(t *testing.T) {
	// Identical descriptions
	s := Score("add audit for HexaKit", "HexaKit", "add audit for HexaKit", "HexaKit")
	if s < 0.9 {
		t.Errorf("identical: got %f, want >= 0.9", s)
	}
	// Different repos, different descriptions
	s2 := Score("add audit for HexaKit", "HexaKit", "scan network for vulnerabilities", "NetScan")
	if s2 > 0.3 {
		t.Errorf("very different: got %f, want <= 0.3", s2)
	}
	// Same repo, different descriptions
	s3 := Score("add audit for HexaKit", "HexaKit", "fix login bug in HexaKit", "HexaKit")
	if s3 < 0.1 || s3 > 0.6 {
		t.Errorf("same repo diff desc: got %f, want 0.1-0.6", s3)
	}
}
