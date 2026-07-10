// Package locator implements the scoring and filtering engine behind a
// Qt-Creator-style Locator (Ctrl+K) / fuzzy quick-open box. It is
// deliberately UI-agnostic: it knows nothing about widgets, windows, or
// rendering, so an IDE front-end can feed it a flat list of candidates
// (files, symbols, lines) and render the ranked result however it likes.
//
// Matching is subsequence fuzzy matching: a query matches a candidate when
// every rune of the query appears in the candidate, in order, comparing
// case-insensitively. Scoring rewards matches a human reads as "closer" --
// consecutive runs, hits on word / camelCase / path-separator boundaries,
// and prefix or exact matches -- and penalises the gaps between scattered
// matches. All comparison is rune-based, so multi-byte / Unicode
// candidates rank correctly.
package locator

import (
	"sort"
	"strings"
	"unicode"
)

// Item is a single quick-open candidate. Name is the primary string the
// query is scored against (a file name, symbol, or line label). Detail is
// carried through untouched for the UI to show as a subtitle (a path, a
// line preview) and Kind is a free-form category tag ("file", "symbol",
// ...). Only Name participates in scoring.
type Item struct {
	Name   string
	Detail string
	Kind   string
}

// Scoring weights. Kept small and additive so the granular subsequence
// score acts as a stable tiebreaker inside a tier, while the large tier
// bonuses further down dominate across tiers.
const (
	scoreMatch       = 16 // base value earned by each matched rune
	bonusBoundary    = 8  // matched rune sits on a word/camelCase/separator boundary
	bonusConsecutive = 8  // matched rune is adjacent to the previous match
	gapStart         = 3  // penalty for opening a gap between two matches
	gapExtend        = 1  // extra penalty per additional skipped rune
	gapPenaltyMax    = 12 // cap on one gap's penalty (keeps matched scores positive & bounded)
	leadingCap       = 3  // cap on the "how far in does the match start" penalty

	// Tier bonuses. Each dwarfs any achievable granular score, so the
	// tiers strictly order matches: exact(case) > exact(fold) >
	// prefix(case) > prefix(fold) > plain subsequence.
	exactCaseBonus  = 1_000_000
	exactFoldBonus  = 900_000
	prefixCaseBonus = 100_000
	prefixFoldBonus = 50_000
)

// FuzzyScore reports whether query is a case-insensitive subsequence of
// candidate and, when it is, a score where higher means a better match.
//
// An empty query matches every candidate with a neutral score of 0. A
// non-match returns (0, false); callers MUST consult matched rather than
// the score, because a legitimate (very gappy) match can also score low --
// though never zero, so a matched score is always positive.
func FuzzyScore(query, candidate string) (score int, matched bool) {
	qf := foldRunes(query)
	if len(qf) == 0 {
		return 0, true // empty query: matches everything, neutral score
	}
	cf := foldRunes(candidate)
	cr := []rune(candidate) // original case, needed for camelCase boundaries

	// Greedy left-to-right subsequence match. Greedy-earliest is optimal
	// for deciding subsequence membership: if any valid alignment exists,
	// this finds one. positions[k] is where query rune k landed. The scan
	// index ci only ever advances, so this is a single O(len(candidate))
	// pass -- safe on very long candidates.
	positions := make([]int, 0, len(qf))
	ci := 0
	for _, qr := range qf {
		found := -1
		for ci < len(cf) {
			if cf[ci] == qr {
				found = ci
				ci++
				break
			}
			ci++
		}
		if found < 0 {
			return 0, false
		}
		positions = append(positions, found)
	}

	prev := -1
	for k, i := range positions {
		s := scoreMatch
		if isBoundary(cr, i) {
			s += bonusBoundary
		}
		switch {
		case k == 0:
			// Prefer matches that start nearer the front, but only mildly.
			s -= min(i, leadingCap)
		case i == prev+1:
			s += bonusConsecutive
		default:
			gap := i - prev - 1 // runes skipped since the previous match
			s -= min(gapStart+(gap-1)*gapExtend, gapPenaltyMax)
		}
		score += s
		prev = i
	}

	// Tier bonuses, most specific first. foldPrefix means the query is a
	// case-insensitive prefix of (or equal to) the candidate.
	foldPrefix := len(qf) <= len(cf) && runesEqual(cf[:len(qf)], qf)
	switch {
	case query == candidate:
		score += exactCaseBonus
	case len(qf) == len(cf) && foldPrefix:
		score += exactFoldBonus
	case strings.HasPrefix(candidate, query):
		score += prefixCaseBonus
	case foldPrefix:
		score += prefixFoldBonus
	}
	return score, true
}

// Match returns the items whose Name matches query (case-insensitive
// subsequence), best score first. Ties break by Name ascending, then by
// original slice order, so the result is fully deterministic and stable.
// Non-matching items are dropped. An empty query returns every item: all
// share the neutral score, so they come back Name-sorted. The returned
// slice is always non-nil (empty when nothing matches).
func Match(query string, items []Item) []Item {
	type hit struct {
		item  Item
		score int
	}
	hits := make([]hit, 0, len(items))
	for _, it := range items {
		if s, ok := FuzzyScore(query, it.Name); ok {
			hits = append(hits, hit{it, s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].score != hits[b].score {
			return hits[a].score > hits[b].score
		}
		return hits[a].item.Name < hits[b].item.Name
	})
	out := make([]Item, len(hits))
	for i, h := range hits {
		out[i] = h.item
	}
	return out
}

// foldRunes returns the runes of s, each lower-cased for case-insensitive
// comparison. Operating on a rune slice (not bytes) keeps indexing correct
// for multi-byte / Unicode input.
func foldRunes(s string) []rune {
	r := []rune(s)
	for i := range r {
		r[i] = unicode.ToLower(r[i])
	}
	return r
}

// runesEqual reports whether two rune slices are identical.
func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// isAlnum reports whether r is a letter or digit (Unicode-aware). Anything
// else -- space, '/', '.', '_', '-', punctuation -- counts as a separator.
func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// isBoundary reports whether cr[i] begins a new "word" the way a human
// reads it: the first rune, the rune right after a separator (space, '/',
// '.', '_', '-', ...), or the upper-case rune of a camelCase hump
// (non-upper -> Upper).
func isBoundary(cr []rune, i int) bool {
	if i == 0 {
		return true
	}
	prev, curr := cr[i-1], cr[i]
	if !isAlnum(prev) {
		return true
	}
	if unicode.IsUpper(curr) && !unicode.IsUpper(prev) {
		return true
	}
	return false
}
