// Package similarity — hybrid semantic near-duplicate detection.
//
// Combines three signals into a single score in [0, 1]:
//   - token Jaccard   (weight 0.6): set similarity of normalized word tokens
//   - Levenshtein     (weight 0.2): edit distance / max(len)
//   - repo overlap    (weight 0.2): 1 if same repo, 0 otherwise
//
// Score thresholds:
//   >= 0.75 — duplicates (skip one)
//   0.40-0.74 — near-duplicates (review)
//   < 0.40  — distinct
package similarity

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"unicode"
)

// Hash returns the first 12 hex chars of SHA1(s) (lowercased).
// Used for stable task IDs derived from natural-language descriptions.
func Hash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// tokenize lowercases, splits on non-alphanumerics, drops stopwords
// and very short tokens. Returns a sorted, deduped set.
var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true,
	"at": true, "be": true, "by": true, "for": true, "from": true,
	"has": true, "have": true, "in": true, "is": true, "it": true,
	"of": true, "on": true, "or": true, "that": true, "the": true,
	"this": true, "to": true, "was": true, "were": true, "will": true,
	"with": true, "we": true, "you": true, "your": true, "i": true,
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) < 3 || stopwords[w] {
			continue
		}
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// jaccard returns |A ∩ B| / |A ∪ B| over the token sets.
func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	setA := map[string]bool{}
	for _, t := range a {
		setA[t] = true
	}
	setB := map[string]bool{}
	for _, t := range b {
		setB[t] = true
	}
	intersect := 0
	for t := range setA {
		if setB[t] {
			intersect++
		}
	}
	union := len(setA) + len(setB) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

// levenshtein returns 1 - editDistance/max(len(a), len(b)).
// 1.0 = identical, 0.0 = completely different.
func levenshtein(a, b string) float64 {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1.0
	}
	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	if maxLen == 0 {
		return 1.0
	}
	// Standard DP
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := 0; j <= len(rb); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			min3 := prev[j] + 1
			if curr[j-1]+1 < min3 {
				min3 = curr[j-1] + 1
			}
			if prev[j-1]+cost < min3 {
				min3 = prev[j-1] + cost
			}
			curr[j] = min3
		}
		prev, curr = curr, prev
	}
	dist := prev[len(rb)]
	return 1.0 - float64(dist)/float64(maxLen)
}

// Score returns the hybrid similarity of two task-like records.
//   descA, descB: natural-language descriptions
//   repoA, repoB: repo identifiers ("" if unknown)
func Score(descA, repoA, descB, repoB string) float64 {
	tokA := tokenize(descA)
	tokB := tokenize(descB)
	j := jaccard(tokA, tokB)

	// Levenshtein on the joined tokens (less noisy than char-level on raw).
	joinedA := strings.Join(tokA, " ")
	joinedB := strings.Join(tokB, " ")
	lev := levenshtein(joinedA, joinedB)

	repoScore := 0.0
	if repoA != "" && repoA == repoB {
		repoScore = 1.0
	}

	return 0.6*j + 0.2*lev + 0.2*repoScore
}

// Hybrid is an alias for Score, kept for cmd layer compatibility.
func Hybrid(descA, repoA, descB, repoB string) float64 {
	return Score(descA, repoA, descB, repoB)
}
