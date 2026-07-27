package gui

import (
	"strings"
	"testing"
)

// Everything here is pure model code: WrapLayout and ScanGoFoldRegions take
// strings and return structs, so no Frame, no font and no graphics context is
// ever constructed.

// ---------------------------------------------------------------------------
// Fold regions
// ---------------------------------------------------------------------------

// foldSrc joins numbered lines so each test case reads like the buffer it
// models and the expected 0-based line numbers can be counted off directly.
func foldSrc(lines ...string) string { return strings.Join(lines, "\n") }

func foldsEqual(a, b []SyntaxFoldRegion) bool {
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

// TestScanGoFoldRegionsIgnoresLiteralBraces: a '{' inside a line comment, a
// string literal or a rune literal must not open or close a block. The old
// rune-counting scan mis-paired all three.
func TestScanGoFoldRegionsIgnoresLiteralBraces(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",      //  0
		"",               //  1
		"// func f() {",  //  2
		"func g() {",     //  3
		"\ts := \"{{{\"", //  4
		"\tc := '}'",     //  5
		"\t_ = s",        //  6
		"\t_ = c",        //  7
		"}",              //  8
	))
	want := []SyntaxFoldRegion{{StartLine: 3, EndLine: 8, Kind: FoldKindBlock}}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsBlockCommentBraces: braces inside a /* */ comment are
// invisible, and the comment itself folds because it spans lines.
func TestScanGoFoldRegionsBlockCommentBraces(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",         // 0
		"",                  // 1
		"/*",                // 2
		"if x { } else { }", // 3
		"*/",                // 4
		"func f() {",        // 5
		"\tprintln(\"}\")",  // 6
		"}",                 // 7
	))
	want := []SyntaxFoldRegion{
		{StartLine: 2, EndLine: 4, Kind: FoldKindComment},
		{StartLine: 5, EndLine: 7, Kind: FoldKindBlock},
	}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsRawStringBraces: a raw string spans lines and may carry
// braces; neither the braces nor the line span may confuse the scan.
func TestScanGoFoldRegionsRawStringBraces(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",  // 0
		"",           // 1
		"func f() {", // 2
		"\ts := `",   // 3
		"{",          // 4
		"{",          // 5
		"`",          // 6
		"\t_ = s",    // 7
		"}",          // 8
	))
	want := []SyntaxFoldRegion{{StartLine: 2, EndLine: 8, Kind: FoldKindBlock}}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsImportBlock: an import group folds; the one-line func
// body does not (single-line spans are never foldable), and the call parens
// inside it must not be mistaken for the import's closer.
func TestScanGoFoldRegionsImportBlock(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",                            // 0
		"",                                     // 1
		"import (",                             // 2
		"\t\"fmt\"",                            // 3
		"\t\"os\"",                             // 4
		")",                                    // 5
		"",                                     // 6
		"func main() { fmt.Println(os.Args) }", // 7
	))
	want := []SyntaxFoldRegion{{StartLine: 2, EndLine: 5, Kind: FoldKindImport}}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsSingleImportNotFoldable: a parenless import has nothing
// to fold, and a non-import paren group is not an import fold either.
func TestScanGoFoldRegionsSingleImportNotFoldable(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",           // 0
		"",                    // 1
		"import \"fmt\"",      // 2
		"",                    // 3
		"var _ = fmt.Sprint(", // 4
		"\t1,",                // 5
		")",                   // 6
	))
	if len(got) != 0 {
		t.Fatalf("ScanGoFoldRegions = %v, want no regions", got)
	}
}

// TestScanGoFoldRegionsCommentRuns: consecutive whole-line comments fold as one
// region; a lone comment line, a blank-separated pair and a comment trailing
// real code do not start one.
func TestScanGoFoldRegionsCommentRuns(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",             //  0
		"",                      //  1
		"// first",              //  2
		"// second",             //  3
		"// third",              //  4
		"",                      //  5
		"// lonely",             //  6
		"",                      //  7
		"var x = 1 // trailing", //  8
		"// after code",         //  9
		"// still after",        // 10
	))
	want := []SyntaxFoldRegion{
		{StartLine: 2, EndLine: 4, Kind: FoldKindComment},
		{StartLine: 9, EndLine: 10, Kind: FoldKindComment},
	}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsNested: nested blocks all fold, ordered outermost first.
func TestScanGoFoldRegionsNested(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",                    // 0
		"",                             // 1
		"func f() {",                   // 2
		"\tif true {",                  // 3
		"\t\tfor i := 0; i < 3; i++ {", // 4
		"\t\t\t_ = i",                  // 5
		"\t\t}",                        // 6
		"\t}",                          // 7
		"}",                            // 8
	))
	want := []SyntaxFoldRegion{
		{StartLine: 2, EndLine: 8, Kind: FoldKindBlock},
		{StartLine: 3, EndLine: 7, Kind: FoldKindBlock},
		{StartLine: 4, EndLine: 6, Kind: FoldKindBlock},
	}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsMidLineBrace: a '{' that is not the last thing on its
// line still opens a block. computeFoldRegions required a trailing brace and
// missed multi-line composite literals entirely.
func TestScanGoFoldRegionsMidLineBrace(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",                        // 0
		"",                                 // 1
		"var m = map[string]int{\"a\": 1,", // 2
		"\t\"b\": 2}",                      // 3
	))
	want := []SyntaxFoldRegion{{StartLine: 2, EndLine: 3, Kind: FoldKindBlock}}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsOrdersOuterFirst: two blocks opening on the same line
// are reported outermost first, so the order is deterministic for a gutter that
// draws one marker per start line.
func TestScanGoFoldRegionsOrdersOuterFirst(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",         // 0
		"",                  // 1
		"func f() { if x {", // 2
		"\t\ta()",           // 3
		"\t}",               // 4
		"}",                 // 5
	))
	want := []SyntaxFoldRegion{
		{StartLine: 2, EndLine: 5, Kind: FoldKindBlock},
		{StartLine: 2, EndLine: 4, Kind: FoldKindBlock},
	}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsBraceElseBrace: "} else {" closes one block and opens
// the next on the same line.
func TestScanGoFoldRegionsBraceElseBrace(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"package p",  // 0
		"",           // 1
		"func f() {", // 2
		"\tif x {",   // 3
		"\t\ta()",    // 4
		"\t} else {", // 5
		"\t\tb()",    // 6
		"\t}",        // 7
		"}",          // 8
	))
	want := []SyntaxFoldRegion{
		{StartLine: 2, EndLine: 8, Kind: FoldKindBlock},
		{StartLine: 3, EndLine: 5, Kind: FoldKindBlock},
		{StartLine: 5, EndLine: 7, Kind: FoldKindBlock},
	}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsUnterminatedString: a half-typed literal is the normal
// state of an open editor. The scan error is swallowed and the enclosing block
// is still reported.
func TestScanGoFoldRegionsUnterminatedString(t *testing.T) {
	got := ScanGoFoldRegions(foldSrc(
		"func f() {",            // 0
		"\ts := \"unterminated", // 1
		"}",                     // 2
	))
	want := []SyntaxFoldRegion{{StartLine: 0, EndLine: 2, Kind: FoldKindBlock}}
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegions = %v, want %v", got, want)
	}
}

// TestScanGoFoldRegionsGarbageNoPanic: unbalanced, non-Go and outright binary
// input must return whatever was found instead of panicking.
func TestScanGoFoldRegionsGarbageNoPanic(t *testing.T) {
	cases := []string{
		"",
		"}",
		"{",
		"{{{{",
		"}}}}",
		"/*",
		"'",
		"`",
		"@#$%^&*",
		"\x00\x01\x02",
		"import (",
		"// c",
		"func f() {\n",
	}
	for i, in := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d (%q): panicked: %v", i, in, r)
				}
			}()
			_ = ScanGoFoldRegions(in)
		}()
	}
}

// TestScanGoFoldRegionsLines: the line-buffer entry point matches the joined
// source, and honours the same large-file guard as computeFoldRegions.
func TestScanGoFoldRegionsLines(t *testing.T) {
	lines := []string{"package p", "", "func f() {", "\t_ = 1", "}"}
	got := ScanGoFoldRegionsLines(lines)
	want := ScanGoFoldRegions(strings.Join(lines, "\n"))
	if !foldsEqual(got, want) {
		t.Fatalf("ScanGoFoldRegionsLines = %v, want %v", got, want)
	}
	if len(got) != 1 || got[0].Kind != FoldKindBlock {
		t.Fatalf("ScanGoFoldRegionsLines = %v, want one block region", got)
	}

	huge := make([]string, maxFoldComputeLines)
	for i := range huge {
		huge[i] = "{"
	}
	if regs := ScanGoFoldRegionsLines(huge); regs != nil {
		t.Fatalf("above the guard threshold want nil, got %d regions", len(regs))
	}
}

// ---------------------------------------------------------------------------
// Wrap layout
// ---------------------------------------------------------------------------

// rowTexts renders the layout as one string per visual row, which is the
// clearest way to assert a wrap decision.
func rowTexts(w *WrapLayout) []string {
	out := make([]string, w.RowCount())
	for i := range out {
		out[i] = w.RowText(i)
	}
	return out
}

func sameRows(a, b []string) bool {
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

// TestWrapLayoutWordBoundary: the break lands after a space, never mid-word,
// and the space stays on the row it ended.
func TestWrapLayoutWordBoundary(t *testing.T) {
	w := NewWrapLayout([]string{"hello world foo"}, 11, nil)
	want := []string{"hello world ", "foo"}
	if got := rowTexts(w); !sameRows(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	if w.RowCount() != 2 {
		t.Errorf("RowCount = %d, want 2", w.RowCount())
	}
	if first, count := w.RowsForLine(0); first != 0 || count != 2 {
		t.Errorf("RowsForLine(0) = (%d,%d), want (0,2)", first, count)
	}
}

// TestWrapLayoutHardBreakLongToken: a token with no break opportunity is cut at
// the margin instead of overflowing.
func TestWrapLayoutHardBreakLongToken(t *testing.T) {
	w := NewWrapLayout([]string{"aaaaaaaaaa"}, 4, nil)
	want := []string{"aaaa", "aaaa", "aa"}
	if got := rowTexts(w); !sameRows(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
}

// TestWrapLayoutWordLongerThanWidth: a word wider than the whole view falls back
// to hard breaks while the short word before it still wraps at its boundary.
func TestWrapLayoutWordLongerThanWidth(t *testing.T) {
	w := NewWrapLayout([]string{"ab cdefghijkl"}, 5, nil)
	want := []string{"ab ", "cdefg", "hijkl"}
	if got := rowTexts(w); !sameRows(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	// Every row must carry at least one rune, or the layout would not terminate.
	for i := 0; i < w.RowCount(); i++ {
		r, _ := w.Row(i)
		if r.EndCol <= r.StartCol {
			t.Fatalf("row %d is empty: %+v", i, r)
		}
	}
}

// TestWrapLayoutRuneColumnsNotBytes: columns are rune offsets, so multi-byte
// text wraps by character count and RowText slices cleanly.
func TestWrapLayoutRuneColumnsNotBytes(t *testing.T) {
	w := NewWrapLayout([]string{"日本語テキスト"}, 3, nil)
	want := []string{"日本語", "テキス", "ト"}
	if got := rowTexts(w); !sameRows(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	if row, col := w.SourceToVisual(0, 4); row != 1 || col != 1 {
		t.Errorf("SourceToVisual(0,4) = (%d,%d), want (1,1)", row, col)
	}
}

// TestWrapLayoutEmptyLineKeepsARow: a blank line still occupies one visual row,
// otherwise row indices would drift away from the buffer.
func TestWrapLayoutEmptyLineKeepsARow(t *testing.T) {
	w := NewWrapLayout([]string{"", "abc", ""}, 2, nil)
	want := []string{"", "ab", "c", ""}
	if got := rowTexts(w); !sameRows(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	if first, count := w.RowsForLine(1); first != 1 || count != 2 {
		t.Errorf("RowsForLine(1) = (%d,%d), want (1,2)", first, count)
	}
	if first, count := w.RowsForLine(2); first != 3 || count != 1 {
		t.Errorf("RowsForLine(2) = (%d,%d), want (3,1)", first, count)
	}
}

// TestWrapLayoutWrapOff: a non-positive width means one row per source line —
// the same mapping serves a non-wrapping view.
func TestWrapLayoutWrapOff(t *testing.T) {
	lines := []string{"a very long line that would otherwise wrap", "x"}
	w := NewWrapLayout(lines, 0, nil)
	if w.RowCount() != len(lines) {
		t.Fatalf("RowCount = %d, want %d", w.RowCount(), len(lines))
	}
	if got := rowTexts(w); !sameRows(got, lines) {
		t.Fatalf("rows = %q, want %q", got, lines)
	}
}

// TestWrapLayoutInjectedWidth: the width function drives every break decision,
// which is what lets a real font replace the character-cell default later.
func TestWrapLayoutInjectedWidth(t *testing.T) {
	double := func(rune) float64 { return 2 }
	w := NewWrapLayout([]string{"abcdef"}, 4, double)
	want := []string{"ab", "cd", "ef"}
	if got := rowTexts(w); !sameRows(got, want) {
		t.Fatalf("double-width rows = %q, want %q", got, want)
	}

	// A single rune wider than the whole row still gets a row of its own.
	wide := func(rune) float64 { return 3 }
	w = NewWrapLayout([]string{"abc"}, 2, wide)
	want = []string{"a", "b", "c"}
	if got := rowTexts(w); !sameRows(got, want) {
		t.Fatalf("over-wide rows = %q, want %q", got, want)
	}
}

// TestWrapLayoutBoundaryUsesContinuationRow: the column at a wrap boundary is
// reported on the row that continues the line, at offset 0.
func TestWrapLayoutBoundaryUsesContinuationRow(t *testing.T) {
	w := NewWrapLayout([]string{"abcdef"}, 3, nil)
	if row, col := w.SourceToVisual(0, 2); row != 0 || col != 2 {
		t.Errorf("SourceToVisual(0,2) = (%d,%d), want (0,2)", row, col)
	}
	if row, col := w.SourceToVisual(0, 3); row != 1 || col != 0 {
		t.Errorf("SourceToVisual(0,3) = (%d,%d), want (1,0)", row, col)
	}
	// End of line: the last row, one past its last rune.
	if row, col := w.SourceToVisual(0, 6); row != 1 || col != 3 {
		t.Errorf("SourceToVisual(0,6) = (%d,%d), want (1,3)", row, col)
	}
}

// TestWrapLayoutRoundTrip: every in-range source position survives a trip
// through visual space unchanged.
func TestWrapLayoutRoundTrip(t *testing.T) {
	lines := []string{
		"",
		"short",
		"the quick brown fox jumps over the lazy dog",
		"averyveryverylongtokenwithnobreaks",
		"tab\tseparated words here",
		"日本語のテキストも折り返す",
		"trailing spaces    ",
	}
	for _, width := range []float64{1, 3, 7, 12, 40} {
		w := NewWrapLayout(lines, width, nil)
		for line, text := range lines {
			n := len([]rune(text))
			for col := 0; col <= n; col++ {
				row, rowCol := w.SourceToVisual(line, col)
				gotLine, gotCol := w.VisualToSource(row, rowCol)
				if gotLine != line || gotCol != col {
					t.Fatalf("width %v: (%d,%d) -> row (%d,%d) -> (%d,%d)",
						width, line, col, row, rowCol, gotLine, gotCol)
				}
			}
		}
		// Rows must partition each line exactly, with no gap or overlap.
		for line, text := range lines {
			first, count := w.RowsForLine(line)
			if count < 1 {
				t.Fatalf("width %v: line %d has %d rows", width, line, count)
			}
			next := 0
			for i := first; i < first+count; i++ {
				r, ok := w.Row(i)
				if !ok || r.Line != line || r.StartCol != next {
					t.Fatalf("width %v: row %d = %+v (ok=%v), want line %d starting at %d",
						width, i, r, ok, line, next)
				}
				next = r.EndCol
			}
			if next != len([]rune(text)) {
				t.Fatalf("width %v: line %d rows end at %d, want %d", width, line, next, len([]rune(text)))
			}
		}
	}
}

// TestWrapLayoutOutOfRange: every accessor clamps or reports failure instead of
// panicking, including on an empty buffer.
func TestWrapLayoutOutOfRange(t *testing.T) {
	w := NewWrapLayout([]string{"ab", "cd"}, 1, nil)
	if row, col := w.SourceToVisual(-5, -5); row != 0 || col != 0 {
		t.Errorf("SourceToVisual(-5,-5) = (%d,%d), want (0,0)", row, col)
	}
	if row, col := w.SourceToVisual(99, 99); row != w.RowCount()-1 || col != 1 {
		t.Errorf("SourceToVisual(99,99) = (%d,%d), want (%d,1)", row, col, w.RowCount()-1)
	}
	if line, col := w.VisualToSource(99, 99); line != 1 || col != 2 {
		t.Errorf("VisualToSource(99,99) = (%d,%d), want (1,2)", line, col)
	}
	if line, col := w.VisualToSource(-1, -1); line != 0 || col != 0 {
		t.Errorf("VisualToSource(-1,-1) = (%d,%d), want (0,0)", line, col)
	}
	if _, ok := w.Row(-1); ok {
		t.Error("Row(-1) reported ok")
	}
	if _, ok := w.Row(w.RowCount()); ok {
		t.Error("Row(RowCount) reported ok")
	}
	if got := w.RowText(w.RowCount()); got != "" {
		t.Errorf("RowText(RowCount) = %q, want \"\"", got)
	}
	if first, count := w.RowsForLine(7); first != 0 || count != 0 {
		t.Errorf("RowsForLine(7) = (%d,%d), want (0,0)", first, count)
	}

	empty := NewWrapLayout(nil, 10, nil)
	if empty.RowCount() != 0 || empty.LineCount() != 0 {
		t.Fatalf("empty layout: RowCount=%d LineCount=%d, want 0/0", empty.RowCount(), empty.LineCount())
	}
	if row, col := empty.SourceToVisual(0, 0); row != 0 || col != 0 {
		t.Errorf("empty SourceToVisual = (%d,%d), want (0,0)", row, col)
	}
	if line, col := empty.VisualToSource(0, 0); line != 0 || col != 0 {
		t.Errorf("empty VisualToSource = (%d,%d), want (0,0)", line, col)
	}
	if got := empty.RowText(0); got != "" {
		t.Errorf("empty RowText = %q, want \"\"", got)
	}
}
