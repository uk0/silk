package gui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// CodeEditor tokenization-cache tests.
//
// tokenizeLineCached wraps tokenizeLine with a per-line memo keyed by a
// fnv64a hash over (line bytes, inBlock). The tests here pin three
// behaviours we care about on the Draw hot path:
//
//   1. Correctness — cached output matches uncached output for a range
//      of representative Go source lines (and the long-line fallback).
//   2. Invalidation — SetText drops the cache; in-line edits self-heal
//      via hash mismatch; line-count changes drop the whole cache via
//      rebuildText.
//   3. Speed — repeated tokenization of a stable line is faster from
//      the cache than from the raw lexer (benchmarked alongside, for
//      the optional `-bench` run).
// ---------------------------------------------------------------------------

// tokenCorpus is a small set of Go source lines that exercise every branch
// of tokenizeLine: comments (line + block), strings (double / raw / rune),
// numbers (decimal / hex / binary / float / exponent / complex), keywords,
// builtin types, identifiers, function calls, and operators.
var tokCorpus = []string{
	"",
	"package gui",
	"import \"strings\"",
	"// a line comment",
	"/* block */ x := 1",
	"func foo(s string) (int, error) { return 42, nil }",
	"const Pi = 3.14159e0",
	"var hex = 0xDEADBEEF",
	"var bin = 0b1010_0101",
	"var c = 1.5i",
	"r := 'a'",
	"s := `raw\nstring`",
	"if x > 0 && y < 10 { return true } else { return false }",
	"m := map[string]int{\"a\": 1, \"b\": 2}",
	"type Foo struct { x int; y float64 }",
	strings.Repeat("x", maxHighlightLineLength+1), // long-line fallback
}

// tokensEqual asserts two token slices carry the same (typ, text) entries
// in the same order. The token struct does not have explicit start/end
// fields; the per-token text is the equivalent positional payload.
func tokensEqual(a, b []token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].typ != b[i].typ || a[i].text != b[i].text {
			return false
		}
	}
	return true
}

// TestTokenizeCachedMatchesUncached: for every line in the corpus and both
// inBlock states, tokenizeLineCached must agree with tokenizeLine on the
// resulting (tokens, nextBlock) pair on first call (cache miss) and on the
// repeat call (cache hit).
func TestTokenizeCachedMatchesUncached(t *testing.T) {
	for _, inBlock := range []bool{false, true} {
		for i, line := range tokCorpus {
			wantTok, wantNext := tokenizeLine(line, inBlock)

			e := NewCodeEditor()
			gotTok, gotNext := e.tokenizeLineCached(i, line, inBlock)
			if !tokensEqual(gotTok, wantTok) || gotNext != wantNext {
				t.Errorf("miss[inBlock=%v][line=%d %q]: cached=(%v,%v) want=(%v,%v)",
					inBlock, i, line, gotTok, gotNext, wantTok, wantNext)
			}

			// Repeat the call: must be a cache hit and return identical output.
			gotTok2, gotNext2 := e.tokenizeLineCached(i, line, inBlock)
			if !tokensEqual(gotTok2, wantTok) || gotNext2 != wantNext {
				t.Errorf("hit[inBlock=%v][line=%d %q]: cached=(%v,%v) want=(%v,%v)",
					inBlock, i, line, gotTok2, gotNext2, wantTok, wantNext)
			}
		}
	}
}

// TestTokenCacheInBlockStateKeyed: the same line text under different inBlock
// flags must produce different cached entries — otherwise a multi-line block
// comment would mis-color when the prefix state flipped.
func TestTokenCacheInBlockStateKeyed(t *testing.T) {
	e := NewCodeEditor()
	const line = "still inside */ then code"

	outTok, outNext := e.tokenizeLineCached(0, line, false)
	inTok, inNext := e.tokenizeLineCached(0, line, true)

	wantOutTok, wantOutNext := tokenizeLine(line, false)
	wantInTok, wantInNext := tokenizeLine(line, true)

	if !tokensEqual(outTok, wantOutTok) || outNext != wantOutNext {
		t.Errorf("inBlock=false: got (%v,%v) want (%v,%v)", outTok, outNext, wantOutTok, wantOutNext)
	}
	if !tokensEqual(inTok, wantInTok) || inNext != wantInNext {
		t.Errorf("inBlock=true: got (%v,%v) want (%v,%v)", inTok, inNext, wantInTok, wantInNext)
	}
	if tokensEqual(outTok, inTok) {
		t.Errorf("inBlock states collapsed to the same cached output for line %q", line)
	}
}

// TestTokenCacheSetTextClears: SetText replaces the buffer wholesale, so the
// cache map must be dropped — line indices have new meanings, and stale
// entries would mis-color the new content.
func TestTokenCacheSetTextClears(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a := 1\nb := 2\nc := 3")

	// Prime the cache by tokenizing every line.
	for i, ln := range e.Lines() {
		_, _ = e.tokenizeLineCached(i, ln, false)
	}
	if len(e.tokenCache) == 0 {
		t.Fatalf("expected primed tokenCache, got empty map")
	}

	e.SetText("x := 100\ny := 200")
	if len(e.tokenCache) != 0 {
		t.Errorf("SetText did not clear tokenCache: %d entries remain", len(e.tokenCache))
	}
}

// TestTokenCacheInlineEditSelfHeals: typing a character on a line without
// changing the line count leaves the map intact; the next read on that line
// must hash-mismatch and recompute, returning the new line's tokens (not the
// stale ones).
func TestTokenCacheInlineEditSelfHeals(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo\nbar\nbaz")

	// Prime line 1 ("bar").
	beforeTok, _ := e.tokenizeLineCached(1, e.Lines()[1], false)
	if len(beforeTok) == 0 {
		t.Fatalf("primed token slice is empty")
	}

	// Mutate line 1 in place without changing the line count, mimicking the
	// effect of a keystroke that lands through rebuildText.
	lines := e.Lines()
	lines[1] = "bar_x"
	e.rebuildText()

	// In-line edit (line count unchanged) does not clear the map, but the
	// hash mismatch on the next read must trigger a recompute.
	afterTok, _ := e.tokenizeLineCached(1, e.Lines()[1], false)
	wantTok, _ := tokenizeLine(e.Lines()[1], false)
	if !tokensEqual(afterTok, wantTok) {
		t.Errorf("post-edit cached=%v want=%v", afterTok, wantTok)
	}
	if tokensEqual(afterTok, beforeTok) {
		t.Errorf("expected recomputed tokens to differ from pre-edit tokens")
	}
}

// TestTokenCacheLineCountChangeClears: inserting or deleting a line shifts
// every subsequent line index, so the rebuildText hook must drop the whole
// cache. Otherwise line N's old hash would still match at the new line N
// (a different line), silently mis-coloring it.
func TestTokenCacheLineCountChangeClears(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a := 1\nb := 2\nc := 3")
	for i, ln := range e.Lines() {
		_, _ = e.tokenizeLineCached(i, ln, false)
	}
	if len(e.tokenCache) == 0 {
		t.Fatalf("primed cache is empty")
	}

	// Insert a line: count goes from 3 → 4.
	lines := append([]string{"// inserted"}, e.Lines()...)
	// Replace via the lines slice directly; we want to mimic an insert path
	// (delete-line / paste-multi-line) that funnels through rebuildText.
	e.lines = lines
	e.rebuildText()

	if len(e.tokenCache) != 0 {
		t.Errorf("line-count change did not clear cache: %d entries remain", len(e.tokenCache))
	}
}

// TestTokenCacheLongLineCachedAsFallback: a line past maxHighlightLineLength
// falls through to the single-token fallback inside tokenizeLine. That
// fallback must still be cached so repeated Draws don't pay the rune-decode
// cost. We confirm the entry exists after the first call and that the second
// call returns the same slice (same content + length).
func TestTokenCacheLongLineCachedAsFallback(t *testing.T) {
	e := NewCodeEditor()
	longLine := strings.Repeat("z", maxHighlightLineLength+50)

	first, _ := e.tokenizeLineCached(0, longLine, false)
	if len(first) != 1 || first[0].typ != tokNormal {
		t.Fatalf("long-line fallback: got %v, want a single tokNormal", first)
	}
	if _, ok := e.tokenCache[0]; !ok {
		t.Errorf("long-line fallback was not cached")
	}

	second, _ := e.tokenizeLineCached(0, longLine, false)
	if !tokensEqual(first, second) {
		t.Errorf("long-line cache hit drifted: first=%v second=%v", first, second)
	}
}

// BenchmarkTokenizeLineUncached: baseline — call tokenizeLine on the same
// representative line repeatedly with no caching. Reported alongside the
// cached benchmark so the speed-up is visible.
func BenchmarkTokenizeLineUncached(b *testing.B) {
	const line = "func foo(s string) (int, error) { return 42, nil }"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = tokenizeLine(line, false)
	}
}

// BenchmarkTokenizeLineCachedHit: warm the cache once, then loop on the
// same (lineIdx, line, inBlock) tuple. Every iteration should hit the
// fnv64a hash check and return the cached slice immediately, which is
// strictly cheaper than the per-rune lexer scan above.
func BenchmarkTokenizeLineCachedHit(b *testing.B) {
	e := NewCodeEditor()
	const line = "func foo(s string) (int, error) { return 42, nil }"
	// Prime the cache.
	_, _ = e.tokenizeLineCached(0, line, false)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = e.tokenizeLineCached(0, line, false)
	}
}
