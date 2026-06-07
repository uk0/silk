package gui

import "testing"

func TestFuzzyMatchTiers(t *testing.T) {
	cases := []struct {
		name      string
		candidate string
		query     string
		want      bool
		// minScore is a non-strict lower bound; 0 means we only assert matched.
		minScore int
	}{
		{"exact", "Println", "Println", true, 1_000_000},
		{"case-insensitive exact", "Println", "println", true, 900_000},
		{"case-sensitive prefix", "Println", "Print", true, 99_000},
		{"case-insensitive prefix", "println", "Print", true, 49_000},
		{"word-boundary subsequence", "fmt.Println", "fP", true, 80},
		{"subsequence contains-not-prefix", "Sprintln", "Pr", true, 30},
		{"non-matching characters", "Println", "xyz", false, 0},
		{"out-of-order rejected", "Println", "lnz", false, 0},
		{"out-of-order subseq rejected", "Println", "nP", false, 0},
		{"empty query matches anything", "Println", "", true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score, ok := fuzzyMatch(tc.candidate, tc.query)
			if ok != tc.want {
				t.Fatalf("matched=%v, want %v (score=%d)", ok, tc.want, score)
			}
			if !ok {
				if score != 0 {
					t.Fatalf("non-match must score 0, got %d", score)
				}
				return
			}
			if score < tc.minScore {
				t.Fatalf("score=%d, want >= %d", score, tc.minScore)
			}
		})
	}
}

// Stronger ordering invariants between tiers: exact > ci-exact > prefix > ci-prefix > subsequence.
func TestFuzzyMatchTierOrdering(t *testing.T) {
	exact, _ := fuzzyMatch("Println", "Println")
	ciExact, _ := fuzzyMatch("Println", "println")
	prefix, _ := fuzzyMatch("Println", "Print")
	ciPrefix, _ := fuzzyMatch("println", "Print")
	subseq, _ := fuzzyMatch("fmt.Println", "fP")

	if !(exact > ciExact && ciExact > prefix && prefix > ciPrefix && ciPrefix > subseq) {
		t.Fatalf("tier order broken: exact=%d ciExact=%d prefix=%d ciPrefix=%d subseq=%d",
			exact, ciExact, prefix, ciPrefix, subseq)
	}
}

// Consecutive runs and word-boundary hits should outscore scattered matches.
func TestFuzzyMatchWordBoundaryBonus(t *testing.T) {
	boundary, okB := fuzzyMatch("fmt.Println", "fP")
	scattered, okS := fuzzyMatch("framework_helper", "fh")
	if !okB || !okS {
		t.Fatalf("expected both to match: boundary=%v scattered=%v", okB, okS)
	}
	// Both hit word boundaries (start, post-'.' / post-'_'), so both should be solid;
	// each match must at least exceed a bare 2-char subsequence on adjacent letters.
	bareAdj, _ := fuzzyMatch("abcdefXYfg", "fg")
	if boundary <= bareAdj {
		t.Fatalf("word-boundary match (%d) should beat scattered tail-adjacent match (%d)", boundary, bareAdj)
	}
	if scattered <= bareAdj {
		t.Fatalf("scattered WB match (%d) should beat scattered tail-adjacent match (%d)", scattered, bareAdj)
	}
}

func TestRankCompletionsPrefixBeatsContains(t *testing.T) {
	items := []CompletionItem{
		{Text: "Println"},
		{Text: "Printf"},
		{Text: "println"},
		{Text: "Sprintln"},
	}
	got := RankCompletions(items, "Pr")
	// Sprintln only matches via subsequence; it must rank below the prefix hits.
	idx := indexOf(got, "Sprintln")
	if idx < 0 {
		t.Fatalf("expected Sprintln in results, got %v", names(got))
	}
	for _, want := range []string{"Println", "Printf"} {
		wi := indexOf(got, want)
		if wi < 0 {
			t.Fatalf("expected %q in results, got %v", want, names(got))
		}
		if wi >= idx {
			t.Fatalf("%q should rank above Sprintln; got order %v", want, names(got))
		}
	}
}

func TestRankCompletionsSpQueryFavorsSprintln(t *testing.T) {
	items := []CompletionItem{
		{Text: "Println"},
		{Text: "Printf"},
		{Text: "println"},
		{Text: "Sprintln"},
	}
	got := RankCompletions(items, "Sp")
	if len(got) == 0 {
		t.Fatalf("expected results for query Sp")
	}
	if got[0].Text != "Sprintln" {
		t.Fatalf("Sp should rank Sprintln first; got %v", names(got))
	}
}

func TestRankCompletionsExactCaseAwareOrder(t *testing.T) {
	items := []CompletionItem{
		{Text: "Println"},
		{Text: "Printf"},
		{Text: "println"},
		{Text: "Sprintln"},
	}
	got := RankCompletions(items, "Println")
	if len(got) < 2 {
		t.Fatalf("expected at least two hits, got %v", names(got))
	}
	if got[0].Text != "Println" {
		t.Fatalf("exact match should rank #1; got %v", names(got))
	}
	if got[1].Text != "println" {
		t.Fatalf("case-insensitive exact should rank #2; got %v", names(got))
	}
}

func TestRankCompletionsEmptyQueryPreservesOrder(t *testing.T) {
	items := []CompletionItem{
		{Text: "alpha"},
		{Text: "beta"},
		{Text: "gamma"},
	}
	got := RankCompletions(items, "")
	if len(got) != len(items) {
		t.Fatalf("empty query should return all %d items, got %d", len(items), len(got))
	}
	for i := range items {
		if got[i].Text != items[i].Text {
			t.Fatalf("empty query must preserve order; got %v", names(got))
		}
	}
}

func TestRankCompletionsShorterWinsLengthDifferentialThroughScore(t *testing.T) {
	// Two prefix matches: scoring already encodes length (100_000 - len) so the
	// shorter candidate must rank first regardless of input order.
	items := []CompletionItem{
		{Text: "PrintlnLong"},
		{Text: "Println"},
	}
	got := RankCompletions(items, "Print")
	if got[0].Text != "Println" {
		t.Fatalf("shorter candidate should win; got %v", names(got))
	}
}

func TestRankCompletionsLengthTieBreaker(t *testing.T) {
	// Construct a true score tie: identical-text items get the same score and
	// length, so we instead use two candidates whose scores really do collide.
	// Easiest: a query that produces the same length-adjusted score for both.
	// "ab" prefixes both with the same length → scores collide; tie-break by
	// length ascending (equal here) then by input order.
	items := []CompletionItem{
		{Text: "abZZZ"},
		{Text: "abAAA"},
	}
	got := RankCompletions(items, "ab")
	if len(got) != 2 {
		t.Fatalf("expected both to match, got %v", names(got))
	}
	// Same length → same score; input order is the final tie-breaker (stable).
	if got[0].Text != "abZZZ" || got[1].Text != "abAAA" {
		t.Fatalf("stable tie-break must preserve input order; got %v", names(got))
	}
}

func TestRankCompletionsStableOnPerfectTie(t *testing.T) {
	// Identical text → identical score and length → original order wins.
	items := []CompletionItem{
		{Text: "Println", Detail: "first"},
		{Text: "Println", Detail: "second"},
	}
	got := RankCompletions(items, "Println")
	if got[0].Detail != "first" || got[1].Detail != "second" {
		t.Fatalf("perfect ties must preserve input order; got %+v", got)
	}
}

func TestRankCompletionsDropsNonMatches(t *testing.T) {
	items := []CompletionItem{
		{Text: "alpha"},
		{Text: "beta"},
		{Text: "gamma"},
	}
	got := RankCompletions(items, "xyz")
	if len(got) != 0 {
		t.Fatalf("non-matching query should drop everything; got %v", names(got))
	}
}

// --- test helpers ---

func indexOf(items []CompletionItem, text string) int {
	for i, it := range items {
		if it.Text == text {
			return i
		}
	}
	return -1
}

func names(items []CompletionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text
	}
	return out
}
