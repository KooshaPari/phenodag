package main

// phenodag_test.go — focused unit tests (no DB file required).
//
// Run with:   go test -mod=mod ./...

import (
	"strings"
	"testing"
)

func TestTokens(t *testing.T) {
	parts := tokens("Hello, WORLD!  hello-world foo_bar 123")
	joined := strings.Join(parts, " ")
	for _, want := range []string{"hello", "world", "foo", "bar", "123"} {
		if !strings.Contains(joined, want) {
			t.Errorf("tokens missing %q in %q", want, joined)
		}
	}
}

func TestJaccardIdentical(t *testing.T) {
	a := []string{"l1", "audit", "hexa", "kit", "slot", "1"}
	b := []string{"l1", "audit", "hexa", "kit", "slot", "1"}
	if got := jaccard(a, b); got != 1.0 {
		t.Errorf("jaccard(a,a) = %v, want 1.0", got)
	}
}

func TestJaccardDisjoint(t *testing.T) {
	a := []string{"hello", "world"}
	b := []string{"foo", "bar"}
	if got := jaccard(a, b); got != 0.0 {
		t.Errorf("jaccard(disjoint) = %v, want 0.0", got)
	}
}

func TestJaccardPartial(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"b", "c", "d"}
	// |A∩B|=2, |A∪B|=4 -> 0.5
	if got := jaccard(a, b); got != 0.5 {
		t.Errorf("jaccard(a,b) = %v, want 0.5", got)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"abc", "abc", 0},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestHybridScoreIdentical(t *testing.T) {
	desc := "L1 audit for HexaKit slot 1: repo state, branches, worktrees, claims"
	repo := "HexaKit"
	got := hybridScore(desc, desc, repo, repo)
	if got < 0.99 {
		t.Errorf("hybridScore(identical) = %v, want >= 0.99", got)
	}
}

func TestHybridScoreRepoOverlap(t *testing.T) {
	descA := "L1 audit for HexaKit slot 1: repo state, branches"
	descB := "L1 audit for HexaKit slot 2: repo state, branches"
	got := hybridScore(descA, descB, "HexaKit", "HexaKit")
	if got < 0.7 {
		t.Errorf("hybridScore(repo-overlap) = %v, want >= 0.7", got)
	}
}

func TestHybridScoreDissimilar(t *testing.T) {
	descA := "L1 audit for HexaKit slot 1: repo state, branches"
	descB := "deploy canary 5% traffic with SLO guards and rollback"
	got := hybridScore(descA, descB, "HexaKit", "agileplus")
	if got > 0.3 {
		t.Errorf("hybridScore(dissimilar) = %v, want <= 0.3", got)
	}
}

func TestHashIDStable(t *testing.T) {
	a := hashID(1, 5, "task-01-05")
	b := hashID(1, 5, "task-01-05")
	if a != b {
		t.Errorf("hashID should be stable: %q != %q", a, b)
	}
	if len(a) != 16 {
		t.Errorf("hashID length = %d, want 16", len(a))
	}
}

func TestV3CoreHas120(t *testing.T) {
	tasks := v3Core()
	if got := len(tasks); got != 120 {
		t.Errorf("v3Core() = %d tasks, want 120", got)
	}
}

func TestV3SideHas60(t *testing.T) {
	tasks := v3Side()
	if got := len(tasks); got != 60 {
		t.Errorf("v3Side() = %d tasks, want 60", got)
	}
}
