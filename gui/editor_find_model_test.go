package gui

// Tests for FindModel, the code editor's local find/replace model. Everything
// here is pure model work: no Frame, no Draw, no font metrics, so it runs
// headless. Layers covered:
//   1. Search    - literal vs regex, case, whole word (Unicode), in-selection,
//                  line/col mapping, zero-width patterns, invalid regex
//   2. Next/Prev - wrap-around from a caret offset
//   3. Replace   - ReplaceOne from a caret, ReplaceAll offset correctness with
//                  longer/shorter/empty replacements, preserve-case

import (
	"strings"
	"testing"
)

// findModelSearch runs a search over the whole text and fails on error.
func findModelSearch(t *testing.T, opt FindOptions, text string) []FindMatch {
	t.Helper()
	ms, err := NewFindModel(opt).Search(text, FindRange{})
	if err != nil {
		t.Fatalf("Search(%q) opt %+v: unexpected error: %v", text, opt, err)
	}
	return ms
}

// findModelSpans returns the matched substrings, which read better in failures
// than raw offsets.
func findModelSpans(text string, ms []FindMatch) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = text[m.Start:m.End]
	}
	return out
}

func findModelStarts(ms []FindMatch) []int {
	out := make([]int, len(ms))
	for i, m := range ms {
		out[i] = m.Start
	}
	return out
}

func findModelEqualInts(a, b []int) bool {
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

// --- 1. Search ---

func TestFindModelLiteralVsRegex(t *testing.T) {
	text := "a.c abc"

	// Literal: the dot is a dot, so only the first token matches.
	got := findModelSearch(t, FindOptions{Query: "a.c"}, text)
	if len(got) != 1 || got[0].Start != 0 || got[0].End != 3 {
		t.Fatalf("literal a.c = %v (%v), want one match [0,3)", got, findModelSpans(text, got))
	}

	// Regex: the dot is any rune, so both tokens match.
	got = findModelSearch(t, FindOptions{Query: "a.c", Regex: true}, text)
	if want := []int{0, 4}; !findModelEqualInts(findModelStarts(got), want) {
		t.Fatalf("regex a.c starts = %v, want %v", findModelStarts(got), want)
	}
}

func TestFindModelLiteralQueryIsQuoted(t *testing.T) {
	// A literal query full of metacharacters must not fail to compile.
	text := "call f a( b"
	got := findModelSearch(t, FindOptions{Query: "a("}, text)
	if len(got) != 1 || text[got[0].Start:got[0].End] != "a(" {
		t.Fatalf("literal a( = %v (%v), want one match", got, findModelSpans(text, got))
	}
}

func TestFindModelCaseSensitivity(t *testing.T) {
	text := "Foo foo FOO"

	if got := findModelSearch(t, FindOptions{Query: "foo"}, text); len(got) != 3 {
		t.Errorf("case-insensitive literal: want 3, got %v", findModelStarts(got))
	}
	got := findModelSearch(t, FindOptions{Query: "foo", CaseSensitive: true}, text)
	if len(got) != 1 || got[0].Start != 4 {
		t.Errorf("case-sensitive literal: want one match at 4, got %v", findModelStarts(got))
	}

	if got := findModelSearch(t, FindOptions{Query: "f.o", Regex: true}, text); len(got) != 3 {
		t.Errorf("case-insensitive regex: want 3, got %v", findModelStarts(got))
	}
	got = findModelSearch(t, FindOptions{Query: "f.o", Regex: true, CaseSensitive: true}, text)
	if len(got) != 1 || got[0].Start != 4 {
		t.Errorf("case-sensitive regex: want one match at 4, got %v", findModelStarts(got))
	}
}

func TestFindModelWholeWordBoundaries(t *testing.T) {
	opt := FindOptions{Query: "foo", WholeWord: true}
	cases := []struct {
		text string
		want int
	}{
		{"foo", 1},         // whole text
		{"foo bar", 1},     // space after
		{"bar foo", 1},     // space before
		{"(foo)", 1},       // punctuation both sides
		{"foobar", 0},      // letter after
		{"barfoo", 0},      // letter before
		{"_foo", 0},        // underscore counts as a word rune
		{"foo_", 0},        // ... on either side
		{"foo1", 0},        // digit after
		{"1foo", 0},        // digit before
		{"foo foo", 2},     // both hits are words
		{"foo foobar", 1},  // second hit is not
		{"foo\nfoo", 2},    // newline is a boundary
		{"foo.foo,foo", 3}, // punctuation is a boundary
	}
	for _, c := range cases {
		if got := findModelSearch(t, opt, c.text); len(got) != c.want {
			t.Errorf("whole-word %q: got %d matches %v, want %d",
				c.text, len(got), findModelSpans(c.text, got), c.want)
		}
	}

	// Unicode boundaries, not RE2's ASCII-only \b: the accented letter after
	// "caf" is a word rune, so "caf" is not a whole word inside "café".
	if got := findModelSearch(t, opt, "café"); len(got) != 0 {
		t.Errorf(`"caf" inside "café" must not be a whole word, got %v`, got)
	}
	if got := findModelSearch(t, FindOptions{Query: "caf", WholeWord: true}, "un caf"); len(got) != 1 {
		t.Errorf(`"caf" in "un caf" must be a whole word, got %v`, got)
	}
	// A multi-byte trailing rune of the match itself is decoded correctly.
	if got := findModelSearch(t, FindOptions{Query: "café", WholeWord: true}, "un café noir"); len(got) != 1 {
		t.Errorf(`"café" in "un café noir" must be a whole word, got %v`, got)
	}
	if got := findModelSearch(t, FindOptions{Query: "café", WholeWord: true}, "un cafés"); len(got) != 0 {
		t.Errorf(`"café" inside "cafés" must not be a whole word, got %v`, got)
	}

	// A query that begins and ends on punctuation needs no outer boundary: the
	// boundary is only enforced where two word runes would meet.
	if got := findModelSearch(t, FindOptions{Query: "-", WholeWord: true}, "a-b"); len(got) != 1 {
		t.Errorf(`"-" in "a-b" must match with whole word on, got %v`, got)
	}

	// Whole word composes with case folding and with regex mode.
	if got := findModelSearch(t, FindOptions{Query: "Foo", WholeWord: true}, "FOO foobar"); len(got) != 1 {
		t.Errorf("whole-word + case fold: want 1, got %v", findModelStarts(got))
	}
	got := findModelSearch(t, FindOptions{Query: "fo+", Regex: true, WholeWord: true}, "foo fooo foobar")
	if len(got) != 2 {
		t.Errorf("whole-word + regex: want 2, got %v", findModelSpans("foo fooo foobar", got))
	}
}

func TestFindModelInSelectionScope(t *testing.T) {
	text := "foo foo foo" // matches at 0, 4, 8

	// InSelection off: sel is ignored entirely.
	ms, err := NewFindModel(FindOptions{Query: "foo"}).Search(text, FindRange{Start: 4, End: 7})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ms) != 3 {
		t.Errorf("InSelection off must ignore sel: got %v", findModelStarts(ms))
	}

	m := NewFindModel(FindOptions{Query: "foo", InSelection: true})

	ms, _ = m.Search(text, FindRange{Start: 4, End: 11})
	if want := []int{4, 8}; !findModelEqualInts(findModelStarts(ms), want) {
		t.Errorf("sel [4,11): got %v, want %v", findModelStarts(ms), want)
	}

	// A match straddling the selection edge is dropped, not clipped.
	ms, _ = m.Search(text, FindRange{Start: 1, End: 11})
	if want := []int{4, 8}; !findModelEqualInts(findModelStarts(ms), want) {
		t.Errorf("straddling match must be dropped: got %v, want %v", findModelStarts(ms), want)
	}
	ms, _ = m.Search(text, FindRange{Start: 2, End: 7})
	if want := []int{4}; !findModelEqualInts(findModelStarts(ms), want) {
		t.Errorf("sel [2,7): got %v, want %v", findModelStarts(ms), want)
	}

	// A reversed selection is normalized; an empty one finds nothing.
	ms, _ = m.Search(text, FindRange{Start: 11, End: 4})
	if want := []int{4, 8}; !findModelEqualInts(findModelStarts(ms), want) {
		t.Errorf("reversed sel: got %v, want %v", findModelStarts(ms), want)
	}
	if ms, _ = m.Search(text, FindRange{Start: 5, End: 5}); len(ms) != 0 {
		t.Errorf("empty sel must find nothing, got %v", findModelStarts(ms))
	}
	// Out-of-range selections are clamped instead of panicking.
	ms, _ = m.Search(text, FindRange{Start: -20, End: 999})
	if len(ms) != 3 {
		t.Errorf("clamped sel: got %v, want 3 matches", findModelStarts(ms))
	}
}

func TestFindModelLineAndColumn(t *testing.T) {
	// Multi-byte runes must shift byte offsets but not rune columns.
	text := "héllo foo\nbar foo\n\tfoo"
	got := findModelSearch(t, FindOptions{Query: "foo"}, text)
	want := []FindMatch{
		{Start: 7, End: 10, Line: 0, Col: 6},
		{Start: 15, End: 18, Line: 1, Col: 4},
		{Start: 20, End: 23, Line: 2, Col: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("want %d matches, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("match %d = %+v, want %+v", i, got[i], want[i])
		}
		if text[got[i].Start:got[i].End] != "foo" {
			t.Errorf("match %d spans %q, want %q", i, text[got[i].Start:got[i].End], "foo")
		}
	}
}

func TestFindModelNonOverlapping(t *testing.T) {
	// Same rule as the editor's own scanner: adjacent, never overlapping.
	got := findModelSearch(t, FindOptions{Query: "aa"}, "aaaa")
	if want := []int{0, 2}; !findModelEqualInts(findModelStarts(got), want) {
		t.Fatalf("starts = %v, want %v", findModelStarts(got), want)
	}
}

func TestFindModelEmptyQuery(t *testing.T) {
	m := NewFindModel(FindOptions{})
	if ms, err := m.Search("abc", FindRange{}); err != nil || ms != nil {
		t.Errorf("empty query: got %v, %v; want nil, nil", ms, err)
	}
	if _, ok, _ := m.Next("abc", FindRange{}, 0); ok {
		t.Error("empty query: Next must report no match")
	}
	if out, applied, err := m.ReplaceAll("abc", "X", FindRange{}); out != "abc" || applied != nil || err != nil {
		t.Errorf("empty query ReplaceAll: got %q, %v, %v; want unchanged", out, applied, err)
	}
}

func TestFindModelZeroWidthPatternTerminates(t *testing.T) {
	// "a*" can match nothing at all. The scan must advance a rune past an empty
	// match instead of retrying at the same offset, so this returns rather than
	// hanging, and no two matches share a start.
	text := "baab"
	got := findModelSearch(t, FindOptions{Query: "a*", Regex: true}, text)
	want := []FindMatch{
		{Start: 0, End: 0, Line: 0, Col: 0}, // empty, before 'b'
		{Start: 1, End: 3, Line: 0, Col: 1}, // "aa"
		{Start: 4, End: 4, Line: 0, Col: 4}, // empty, at end of text
	}
	if len(got) != len(want) {
		t.Fatalf("a* on %q: got %d matches %+v, want %d", text, len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("a* match %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// A pattern that only ever matches empty must terminate too, one hit per
	// line start (^ is per-line).
	got = findModelSearch(t, FindOptions{Query: "^", Regex: true}, "a\nb\nc")
	if want := []int{0, 2, 4}; !findModelEqualInts(findModelStarts(got), want) {
		t.Errorf("^ starts = %v, want %v", findModelStarts(got), want)
	}
	// ... and the anchor really is per-line, not per-document.
	got = findModelSearch(t, FindOptions{Query: "^b", Regex: true}, "a\nb\nc")
	if len(got) != 1 || got[0].Start != 2 || got[0].Line != 1 {
		t.Errorf("^b = %+v, want one match at offset 2 on line 1", got)
	}

	// Next must step off a zero-width match sitting on the caret.
	m := NewFindModel(FindOptions{Query: "a*", Regex: true})
	if hit, ok, _ := m.Next(text, FindRange{}, 0); !ok || hit.Start != 1 {
		t.Errorf("Next past zero-width match at caret = %+v (ok %v), want start 1", hit, ok)
	}

	// Replacing a zero-width match inserts without looping or losing text.
	out, applied, err := m.ReplaceAll(text, "-", FindRange{})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if out != "-b-b-" || len(applied) != 3 {
		t.Errorf("ReplaceAll on zero-width = %q %v, want %q with 3 ranges", out, applied, "-b-b-")
	}
}

func TestFindModelInvalidRegex(t *testing.T) {
	m := NewFindModel(FindOptions{Query: "a(", Regex: true})
	text := "a(b"

	ms, err := m.Search(text, FindRange{})
	if err == nil {
		t.Fatal("Search with invalid regex: want error")
	}
	if ms != nil {
		t.Errorf("Search with invalid regex returned matches: %v", ms)
	}
	if !strings.Contains(err.Error(), "a(") {
		t.Errorf("error %q should name the offending pattern", err)
	}

	if _, ok, err := m.Next(text, FindRange{}, 0); ok || err == nil {
		t.Errorf("Next with invalid regex = ok %v, err %v; want false, error", ok, err)
	}
	if _, ok, err := m.Prev(text, FindRange{}, 0); ok || err == nil {
		t.Errorf("Prev with invalid regex = ok %v, err %v; want false, error", ok, err)
	}
	if out, applied, err := m.ReplaceOne(text, "X", FindRange{}, 0); out != text || applied != nil || err == nil {
		t.Errorf("ReplaceOne with invalid regex = %q, %v, %v; want text unchanged + error", out, applied, err)
	}
	if out, applied, err := m.ReplaceAll(text, "X", FindRange{}); out != text || applied != nil || err == nil {
		t.Errorf("ReplaceAll with invalid regex = %q, %v, %v; want text unchanged + error", out, applied, err)
	}
}

// --- 2. Next / Prev ---

func TestFindModelNextPrevWrapAround(t *testing.T) {
	text := "foo foo foo" // starts 0, 4, 8
	m := NewFindModel(FindOptions{Query: "foo"})

	next := []struct{ caret, want int }{
		{-5, 0}, {0, 4}, {1, 4}, {4, 8}, {7, 8}, {8, 0}, {11, 0}, {999, 0},
	}
	for _, c := range next {
		hit, ok, err := m.Next(text, FindRange{}, c.caret)
		if err != nil || !ok {
			t.Fatalf("Next(caret %d) = ok %v, err %v", c.caret, ok, err)
		}
		if hit.Start != c.want {
			t.Errorf("Next(caret %d) start = %d, want %d", c.caret, hit.Start, c.want)
		}
	}

	prev := []struct{ caret, want int }{
		{999, 8}, {11, 8}, {8, 4}, {5, 4}, {4, 0}, {1, 0}, {0, 8}, {-5, 8},
	}
	for _, c := range prev {
		hit, ok, err := m.Prev(text, FindRange{}, c.caret)
		if err != nil || !ok {
			t.Fatalf("Prev(caret %d) = ok %v, err %v", c.caret, ok, err)
		}
		if hit.Start != c.want {
			t.Errorf("Prev(caret %d) start = %d, want %d", c.caret, hit.Start, c.want)
		}
	}

	// Nothing to find: ok is false and no error.
	miss := NewFindModel(FindOptions{Query: "zzz"})
	if _, ok, err := miss.Next(text, FindRange{}, 0); ok || err != nil {
		t.Errorf("Next with no matches = ok %v, err %v; want false, nil", ok, err)
	}
	if _, ok, err := miss.Prev(text, FindRange{}, 0); ok || err != nil {
		t.Errorf("Prev with no matches = ok %v, err %v; want false, nil", ok, err)
	}
}

func TestFindModelNextPrevRespectSelectionScope(t *testing.T) {
	text := "foo foo foo"
	m := NewFindModel(FindOptions{Query: "foo", InSelection: true})
	sel := FindRange{Start: 4, End: 11}

	// Wrapping stays inside the selection: caret past the last in-scope match
	// comes back to the first in-scope one, never to offset 0.
	if hit, ok, _ := m.Next(text, sel, 8); !ok || hit.Start != 4 {
		t.Errorf("Next wrap in selection = %+v (ok %v), want start 4", hit, ok)
	}
	if hit, ok, _ := m.Prev(text, sel, 4); !ok || hit.Start != 8 {
		t.Errorf("Prev wrap in selection = %+v (ok %v), want start 8", hit, ok)
	}
}

// --- 3. Replace ---

func TestFindModelReplaceOneFromCaret(t *testing.T) {
	text := "foo bar foo"
	m := NewFindModel(FindOptions{Query: "foo"})

	out, applied, err := m.ReplaceOne(text, "X", FindRange{}, 0)
	if err != nil {
		t.Fatalf("ReplaceOne: %v", err)
	}
	if out != "X bar foo" {
		t.Errorf("ReplaceOne(caret 0) = %q, want %q", out, "X bar foo")
	}
	if len(applied) != 1 || applied[0] != (FindRange{Start: 0, End: 1}) {
		t.Errorf("applied = %v, want [{0 1}]", applied)
	}

	// The caret picks the match at or after it, so a highlighted match is the
	// one that gets replaced.
	out, applied, _ = m.ReplaceOne(text, "X", FindRange{}, 4)
	if out != "foo bar X" || len(applied) != 1 || applied[0] != (FindRange{Start: 8, End: 9}) {
		t.Errorf("ReplaceOne(caret 4) = %q %v, want %q [{8 9}]", out, applied, "foo bar X")
	}
	out, _, _ = m.ReplaceOne(text, "X", FindRange{}, 8)
	if out != "foo bar X" {
		t.Errorf("ReplaceOne(caret 8) = %q, want %q", out, "foo bar X")
	}
	// Past the end it wraps to the first match.
	out, _, _ = m.ReplaceOne(text, "X", FindRange{}, 999)
	if out != "X bar foo" {
		t.Errorf("ReplaceOne(caret 999) = %q, want %q", out, "X bar foo")
	}

	// The applied range indexes the new text, so it stays right when the
	// replacement is longer than the match.
	out, applied, _ = m.ReplaceOne(text, "quux", FindRange{}, 4)
	if len(applied) != 1 || out[applied[0].Start:applied[0].End] != "quux" {
		t.Errorf("ReplaceOne long repl: %q %v does not point at the insertion", out, applied)
	}

	// No match: text untouched, nothing applied, no error.
	miss := NewFindModel(FindOptions{Query: "zzz"})
	if out, applied, err := miss.ReplaceOne(text, "X", FindRange{}, 0); out != text || applied != nil || err != nil {
		t.Errorf("ReplaceOne no match = %q, %v, %v; want unchanged", out, applied, err)
	}
}

func TestFindModelReplaceAllOffsets(t *testing.T) {
	text := "foo bar foo baz foo"
	m := NewFindModel(FindOptions{Query: "foo"})

	cases := []struct {
		repl    string
		want    string
		applied []FindRange
	}{
		{"XY", "XY bar XY baz XY", []FindRange{{0, 2}, {7, 9}, {14, 16}}},                   // shorter
		{"longer", "longer bar longer baz longer", []FindRange{{0, 6}, {11, 17}, {22, 28}}}, // longer
		{"", " bar  baz ", []FindRange{{0, 0}, {5, 5}, {10, 10}}},                           // deletion
		{"foofoo", "foofoo bar foofoo baz foofoo", []FindRange{{0, 6}, {11, 17}, {22, 28}}}, // repl contains the query
	}
	for _, c := range cases {
		out, applied, err := m.ReplaceAll(text, c.repl, FindRange{})
		if err != nil {
			t.Fatalf("ReplaceAll(%q): %v", c.repl, err)
		}
		if out != c.want {
			t.Errorf("ReplaceAll(%q) = %q, want %q", c.repl, out, c.want)
		}
		if len(applied) != len(c.applied) {
			t.Fatalf("ReplaceAll(%q) applied = %v, want %v", c.repl, applied, c.applied)
		}
		for i, r := range c.applied {
			if applied[i] != r {
				t.Errorf("ReplaceAll(%q) applied[%d] = %v, want %v", c.repl, i, applied[i], r)
			}
			// The range must actually frame the inserted text in the new text.
			if applied[i].End <= len(out) && out[applied[i].Start:applied[i].End] != c.repl {
				t.Errorf("ReplaceAll(%q) applied[%d] spans %q, want %q",
					c.repl, i, out[applied[i].Start:applied[i].End], c.repl)
			}
		}
	}
}

func TestFindModelReplaceAllInSelection(t *testing.T) {
	text := "foo foo foo"
	m := NewFindModel(FindOptions{Query: "foo", InSelection: true})
	out, applied, err := m.ReplaceAll(text, "X", FindRange{Start: 4, End: 11})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if out != "foo X X" {
		t.Errorf("scoped ReplaceAll = %q, want %q", out, "foo X X")
	}
	if len(applied) != 2 || applied[0] != (FindRange{4, 5}) || applied[1] != (FindRange{6, 7}) {
		t.Errorf("scoped applied = %v, want [{4 5} {6 7}]", applied)
	}
}

func TestFindModelReplaceAllRegex(t *testing.T) {
	// Regex replace inserts the replacement literally (no $1 expansion), and
	// variable-length matches keep the applied ranges honest.
	text := "x1 x22 x333"
	m := NewFindModel(FindOptions{Query: `x\d+`, Regex: true})
	out, applied, err := m.ReplaceAll(text, "n", FindRange{})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if out != "n n n" {
		t.Errorf("regex ReplaceAll = %q, want %q", out, "n n n")
	}
	for i, r := range applied {
		if out[r.Start:r.End] != "n" {
			t.Errorf("applied[%d] = %v spans %q, want \"n\"", i, r, out[r.Start:r.End])
		}
	}
}

func TestFindModelPreserveCaseReplace(t *testing.T) {
	text := "FOO Foo foo fOo"

	// Off: the replacement is written exactly as typed.
	plain := NewFindModel(FindOptions{Query: "foo"})
	if out, _, _ := plain.ReplaceAll(text, "bar", FindRange{}); out != "bar bar bar bar" {
		t.Errorf("PreserveCase off = %q, want %q", out, "bar bar bar bar")
	}

	// On: ALLCAPS -> BAR, Capitalized -> Bar, lower -> bar, mixed -> verbatim.
	keep := NewFindModel(FindOptions{Query: "foo", PreserveCase: true})
	if out, _, _ := keep.ReplaceAll(text, "bar", FindRange{}); out != "BAR Bar bar bar" {
		t.Errorf("PreserveCase on = %q, want %q", out, "BAR Bar bar bar")
	}
	// An uppercase replacement is folded down for a lowercase match.
	if out, _, _ := keep.ReplaceAll("foo", "BAR", FindRange{}); out != "bar" {
		t.Errorf("lower match + upper repl = %q, want %q", out, "bar")
	}
	// Capitalized only touches the first rune, so the replacement keeps its humps.
	if out, _, _ := keep.ReplaceAll("Foo", "fooBar", FindRange{}); out != "FooBar" {
		t.Errorf("title match = %q, want %q", out, "FooBar")
	}
	// A lone uppercase letter is Capitalized, not ALLCAPS.
	single := NewFindModel(FindOptions{Query: "I", CaseSensitive: true, PreserveCase: true})
	if out, _, _ := single.ReplaceAll("I think", "you", FindRange{}); out != "You think" {
		t.Errorf("single-letter match = %q, want %q", out, "You think")
	}
	// Uncased runes are ignored when classifying.
	snake := NewFindModel(FindOptions{Query: "foo_bar", PreserveCase: true})
	if out, _, _ := snake.ReplaceAll("FOO_BAR", "baz_qux", FindRange{}); out != "BAZ_QUX" {
		t.Errorf("uppercase snake case = %q, want %q", out, "BAZ_QUX")
	}
	// ReplaceOne recases too.
	if out, _, _ := keep.ReplaceOne(text, "bar", FindRange{}, 0); out != "BAR Foo foo fOo" {
		t.Errorf("ReplaceOne preserve case = %q, want %q", out, "BAR Foo foo fOo")
	}
}

func TestFindModelCaseClassification(t *testing.T) {
	cases := []struct {
		in   string
		want findCase
	}{
		{"FOO", findCaseUpper},
		{"FOO_BAR", findCaseUpper},
		{"AB", findCaseUpper},
		{"Foo", findCaseTitle},
		{"F", findCaseTitle},
		{"Foo_bar", findCaseTitle},
		{"foo", findCaseLower},
		{"foo_bar1", findCaseLower},
		{"fooBar", findCaseOther},
		{"fOo", findCaseOther},
		{"FooBar", findCaseOther},
		{"", findCaseOther},
		{"123_", findCaseOther},
	}
	for _, c := range cases {
		if got := findCaseOf(c.in); got != c.want {
			t.Errorf("findCaseOf(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := findApplyCase(findCaseOther, "aBc"); got != "aBc" {
		t.Errorf("findApplyCase(other) = %q, want %q", got, "aBc")
	}
	if got := findApplyCase(findCaseTitle, ""); got != "" {
		t.Errorf("findApplyCase(title, empty) = %q, want empty", got)
	}
}
