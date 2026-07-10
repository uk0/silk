package locator

import (
	"fmt"
	"strings"
	"testing"
)

// --- subsequence correctness -------------------------------------------

func TestFuzzyScoreSubsequence(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		candidate string
		want      bool
	}{
		{"contiguous", "abc", "abc", true},
		{"scattered subsequence", "abc", "xaybzc", true},
		{"single rune present", "z", "buzz", true},
		{"whole candidate is query", "silk", "silk", true},
		{"out of order rejected", "acb", "abc", false},
		{"reversed rejected", "ba", "abc", false},
		{"missing rune rejected", "abcd", "abc", false},
		{"query longer than candidate", "abcd", "abx", false},
		{"none of the runes present", "xyz", "abc", false},
		{"partial then missing", "abz", "abc", false},
		{"empty candidate, nonempty query", "a", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score, ok := FuzzyScore(tc.query, tc.candidate)
			if ok != tc.want {
				t.Fatalf("FuzzyScore(%q,%q) matched=%v, want %v (score=%d)", tc.query, tc.candidate, ok, tc.want, score)
			}
			if !ok && score != 0 {
				t.Fatalf("non-match must score 0, got %d", score)
			}
			if ok && tc.query != "" && score <= 0 {
				t.Fatalf("a real match must score positive, got %d", score)
			}
		})
	}
}

// --- case insensitivity ------------------------------------------------

func TestFuzzyScoreCaseInsensitive(t *testing.T) {
	// Matching (the matched bool) is case-insensitive however the query and
	// candidate are cased -- that is the case-insensitivity contract.
	matchPairs := []struct{ query, candidate string }{
		{"FOO", "foobar"},
		{"foo", "FOOBAR"},
		{"FoObAr", "foobar"},
		{"SILK", "Silk"},
		{"br", "FooBar"},
	}
	for _, p := range matchPairs {
		if _, ok := FuzzyScore(p.query, p.candidate); !ok {
			t.Fatalf("FuzzyScore(%q,%q) should match case-insensitively", p.query, p.candidate)
		}
	}

	// Uniformly re-casing BOTH query and candidate must not change the
	// score: subsequence positions and the matched tier are preserved, and
	// camelCase humps only exist in mixed case, so a uniform-case base is
	// unaffected. (Scores DO legitimately differ by case otherwise -- an
	// exact-case prefix outranks a folded one, and camelCase earns a
	// boundary bonus -- so equality only holds under a uniform flip.)
	bases := []struct{ query, candidate string }{
		{"foo", "foobar"}, // prefix tier
		{"fbr", "foobar"}, // scattered subsequence
		{"a", "banana"},   // single rune, mid-word
	}
	for _, b := range bases {
		lo, _ := FuzzyScore(strings.ToLower(b.query), strings.ToLower(b.candidate))
		up, _ := FuzzyScore(strings.ToUpper(b.query), strings.ToUpper(b.candidate))
		if lo != up {
			t.Fatalf("uniform re-casing changed score for %q/%q: lower=%d upper=%d",
				b.query, b.candidate, lo, up)
		}
	}
}

// --- empty query -------------------------------------------------------

func TestFuzzyScoreEmptyQuery(t *testing.T) {
	for _, cand := range []string{"anything", "", "日本語", "a/b/c.go"} {
		score, ok := FuzzyScore("", cand)
		if !ok {
			t.Fatalf("empty query should match %q", cand)
		}
		if score != 0 {
			t.Fatalf("empty query neutral score should be 0, got %d for %q", score, cand)
		}
	}
}

// --- unicode / multibyte -----------------------------------------------

func TestFuzzyScoreUnicode(t *testing.T) {
	// Rune-correct subsequence: byte matching would misindex these.
	if _, ok := FuzzyScore("本F", "日本語Foo"); !ok {
		t.Fatal("multibyte subsequence should match")
	}
	if _, ok := FuzzyScore("βγ", "αβγ"); !ok {
		t.Fatal("greek subsequence should match")
	}
	if _, ok := FuzzyScore("γβ", "αβγ"); ok {
		t.Fatal("greek out-of-order must NOT match")
	}
	if _, ok := FuzzyScore("語日", "日本語"); ok {
		t.Fatal("CJK out-of-order must NOT match")
	}

	// Unicode case folding: accented upper folds to its lower form.
	if _, ok := FuzzyScore("É", "Café"); !ok {
		t.Fatal("accented uppercase query should fold-match")
	}
	if _, ok := FuzzyScore("CAFÉ", "café-latte"); !ok {
		t.Fatal("accented fold prefix should match")
	}
	if _, ok := FuzzyScore("ñ", "nino"); ok {
		t.Fatal("ñ is not present in \"nino\"; must not match")
	}
}

// --- tier ordering: exact > ci-exact > prefix > ci-prefix > subseq -----

func TestFuzzyScoreTierOrdering(t *testing.T) {
	exact, _ := FuzzyScore("Println", "Println")   // exact, same case
	ciExact, _ := FuzzyScore("Println", "println") // exact, folded
	prefix, _ := FuzzyScore("Print", "Println")    // prefix, same case
	ciPrefix, _ := FuzzyScore("print", "Println")  // prefix, folded
	subseq, _ := FuzzyScore("pln", "Println")      // scattered subsequence

	if !(exact > ciExact && ciExact > prefix && prefix > ciPrefix && ciPrefix > subseq) {
		t.Fatalf("tier order broken: exact=%d ciExact=%d prefix=%d ciPrefix=%d subseq=%d",
			exact, ciExact, prefix, ciPrefix, subseq)
	}
}

// --- boundary / consecutive bonuses ------------------------------------

func TestFuzzyScoreBoundaryBonuses(t *testing.T) {
	// camelCase boundary beats a same-position mid-word match.
	camel, okC := FuzzyScore("gp", "getPixel")
	midWord, okM := FuzzyScore("gp", "grumpy")
	if !okC || !okM {
		t.Fatalf("both should match: camel=%v mid=%v", okC, okM)
	}
	if camel <= midWord {
		t.Fatalf("camelCase boundary (%d) should beat mid-word scatter (%d)", camel, midWord)
	}

	// Path-separator boundary beats a mid-word match.
	pathB, okP := FuzzyScore("gb", "gui/button")
	midB, okG := FuzzyScore("gb", "grubby")
	if !okP || !okG {
		t.Fatalf("both should match: path=%v mid=%v", okP, okG)
	}
	if pathB <= midB {
		t.Fatalf("path-separator boundary (%d) should beat mid-word scatter (%d)", pathB, midB)
	}

	// Consecutive run beats a scattered match even when the scattered one
	// starts at the very front ("ab" contiguous in xab vs gapped in axb).
	consec, _ := FuzzyScore("ab", "xab")
	scatter, _ := FuzzyScore("ab", "axb")
	if consec <= scatter {
		t.Fatalf("consecutive (%d) should beat scattered (%d)", consec, scatter)
	}

	// A bigger gap must score no better than a smaller gap.
	small, _ := FuzzyScore("ab", "axb")  // gap 1
	big, _ := FuzzyScore("ab", "axxxxb") // gap 4
	if big > small {
		t.Fatalf("larger gap (%d) must not beat smaller gap (%d)", big, small)
	}

	// A prefix match must beat a non-prefix camelCase boundary match.
	pfx, _ := FuzzyScore("fo", "foobar")
	bnd, _ := FuzzyScore("fo", "xFooBar")
	if pfx <= bnd {
		t.Fatalf("prefix (%d) should beat non-prefix boundary (%d)", pfx, bnd)
	}
}

// --- concrete ranking: query "fb" over FooBar / fizzbuzz / afb ---------

func TestFuzzyScoreRankingConcrete(t *testing.T) {
	foobar, okF := FuzzyScore("fb", "FooBar") // prefix start + camelCase boundary
	afb, okA := FuzzyScore("fb", "afb")       // contiguous "fb"
	fizz, okZ := FuzzyScore("fb", "fizzbuzz") // scattered f...b
	if !okF || !okA || !okZ {
		t.Fatalf("all three should match: FooBar=%v afb=%v fizzbuzz=%v", okF, okA, okZ)
	}
	if !(foobar > afb && afb > fizz) {
		t.Fatalf("ranking wrong: FooBar=%d afb=%d fizzbuzz=%d (want FooBar>afb>fizzbuzz)",
			foobar, afb, fizz)
	}
}

// --- no panic / correctness on very long input -------------------------

func TestFuzzyScoreLongInput(t *testing.T) {
	long := strings.Repeat("a", 100_000) + "b"
	if score, ok := FuzzyScore("ab", long); !ok || score <= 0 {
		t.Fatalf("long match should succeed with positive score, got ok=%v score=%d", ok, score)
	}
	if _, ok := FuzzyScore("abc", strings.Repeat("a", 100_000)); ok {
		t.Fatal("long candidate without b/c must not match")
	}

	// Very long unicode candidate, rune-correct.
	uni := strings.Repeat("日", 50_000) + "本"
	if _, ok := FuzzyScore("日本", uni); !ok {
		t.Fatal("long unicode subsequence should match")
	}

	// Long query that is an exact match must not panic and must win big.
	q := strings.Repeat("silk", 10_000)
	if score, ok := FuzzyScore(q, q); !ok || score < exactCaseBonus {
		t.Fatalf("long exact match should score >= %d, got ok=%v score=%d", exactCaseBonus, ok, score)
	}
}

// --- Match: dropping, ordering, concrete ranking -----------------------

func TestMatchDropsNonMatches(t *testing.T) {
	items := []Item{
		{Name: "axby"},
		{Name: "xyz"},
		{Name: "zzz"},
	}
	got := Match("xyz", items)
	if len(got) != 1 || got[0].Name != "xyz" {
		t.Fatalf("Match should keep only \"xyz\", got %v", names(got))
	}
}

func TestMatchConcreteRanking(t *testing.T) {
	items := []Item{
		{Name: "fizzbuzz"},
		{Name: "afb"},
		{Name: "FooBar"},
	}
	got := Match("fb", items)
	want := []string{"FooBar", "afb", "fizzbuzz"}
	if !equalNames(got, want) {
		t.Fatalf("ranking wrong: got %v want %v", names(got), want)
	}
	assertScoreDesc(t, "fb", got)
}

func TestMatchPrefixBeatsBoundary(t *testing.T) {
	items := []Item{
		{Name: "xFooBar"}, // boundary match, not a prefix
		{Name: "foobar"},  // prefix match
	}
	got := Match("fo", items)
	if got[0].Name != "foobar" {
		t.Fatalf("prefix \"foobar\" should rank first, got %v", names(got))
	}
}

// --- Match: empty query returns everything -----------------------------

func TestMatchEmptyQueryReturnsAll(t *testing.T) {
	items := []Item{
		{Name: "c"}, {Name: "a"}, {Name: "b"},
	}
	got := Match("", items)
	if len(got) != len(items) {
		t.Fatalf("empty query should return all %d items, got %d", len(items), len(got))
	}
	// All share the neutral score, so they come back Name-sorted.
	want := []string{"a", "b", "c"}
	if !equalNames(got, want) {
		t.Fatalf("empty query order: got %v want %v (Name-sorted)", names(got), want)
	}
}

// --- Match: empty item list --------------------------------------------

func TestMatchEmptyItems(t *testing.T) {
	if got := Match("query", nil); len(got) != 0 {
		t.Fatalf("Match(query, nil) should be empty, got %v", names(got))
	}
	if got := Match("query", []Item{}); len(got) != 0 {
		t.Fatalf("Match(query, empty) should be empty, got %v", names(got))
	}
	if got := Match("", nil); len(got) != 0 {
		t.Fatalf("Match(\"\", nil) should be empty, got %v", names(got))
	}
	// Result must be non-nil so callers can range without a nil check.
	if got := Match("nomatch", []Item{{Name: "zzz"}}); got == nil {
		t.Fatal("Match must return a non-nil slice even with no hits")
	}
}

// --- Match: stable tie ordering ----------------------------------------

func TestMatchStableTieOrder(t *testing.T) {
	// Equal scores, different Names -> Name ascending.
	byName := Match("x", []Item{{Name: "bx"}, {Name: "ax"}})
	if !equalNames(byName, []string{"ax", "bx"}) {
		t.Fatalf("equal-score tie should sort by Name asc, got %v", names(byName))
	}

	// Identical Name and score, different Detail -> input order preserved.
	got := Match("dup", []Item{
		{Name: "dup", Detail: "first"},
		{Name: "dup", Detail: "second"},
	})
	if len(got) != 2 || got[0].Detail != "first" || got[1].Detail != "second" {
		t.Fatalf("identical Name/score must preserve input order, got %v",
			[]string{detail(got, 0), detail(got, 1)})
	}
}

// --- Match: Kind/Detail carried through untouched -----------------------

func TestMatchCarriesMetadata(t *testing.T) {
	in := Item{Name: "main.go", Detail: "cmd/app/main.go", Kind: "file"}
	got := Match("mg", []Item{in})
	if len(got) != 1 || got[0] != in {
		t.Fatalf("Match must carry Detail/Kind through unchanged, got %+v", got)
	}
}

// --- Match: scale, no panic --------------------------------------------

func TestMatchManyItemsNoPanic(t *testing.T) {
	const n = 10_000
	items := make([]Item, n)
	for i := range items {
		items[i] = Item{Name: fmt.Sprintf("item%05d", i)}
	}
	got := Match("item", items)
	if len(got) != n {
		t.Fatalf("all %d items share the \"item\" prefix; got %d", n, len(got))
	}
	assertScoreDesc(t, "item", got)
}

// --- helpers -----------------------------------------------------------

func names(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Name
	}
	return out
}

func detail(items []Item, i int) string {
	if i < 0 || i >= len(items) {
		return "<oob>"
	}
	return items[i].Detail
}

func equalNames(items []Item, want []string) bool {
	if len(items) != len(want) {
		return false
	}
	for i, it := range items {
		if it.Name != want[i] {
			return false
		}
	}
	return true
}

// assertScoreDesc verifies the slice is ordered by descending score and,
// for equal scores, ascending Name -- the exact contract Match promises.
func assertScoreDesc(t *testing.T, query string, items []Item) {
	t.Helper()
	for i := 1; i < len(items); i++ {
		prev, _ := FuzzyScore(query, items[i-1].Name)
		cur, _ := FuzzyScore(query, items[i].Name)
		if cur > prev {
			t.Fatalf("not score-descending at %d: %q=%d before %q=%d",
				i, items[i-1].Name, prev, items[i].Name, cur)
		}
		if cur == prev && items[i-1].Name > items[i].Name {
			t.Fatalf("tie not Name-ascending at %d: %q before %q",
				i, items[i-1].Name, items[i].Name)
		}
	}
}
