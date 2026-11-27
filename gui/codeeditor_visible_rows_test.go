package gui

import (
	"slices"
	"strings"
	"testing"
)

func TestEditorUnfoldedRowsReuseIdentityAfterTyping(t *testing.T) {
	e := NewCodeEditor()
	defer e.CancelSymbolIndex()
	e.SetSize(1000, 700)
	e.SetText("package p\n" + strings.Repeat("// unchanged\n", 100_000) + "var Bottom = 1\n")
	e.ScrollToLineCol(100_001, 14)
	first := e.visibleLineIndices()
	e.OnTextInput(" ")
	next := e.visibleLineIndices()
	if len(next) != len(e.lines) || next[len(next)-1] != len(e.lines)-1 || &next[0] != &first[0] {
		t.Fatal("ordinary typing rebuilt the unfolded index")
	}
	e.Undo()
	if restored := e.visibleLineIndices(); &restored[0] != &first[0] {
		t.Fatal("undo rebuilt the unfolded index")
	}
	if allocs := testing.AllocsPerRun(12, func() {
		e.invalidateFoldCache()
		_ = e.visibleLineIndices()
	}); allocs != 0 || e.foldRegionsValid {
		t.Fatalf("unfolded index should require neither an allocation nor a fold scan: allocs=%v, scanned=%v", allocs, e.foldRegionsValid)
	}
}

func TestEditorVisibleRowsKeepFoldAndResizeMapping(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\nfunc f() {\n\tif true {\n\t\tprintln(1)\n\t}\n}\n// tail")
	e.ToggleFold(2)
	if got := e.visibleLineIndices(); !slices.Equal(got, []int{0, 1, 2, 5, 6}) {
		t.Fatalf("nested collapsed indices = %v", got)
	}
	e.cursorLine, e.cursorCol = 4, 1
	e.clampCursorVisible()
	if e.cursorLine != 2 || e.lineToVisualRow(5) != 3 {
		t.Fatal("folded caret did not map to its visible header or next row")
	}
	e.UnfoldAll()
	if got := e.visibleLineIndices(); !slices.Equal(got, []int{0, 1, 2, 3, 4, 5, 6}) {
		t.Fatalf("unfolded indices = %v", got)
	}
	e.SetText("x\ny")
	if got := e.visibleLineIndices(); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("visible index reused the wrong document size: %v", got)
	}
	if e.lineToVisualRow(-1) != 0 || e.lineToVisualRow(99) != 1 {
		t.Fatal("out-of-range source coordinates were not clamped")
	}
}
