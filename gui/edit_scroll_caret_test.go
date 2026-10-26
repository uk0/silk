package gui

import (
	"fmt"
	"testing"
)

// A property row can be a fraction of a pixel shorter than font + padding.
// Counting zero visible rows used to scroll the only line out of view on
// mouse-up, then back on the next arrow key (also affected ordinary Edits).
func TestSingleLineEditCaretNeverScrollsVertically(t *testing.T) {
	for _, below := range []float64{.5, 3, 20} {
		t.Run(fmt.Sprint(below), func(t *testing.T) {
			e := NewEdit()
			e.SetSize(118, e.RowHeight()+e.padding.T+e.padding.B-below)
			e.SetText("66.12345678901234")
			for i := 0; i < 4; i++ {
				e.OnLeftDown(e.padding.L+10, e.padding.T+2)
				e.OnLeftUp(e.padding.L+10, e.padding.T+2)
				if got := e.ScrollY(); got != 0 {
					t.Fatalf("click %d hid the single line: scrollY=%g viewport=%g rowHeight=%g", i, got, e.Height()-e.padding.T-e.padding.B, e.RowHeight())
				}
				e.OnKeyDown(KeyRight, false)
				e.SelectAll()
				if e.ScrollY() != 0 || e.SelectionText() != "66.12345678901234" {
					t.Fatal("caret/selection lost the only visible line")
				}
			}
			e.SetScrollY(1) // stale scroll after switching from a multiline editor
			e.ScrollToCaret()
			if e.ScrollY() != 0 {
				t.Fatal("stale single-line scroll not cleared")
			}
		})
	}
}

func TestTinyMultilineEditCaretKeepsAtLeastOneRow(t *testing.T) {
	e := NewEdit()
	e.SetMultiLine(true)
	e.SetSize(140, e.RowHeight()+e.padding.T+e.padding.B-.5)
	e.SetText("first\nsecond\nthird")
	for _, row := range []int{0, 1, 2, 0} {
		e.SetCaretRowCol(row, 1)
		for i := 0; i < 3; i++ {
			e.ScrollToCaret()
			if got := e.ScrollY(); got != float64(row) {
				t.Fatalf("row %d scroll=%g", row, got)
			}
		}
	}
}
