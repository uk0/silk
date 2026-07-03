package gui

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Code folding (pure helpers + UI/state API, no GL / no Draw)
//
// computeFoldRegions and visibleLines are pure: they take a line slice / fold
// state and return plain data, so the folding model is unit-testable without a
// graphics context. The public API (ToggleFold / IsFolded / FoldRegions) is the
// UI/state layer, keyed 0-based like breakpoints and bookmarks.
// ---------------------------------------------------------------------------

func regions(rs ...[2]int) []foldRegion {
	out := make([]foldRegion, len(rs))
	for i, r := range rs {
		out[i] = foldRegion{startLine: r[0], endLine: r[1]}
	}
	return out
}

func TestComputeFoldRegionsSimple(t *testing.T) {
	lines := []string{
		"func main() {", // 0  opens
		"\tx := 1",      // 1
		"\ty := 2",      // 2
		"}",             // 3  closes -> region 0..3
		"",              // 4
	}
	got := computeFoldRegions(lines)
	want := regions([2]int{0, 3})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("computeFoldRegions = %v, want %v", got, want)
	}
}

func TestComputeFoldRegionsNested(t *testing.T) {
	lines := []string{
		"func f() {", // 0 opens outer
		"\tif a {",   // 1 opens inner
		"\t\tg()",    // 2
		"\t}",        // 3 closes inner -> 1..3
		"\treturn",   // 4
		"}",          // 5 closes outer -> 0..5
	}
	got := computeFoldRegions(lines)
	// Ordered by start line: outer first, then inner.
	want := regions([2]int{0, 5}, [2]int{1, 3})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nested computeFoldRegions = %v, want %v", got, want)
	}
}

func TestComputeFoldRegionsBraceElseBrace(t *testing.T) {
	// A "} else {" line both closes one block and opens the next.
	lines := []string{
		"if a {",   // 0 opens A
		"\tx()",    // 1
		"} else {", // 2 closes A (0..2), opens B at depth 0
		"\ty()",    // 3
		"}",        // 4 closes B -> 2..4
	}
	got := computeFoldRegions(lines)
	want := regions([2]int{0, 2}, [2]int{2, 4})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("brace-else-brace computeFoldRegions = %v, want %v", got, want)
	}
}

func TestComputeFoldRegionsSingleLineNotFoldable(t *testing.T) {
	// A block that opens and closes on the same line is not a foldable region.
	lines := []string{
		"x := struct{}{}", // 0 - braces balanced on one line
		"y := 1",          // 1
	}
	got := computeFoldRegions(lines)
	if len(got) != 0 {
		t.Errorf("single-line braces should yield no regions, got %v", got)
	}
}

func TestComputeFoldRegionsUnbalancedNoPanic(t *testing.T) {
	// Unbalanced braces must be handled without panic and without inventing a
	// region that never closes.
	cases := [][]string{
		{"func f() {", "\tx()"},  // opener, no closer
		{"\t}", "}", "}"},        // only closers
		{"a {", "b {", "c {"},    // three openers, no closers
		{"}", "a {", "\tx", "}"}, // stray closer then a real region
		{},                       // empty input
		{""},                     // single blank line
	}
	for i, lines := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d: computeFoldRegions panicked: %v", i, r)
				}
			}()
			_ = computeFoldRegions(lines)
		}()
	}

	// The "stray closer then a real region" case should still find the region.
	got := computeFoldRegions([]string{"}", "a {", "\tx", "}"})
	want := regions([2]int{1, 3})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stray-closer case = %v, want %v", got, want)
	}
}

func TestVisibleLinesNoFold(t *testing.T) {
	regs := regions([2]int{0, 3})
	got := visibleLines(5, map[int]bool{}, regs)
	want := []int{0, 1, 2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("no fold visibleLines = %v, want %v", got, want)
	}
}

func TestVisibleLinesFoldHidesBody(t *testing.T) {
	// Region 0..3 folded: lines 1,2,3 hidden, start line 0 stays.
	regs := regions([2]int{0, 3})
	got := visibleLines(5, map[int]bool{0: true}, regs)
	want := []int{0, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("folded visibleLines = %v, want %v", got, want)
	}
}

func TestVisibleLinesNestedInnerFold(t *testing.T) {
	// Outer 0..5, inner 1..3. Folding only the inner hides 2,3.
	regs := regions([2]int{0, 5}, [2]int{1, 3})
	got := visibleLines(6, map[int]bool{1: true}, regs)
	want := []int{0, 1, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("inner-fold visibleLines = %v, want %v", got, want)
	}
}

func TestVisibleLinesNestedOuterFold(t *testing.T) {
	// Folding the outer region hides 1..5 regardless of the inner fold state;
	// the inner region's start (line 1) is itself hidden by the outer fold.
	regs := regions([2]int{0, 5}, [2]int{1, 3})
	got := visibleLines(6, map[int]bool{0: true, 1: true}, regs)
	want := []int{0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("outer-fold visibleLines = %v, want %v", got, want)
	}
}

func TestCodeEditorToggleFold(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("func main() {\n\tx := 1\n\ty := 2\n}\n")

	// FoldRegions detects the single brace block 0..3.
	regs := e.FoldRegions()
	if !reflect.DeepEqual(regs, regions([2]int{0, 3})) {
		t.Fatalf("FoldRegions = %v, want [{0 3}]", regs)
	}

	if e.IsFolded(0) {
		t.Errorf("region should start unfolded")
	}

	// Folding the region hides its body: visible-row count drops by the body
	// size (end - start == 3 lines hidden).
	totalRows := len(e.visibleLineIndices())
	e.ToggleFold(0)
	if !e.IsFolded(0) {
		t.Errorf("ToggleFold(0) should fold the region")
	}
	foldedRows := len(e.visibleLineIndices())
	if drop := totalRows - foldedRows; drop != 3 {
		t.Errorf("folding region 0..3 should hide 3 rows, dropped %d (rows %d -> %d)", drop, totalRows, foldedRows)
	}

	// Unfolding restores all rows.
	e.ToggleFold(0)
	if e.IsFolded(0) {
		t.Errorf("second ToggleFold(0) should unfold")
	}
	if got := len(e.visibleLineIndices()); got != totalRows {
		t.Errorf("after unfold visible rows = %d, want %d", got, totalRows)
	}
}

func TestCodeEditorToggleFoldNonStartIgnored(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("func main() {\n\tx := 1\n}\n")

	// Line 1 is not the start of any foldable region; toggling it is a no-op.
	before := len(e.visibleLineIndices())
	e.ToggleFold(1)
	if e.IsFolded(1) {
		t.Errorf("ToggleFold on a non-start line should not fold")
	}
	if got := len(e.visibleLineIndices()); got != before {
		t.Errorf("ToggleFold on a non-start line changed visible rows %d -> %d", before, got)
	}
}

func TestCodeEditorFoldAllUnfoldAll(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("func f() {\n\tif a {\n\t\tg()\n\t}\n\treturn\n}\n")

	// Two regions: outer 0..5, inner 1..3.
	if got := len(e.FoldRegions()); got != 2 {
		t.Fatalf("want 2 fold regions, got %d (%v)", got, e.FoldRegions())
	}

	full := len(e.visibleLineIndices())
	e.FoldAll()
	if !e.IsFolded(0) || !e.IsFolded(1) {
		t.Errorf("FoldAll should fold every region, folded=%v", e.foldedLines)
	}
	// With the outer region folded, only the outer start line (0) plus the
	// trailing blank line (6) remain visible.
	if got := len(e.visibleLineIndices()); got != 2 {
		t.Errorf("after FoldAll visible rows = %d, want 2", got)
	}

	e.UnfoldAll()
	if e.IsFolded(0) || e.IsFolded(1) {
		t.Errorf("UnfoldAll should clear all folds, folded=%v", e.foldedLines)
	}
	if got := len(e.visibleLineIndices()); got != full {
		t.Errorf("after UnfoldAll visible rows = %d, want %d", got, full)
	}
}

func TestCodeEditorFoldCursorClampedOutOfHiddenBody(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("func main() {\n\tx := 1\n\ty := 2\n}\n")

	// Put the cursor inside the body, then fold: the caret must be pulled back to
	// the visible fold-header line so it never sits on a hidden line.
	e.cursorLine = 2
	e.ToggleFold(0)
	if e.cursorLine != 0 {
		t.Errorf("after folding, cursor on hidden line 2 should move to header 0, got %d", e.cursorLine)
	}
}

// TestCodeEditorFoldGutterClick verifies a click on the fold marker (right strip
// of the gutter) toggles the fold on a foldable start line, while a gutter click
// elsewhere toggles the breakpoint instead. Click Y uses the editor's own font
// metrics, matching posFromXY's row math.
func TestCodeEditorFoldGutterClick(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText("func main() {\n\tx := 1\n\ty := 2\n}\n")

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()
	yForRow := func(row int) float64 { return topOff + float64(row)*lh + lh/2 }

	// Sanity: row 0 resolves to line 0 (the foldable start).
	if got, _ := e.posFromXY(5, yForRow(0)); got != 0 {
		t.Fatalf("posFromXY resolved line %d, want 0 (font metrics mismatch)", got)
	}

	// Click the fold marker (right strip: x in [gutterW-12, gutterW)) on line 0.
	foldX := e.gutterW - 6
	e.OnLeftDown(foldX, yForRow(0))
	if !e.IsFolded(0) {
		t.Errorf("fold-marker click on line 0 should fold the region")
	}
	if len(e.Breakpoints()) != 0 {
		t.Errorf("fold-marker click must not set a breakpoint, got %v", e.Breakpoints())
	}

	// Clicking it again unfolds.
	e.OnLeftDown(foldX, yForRow(0))
	if e.IsFolded(0) {
		t.Errorf("second fold-marker click on line 0 should unfold")
	}

	// A click in the left part of the gutter on a foldable line toggles the
	// breakpoint, not the fold.
	bpX := 4.0
	e.OnLeftDown(bpX, yForRow(0))
	if e.IsFolded(0) {
		t.Errorf("left-gutter click should not fold")
	}
	if !e.breakpoints[0] {
		t.Errorf("left-gutter click on line 0 should set a breakpoint")
	}
}
