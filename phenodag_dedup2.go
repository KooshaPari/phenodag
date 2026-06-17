// phenodag_dedup2.go — simhash + n-gram + cmdDedupExplain (port).
//
// Ported from C:/Users/koosh/Dev/dagctl/dagctl_dedup2.go on 2026-06-16
// for the superset merge. Complements phenodag's existing Jaccard +
// Levenshtein + repo-overlap hybrid (phenodag.go:1391) with a second
// dedup strategy: simhash bit-hamming + n-gram Jaccard, exposed via
// `phenodag dedup-explain`.
//
// All code is package main so the existing cmdDupes (Jaccard+Lev) stays
// the default `phenodag dupes` command.
package main

import (
	"fmt"
	"math/bits"
	"regexp"
	"strings"
)

var dedupPortWordSplit = regexp.MustCompile(`[^A-Za-z0-9]+`)

// simhash64Port computes a 64-bit Charikar-style simhash of `s`.
// Tokenizes on non-alphanumeric, lowercases, hashes each token with
// FNV-64, then increments/decrements per-bit counters and uses their
// sign to derive the final hash.
func simhash64Port(s string) uint64 {
	s = strings.ToLower(s)
	tokens := dedupPortWordSplit.Split(s, -1)
	if len(tokens) == 0 {
		return 0
	}
	var v [64]int32
	for _, t := range tokens {
		if t == "" {
			continue
		}
		h := fnv64aPort([]byte(t))
		for i := 0; i < 64; i++ {
			if (h>>uint(i))&1 == 1 {
				v[i]++
			} else {
				v[i]--
			}
		}
	}
	var out uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			out |= 1 << uint(i)
		}
	}
	return out
}

func hammingDistancePort(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// ngramShinglesPort returns the set of character n-grams (default n=3)
// of the lowercased, whitespace-collapsed string.
func ngramShinglesPort(s string, n int) map[string]struct{} {
	if n < 1 {
		n = 3
	}
	s = strings.ToLower(s)
	s = dedupPortWordSplit.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	out := map[string]struct{}{}
	if len(s) < n {
		out[s] = struct{}{}
		return out
	}
	runes := []rune(s)
	for i := 0; i+n <= len(runes); i++ {
		out[string(runes[i:i+n])] = struct{}{}
	}
	return out
}

func jaccardNgramPort(a, b string) float64 {
	A := ngramShinglesPort(a, 3)
	B := ngramShinglesPort(b, 3)
	if len(A) == 0 && len(B) == 0 {
		return 1.0
	}
	intersect := 0
	for k := range A {
		if _, ok := B[k]; ok {
			intersect++
		}
	}
	union := len(A) + len(B) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

// hybridSimilarityPort combines simhash hamming (normalized) and Jaccard
// n-gram, weighted 30/70. Yields 0.0-1.0.
func hybridSimilarityPort(a, subA, b, subB string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1.0
	}
	ha, hb := simhash64Port(a), simhash64Port(b)
	hd := hammingDistancePort(ha, hb)
	simhashSim := 1.0 - float64(hd)/64.0
	jac := jaccardNgramPort(a, b)
	combined := 0.3*simhashSim + 0.7*jac
	if subA != "" && subA == subB {
		combined += 0.05
	}
	if combined > 1.0 {
		combined = 1.0
	}
	return combined
}

// semanticHashPort is the canonical 16-char hex hash compatible with
// dagctl's `semanticHash()`. Uses simhash64 (base16) so the schema
// column gets a stable value even if a single token is added.
func semanticHashPort(s string) string {
	h := simhash64Port(s)
	return fmt.Sprintf("%016x", h)
}

// fnv64aPort is a small FNV-1a hash used by simhash64.
func fnv64aPort(b []byte) uint64 {
	const (
		offset uint64 = 14695981039346656037
		prime  uint64 = 1099511628211
	)
	h := offset
	for _, c := range b {
		h ^= uint64(c)
		h *= prime
	}
	return h
}
