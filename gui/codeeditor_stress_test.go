package gui

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// CodeEditor pathological-input stress baseline.
//
// The editor is tuned for typical Go source. Real-world buffers occasionally
// arrive in shapes the syntax/fold pipeline was never tuned for:
//   - a generated file with 100k lines,
//   - a minified bundle on a single very long line,
//   - obfuscated input with thousands of nested braces.
//
// These tests pin the guards (maxHighlightLineLength / maxFoldComputeLines)
// and assert that SetText completes in bounded time under each pathological
// shape so a future refactor can't silently regress them. Thresholds are
// generous — they only catch true freezes, not minor slowdowns.
// ---------------------------------------------------------------------------

// stressDeadline is the wall-clock budget for any single SetText pathological
// case on a typical dev box. Picked deliberately loose so green CI on a busy
// shared runner doesn't flake — we only want to catch true freezes.
const stressDeadline = 2 * time.Second

// runWithin executes fn and fails the test if it exceeds budget. Returns the
// elapsed duration so a passing case can still log its cost.
func runWithin(t *testing.T, budget time.Duration, label string, fn func()) time.Duration {
	t.Helper()
	start := time.Now()
	fn()
	d := time.Since(start)
	if d > budget {
		t.Fatalf("%s exceeded budget: %v > %v", label, d, budget)
	}
	t.Logf("%s ok in %v", label, d)
	return d
}

// TestSetTextManyLines: SetText with 100k short lines must complete within the
// stress deadline and surface every line through Lines().
func TestSetTextManyLines(t *testing.T) {
	const n = 100000
	var b strings.Builder
	b.Grow(n * 4)
	for i := 0; i < n; i++ {
		b.WriteString("x\n")
	}
	src := b.String()

	e := NewCodeEditor()
	runWithin(t, stressDeadline, "SetText(100k lines)", func() {
		e.SetText(src)
	})
	// strings.Split on a trailing "\n" yields n+1 entries (the last empty).
	if got, want := len(e.Lines()), n+1; got != want {
		t.Fatalf("Lines() = %d, want %d", got, want)
	}
}

// TestSetTextOneVeryLongLine: a single 100k-char line must load fast and stay a
// single line. Highlighting on that line should skip tokenization (guarded).
func TestSetTextOneVeryLongLine(t *testing.T) {
	src := strings.Repeat("a", 100000)

	e := NewCodeEditor()
	runWithin(t, stressDeadline, "SetText(1x100k-char line)", func() {
		e.SetText(src)
	})
	if got := len(e.Lines()); got != 1 {
		t.Fatalf("Lines() = %d, want 1", got)
	}
}

// TestSetTextManyNestedBraces: 10k opening then 10k closing braces must load
// without panic. fold-region computation may either complete in bounded time or
// be skipped by the file-size guard; either is acceptable. The point is no
// freeze and no stack blow-up.
func TestSetTextManyNestedBraces(t *testing.T) {
	const depth = 10000
	lines := make([]string, 0, depth*2)
	for i := 0; i < depth; i++ {
		lines = append(lines, "{")
	}
	for i := 0; i < depth; i++ {
		lines = append(lines, "}")
	}
	src := strings.Join(lines, "\n")

	e := NewCodeEditor()
	runWithin(t, stressDeadline, "SetText(10k nested braces)", func() {
		e.SetText(src)
	})
	// Touch the fold path explicitly so we exercise computeFoldRegions on the
	// post-set state. Either it returns a region or nil (above the threshold);
	// no panic is the assertion.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("FoldRegions panicked on deeply-nested input: %v", r)
		}
	}()
	runWithin(t, stressDeadline, "FoldRegions(10k nested braces)", func() {
		_ = e.FoldRegions()
	})
}

// TestTokenizeLineSkipsLongLine: a line at or above maxHighlightLineLength must
// fall back to a single plain-text (or comment, when inBlock) token. No
// keyword/string/comment lexing happens on the giant line.
func TestTokenizeLineSkipsLongLine(t *testing.T) {
	// Build a line that, if tokenized, would produce many keyword tokens.
	long := strings.Repeat("func ", (maxHighlightLineLength/5)+1)
	if len(long) < maxHighlightLineLength {
		t.Fatalf("test input too short: len=%d, want >= %d", len(long), maxHighlightLineLength)
	}

	tokens, inBlock := tokenizeLine(long, false)
	if inBlock {
		t.Fatalf("inBlock should stay false on plain long line, got true")
	}
	if len(tokens) != 1 {
		t.Fatalf("want a single fallback token, got %d: %v", len(tokens), tokens)
	}
	if tokens[0].typ != tokNormal {
		t.Fatalf("fallback token type = %d, want tokNormal (%d)", tokens[0].typ, tokNormal)
	}
	if tokens[0].text != long {
		t.Fatalf("fallback token text length = %d, want %d (whole line)", len(tokens[0].text), len(long))
	}

	// When the line begins inside a block comment, the fallback must keep it a
	// comment token and preserve inBlock=true for subsequent lines.
	tokens, inBlock = tokenizeLine(long, true)
	if !inBlock {
		t.Fatalf("inBlock should stay true on long line that started inside a comment")
	}
	if len(tokens) != 1 || tokens[0].typ != tokComment {
		t.Fatalf("want single comment fallback, got %+v", tokens)
	}
}

// TestComputeFoldRegionsLargeFileLimit: pure test of the escape hatch. Above
// the threshold computeFoldRegions returns nil; below it returns a non-nil
// (possibly empty) slice computed normally.
func TestComputeFoldRegionsLargeFileLimit(t *testing.T) {
	// Above the threshold: nil.
	huge := make([]string, maxFoldComputeLines+1)
	for i := range huge {
		huge[i] = "x"
	}
	if got := computeFoldRegions(huge); got != nil {
		t.Fatalf("computeFoldRegions above threshold should return nil, got %d regions", len(got))
	}

	// Below the threshold with a real foldable block: non-nil result.
	small := []string{"func f() {", "\tx := 1", "}"}
	regs := computeFoldRegions(small)
	if regs == nil {
		t.Fatalf("computeFoldRegions below threshold should return non-nil, got nil")
	}
	if len(regs) != 1 || regs[0].startLine != 0 || regs[0].endLine != 2 {
		t.Fatalf("computeFoldRegions(small) = %v, want one region 0..2", regs)
	}
}
