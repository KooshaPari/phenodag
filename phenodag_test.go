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

func TestMelosvizCoreHas140(t *testing.T) {
	tasks := melosvizCore()
	if got := len(tasks); got != 140 {
		t.Errorf("melosvizCore() = %d tasks, want 140 (7 stages x 20 width)", got)
	}
}

func TestMelosvizSideHas45(t *testing.T) {
	tasks := melosvizSide()
	if got := len(tasks); got != 45 {
		t.Errorf("melosvizSide() = %d tasks, want 45 (9 side-DAGs x 5)", got)
	}
}

func TestAgileplusCoreHas20(t *testing.T) {
	tasks := agileplusCore()
	if got := len(tasks); got != 20 {
		t.Errorf("agileplusCore() = %d tasks, want 20 (4 stages x 5 width)", got)
	}
	// Sanity check: 4 unique stages (L1..L4) and 5 unique slots (1..5).
	stagesSeen := map[int]bool{}
	slotsSeen := map[int]bool{}
	for _, x := range tasks {
		stagesSeen[x.Stage] = true
		slotsSeen[x.Slot] = true
	}
	if len(stagesSeen) != 4 {
		t.Errorf("agileplusCore() stages = %d, want 4 unique", len(stagesSeen))
	}
	if len(slotsSeen) != 5 {
		t.Errorf("agileplusCore() slots = %d, want 5 unique", len(slotsSeen))
	}
}

func TestAgileplusSideHas30(t *testing.T) {
	tasks := agileplusSide()
	if got := len(tasks); got != 30 {
		t.Errorf("agileplusSide() = %d tasks, want 30 (6 side-DAGs x 5)", got)
	}
	// 6 distinct side-DAGs expected.
	seen := map[string]bool{}
	for _, x := range tasks {
		seen[x.SideDAG] = true
	}
	if len(seen) != 6 {
		t.Errorf("agileplusSide() distinct side-DAGs = %d, want 6", len(seen))
	}
}

func TestTraceraCoreHas20(t *testing.T) {
	tasks := traceraCore()
	if got := len(tasks); got != 20 {
		t.Errorf("traceraCore() = %d tasks, want 20 (4 stages x 5 width)", got)
	}
	stagesSeen := map[int]bool{}
	slotsSeen := map[int]bool{}
	for _, x := range tasks {
		stagesSeen[x.Stage] = true
		slotsSeen[x.Slot] = true
	}
	if len(stagesSeen) != 4 {
		t.Errorf("traceraCore() stages = %d, want 4 unique", len(stagesSeen))
	}
	if len(slotsSeen) != 5 {
		t.Errorf("traceraCore() slots = %d, want 5 unique", len(slotsSeen))
	}
}

func TestTraceraSideHas30(t *testing.T) {
	tasks := traceraSide()
	if got := len(tasks); got != 30 {
		t.Errorf("traceraSide() = %d tasks, want 30 (6 side-DAGs x 5)", got)
	}
	seen := map[string]bool{}
	for _, x := range tasks {
		seen[x.SideDAG] = true
	}
	if len(seen) != 6 {
		t.Errorf("traceraSide() distinct side-DAGs = %d, want 6", len(seen))
	}
}

// --- v3 superset-merge tests ---

func TestV3PortCoreHas120(t *testing.T) {
	tasks := v3PortBuildCore()
	if got := len(tasks); got != 120 {
		t.Errorf("v3PortBuildCore() = %d tasks, want 120 (6 stages x 4 subprojects x 5 slots)", got)
	}
}

func TestV3PortL7Has20(t *testing.T) {
	tasks := v3PortL7Sustain2
	if got := len(tasks); got != 20 {
		t.Errorf("v3PortL7Sustain2 = %d tasks, want 20 (4 subprojects x 5 slots)", got)
	}
}

func TestV3PortSideHas60(t *testing.T) {
	tasks := v3PortBuildSide()
	if got := len(tasks); got != 60 {
		t.Errorf("v3PortBuildSide() = %d tasks, want 60 (12 side-DAGs x 5 slots)", got)
	}
}

func TestV3PortExtend2Has35(t *testing.T) {
	l8 := len(v3PortL8)
	side := 0
	for _, v := range v3PortSideExtend2 {
		side += len(v)
	}
	total := l8 + side
	if l8 != 20 {
		t.Errorf("v3PortL8 = %d tasks, want 20", l8)
	}
	if total != 35 {
		t.Errorf("extend2 total = %d (L8=%d + side=%d), want 35", total, l8, side)
	}
}

func TestV3PortExtend3Has60(t *testing.T) {
	l9 := len(v3PortL9)
	l10 := len(v3PortL10)
	side := 0
	for _, v := range v3PortSideExtend3 {
		side += len(v)
	}
	total := l9 + l10 + side
	if l9 != 20 {
		t.Errorf("v3PortL9 = %d tasks, want 20", l9)
	}
	if l10 != 20 {
		t.Errorf("v3PortL10 = %d tasks, want 20", l10)
	}
	if total != 60 {
		t.Errorf("extend3 total = %d (L9=%d + L10=%d + side=%d), want 60", total, l9, l10, side)
	}
}

func TestSimHashStable(t *testing.T) {
	a := simhash64Port("hello world")
	b := simhash64Port("hello world")
	if a != b {
		t.Errorf("simhash64Port not stable: %x vs %x", a, b)
	}
	c := simhash64Port("totally different text")
	if a == c {
		t.Errorf("simhash64Port collides on distinct inputs: %x", a)
	}
}

func TestHammingDistance(t *testing.T) {
	if got := hammingDistancePort(0, 0); got != 0 {
		t.Errorf("hamming(0,0) = %d, want 0", got)
	}
	if got := hammingDistancePort(0xFFFFFFFFFFFFFFFF, 0); got != 64 {
		t.Errorf("hamming(all,0) = %d, want 64", got)
	}
	if got := hammingDistancePort(0xAAAAAAAAAAAAAAAA, 0xAAAAAAAAAAAAAAAA); got != 0 {
		t.Errorf("hamming(self,self) = %d, want 0", got)
	}
}
