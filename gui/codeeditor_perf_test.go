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

// ---------------------------------------------------------------------------
// Draw-hot-path measurement + fold-region caches.
//
// Three optimizations proven below, all behavior-preserving:
//
//   1. measureText / per-token advance take a monospace fast path
//      (len(s)*charWidth) instead of a per-string cgo Cairo TextExtents call.
//   2. FoldRegions() memoizes the O(all lines) brace scan.
//   3. visibleLineIndices() memoizes the O(all lines) visible-index build.
//
// The tests pin pixel-identity (fast path == Cairo shaping for monospace ASCII,
// fallback for anything else) and cache invalidation; the benchmarks show the
// alloc/ns drop before→after.
// ---------------------------------------------------------------------------

// requireMonoFont skips the caller when the editor's font is not detected as
// monospace in this environment (e.g. no Cairo font backend), since the fast
// path is then inert and there is nothing to assert.
func requireMonoFont(t *testing.T, e *CodeEditor) {
	t.Helper()
	e.ensureFontMetrics()
	if !e.monoFont {
		t.Skip("editor font not detected as monospace here; measure fast path inert")
	}
}

// foldSource2000 builds a ~2000-line Go-ish buffer full of nested brace blocks
// so computeFoldRegions has real work (many multi-line regions) to do.
func foldSource2000() string {
	var sb strings.Builder
	for i := 0; i < 400; i++ {
		sb.WriteString("func f() {\n")
		sb.WriteString("\tif x > 0 {\n")
		sb.WriteString("\t\treturn 1\n")
		sb.WriteString("\t}\n")
		sb.WriteString("}\n")
	}
	return sb.String()
}

// TestMeasureTextMonospaceMatchesShaping: on a monospace font the fast path
// (len(s)*charWidth) must equal Cairo's shaped advance to sub-pixel precision
// for every pure-ASCII corpus line, i.e. rendering is pixel-identical.
func TestMeasureTextMonospaceMatchesShaping(t *testing.T) {
	e := NewCodeEditor()
	requireMonoFont(t, e)
	const eps = 1e-4 // far below one pixel; guards against float sum-vs-mul ULPs
	for _, s := range tokCorpus {
		if s == "" {
			continue
		}
		got := e.measureText(s)
		want := e.font.TextExtents(s).XAdvance
		if diff := got - want; diff > eps || diff < -eps {
			t.Errorf("measureText(%q)=%v shaped=%v diff=%g (ascii=%v)", s, got, want, diff, isASCIIPrintable(s))
		}
	}
}

// TestMeasureTextNonASCIIFallback: strings with a tab or a non-ASCII rune must
// bypass the fast path and return the exact shaped advance (no approximation).
func TestMeasureTextNonASCIIFallback(t *testing.T) {
	e := NewCodeEditor()
	requireMonoFont(t, e)
	for _, s := range []string{"\tindented", "func\tx", "héllo", "日本語", "café"} {
		if isASCIIPrintable(s) {
			t.Fatalf("test string %q should not be ASCII-printable", s)
		}
		got := e.measureText(s)
		want := e.font.TextExtents(s).XAdvance
		if got != want {
			t.Errorf("fallback measureText(%q)=%v want exact shaped %v", s, got, want)
		}
	}
}

// TestMeasureTextZeroAllocMonospace: the monospace fast path must not allocate
// (it was a cgo TextExtents call with a C.CString heap allocation before).
func TestMeasureTextZeroAllocMonospace(t *testing.T) {
	e := NewCodeEditor()
	requireMonoFont(t, e)
	const line = "func foo(s string) (int, error) { return 42, nil }"
	_ = e.measureText(line) // warm ensureFontMetrics
	if n := testing.AllocsPerRun(200, func() { _ = e.measureText(line) }); n != 0 {
		t.Errorf("measureText fast path allocated %v times/op, want 0", n)
	}
}

// TestFoldRegionsCacheMatchesUncached: the cached FoldRegions() must return the
// same regions as a fresh computeFoldRegions over the current lines.
func TestFoldRegionsCacheMatchesUncached(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(foldSource2000())
	got := e.FoldRegions()          // first call computes + caches
	want := computeFoldRegions(e.Lines())
	if len(got) != len(want) {
		t.Fatalf("cached fold region count=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("region[%d]=%+v want %+v", i, got[i], want[i])
		}
	}
	// Second call is a cache hit and must be identical.
	if got2 := e.FoldRegions(); len(got2) != len(want) {
		t.Fatalf("cache-hit fold region count=%d want=%d", len(got2), len(want))
	}
}

// TestFoldCacheInvalidatesOnEdit: changing a brace via an in-line edit (line
// count unchanged) must be reflected by FoldRegions() — the cache invalidates
// through rebuildText on every edit, not merely on a line-count change.
func TestFoldCacheInvalidatesOnEdit(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("func f() {\n\treturn\n}\nfunc g() {\n\treturn\n}")
	before := len(e.FoldRegions()) // prime cache: two regions (f and g bodies)
	if before != 2 {
		t.Fatalf("setup: expected 2 fold regions, got %d", before)
	}

	// Drop the opening brace of g, keeping the line count the same. Its region
	// (now an unbalanced body) must disappear from the recomputed set.
	lines := e.Lines()
	lines[3] = "func g()"
	e.rebuildText()

	after := len(e.FoldRegions())
	wantAfter := len(computeFoldRegions(e.Lines()))
	if after != wantAfter {
		t.Errorf("post-edit FoldRegions()=%d want %d (stale cache?)", after, wantAfter)
	}
	if after == before {
		t.Errorf("expected fold-region count to change after removing a brace (before=%d after=%d)", before, after)
	}
}

// TestVisLineCacheInvalidatesOnFoldToggle: collapsing a region must shrink the
// visible-line list, proving the visLine cache invalidates on ToggleFold.
func TestVisLineCacheInvalidatesOnFoldToggle(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("func f() {\n\ta := 1\n\tb := 2\n}\ntail")
	full := len(e.visibleLineIndices()) // prime cache: all lines visible
	e.ToggleFold(0)                     // collapse the func body (lines 1..3)
	folded := len(e.visibleLineIndices())
	if folded >= full {
		t.Errorf("visibleLineIndices did not shrink after fold: full=%d folded=%d", full, folded)
	}
	want := len(visibleLines(len(e.Lines()), e.foldedLines, e.FoldRegions()))
	if folded != want {
		t.Errorf("cached visibleLineIndices=%d want %d", folded, want)
	}
	e.ToggleFold(0) // expand again → back to full
	if got := len(e.visibleLineIndices()); got != full {
		t.Errorf("visibleLineIndices after re-expand=%d want %d", got, full)
	}
}

// TestVisLineCacheInvalidatesOnSetText: SetText replaces the buffer and resets
// folds, so a stale visible-line slice must not survive.
func TestVisLineCacheInvalidatesOnSetText(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	if got := len(e.visibleLineIndices()); got != 3 {
		t.Fatalf("primed visibleLineIndices=%d want 3", got)
	}
	e.SetText("x\ny")
	if got := len(e.visibleLineIndices()); got != 2 {
		t.Errorf("post-SetText visibleLineIndices=%d want 2 (stale cache?)", got)
	}
}

// BenchmarkMeasureTextShaping: baseline — raw cgo Cairo TextExtents on a typical
// line (C.CString alloc + shaping) every call. Reported next to the fast path.
func BenchmarkMeasureTextShaping(b *testing.B) {
	e := NewCodeEditor()
	const line = "func foo(s string) (int, error) { return 42, nil }"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.font.TextExtents(line).XAdvance
	}
}

// BenchmarkMeasureTextMonospace: the monospace fast path on the same line —
// len(s)*charWidth, no cgo, no allocation.
func BenchmarkMeasureTextMonospace(b *testing.B) {
	e := NewCodeEditor()
	const line = "func foo(s string) (int, error) { return 42, nil }"
	_ = e.measureText(line) // warm ensureFontMetrics
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.measureText(line)
	}
}

// BenchmarkEditorDrawMeasurePathShaping: baseline for the per-token x-advance
// loop in drawHighlightedLine — one cgo TextExtents per visible token.
func BenchmarkEditorDrawMeasurePathShaping(b *testing.B) {
	e := NewCodeEditor()
	const line = "func foo(s string) (int, error) { return 42, nil }"
	toks, _ := tokenizeLine(line, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var x float64
		for _, tk := range toks {
			x += e.font.TextExtents(tk.text).XAdvance
		}
		_ = x
	}
}

// BenchmarkEditorDrawMeasurePath: the optimized per-token advance — measureText
// fast path per token, matching the drawHighlightedLine hot loop with no GL.
func BenchmarkEditorDrawMeasurePath(b *testing.B) {
	e := NewCodeEditor()
	const line = "func foo(s string) (int, error) { return 42, nil }"
	toks, _ := tokenizeLine(line, false)
	_ = e.measureText("0") // warm ensureFontMetrics
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var x float64
		for _, tk := range toks {
			x += e.measureText(tk.text)
		}
		_ = x
	}
}

// BenchmarkFoldRegionsUncached: baseline — recompute the brace scan over a
// 2000-line buffer every call, as the pre-cache Draw did 3+ times per frame.
func BenchmarkFoldRegionsUncached(b *testing.B) {
	lines := strings.Split(foldSource2000(), "\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeFoldRegions(lines)
	}
}

// BenchmarkFoldRegionsCached: FoldRegions() on the same buffer after priming —
// every call is a cache hit returning the memoized slice (no O(lines) scan, no
// allocation).
func BenchmarkFoldRegionsCached(b *testing.B) {
	e := NewCodeEditor()
	e.SetText(foldSource2000())
	_ = e.FoldRegions() // prime the cache
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.FoldRegions()
	}
}
