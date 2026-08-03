package gui

import "testing"

// TestTreeViewRowIndexAtRowBoundary: rowBottom(i) is the exclusive bottom of
// row i, so row i owns [ypos, rowBottom) and the pixel at rowBottom(i) belongs
// to row i+1. The search predicate used >= and therefore handed every exact row
// boundary to the row above — one click in every rowHeight lands one row high.
func TestTreeViewRowIndexAtRowBoundary(t *testing.T) {
	tv := newKbTree() // 4 uniform rows: A, A0, A1, B
	rh := tv.defRowHeight
	if len(tv.rows) != 4 {
		t.Fatalf("fixture: %d rows, want 4", len(tv.rows))
	}

	for r := 0; r < len(tv.rows); r++ {
		top := tv.rows[r].ypos
		for _, y := range []float64{top, top + 0.5, top + rh - 0.5} {
			if got := tv.rowIndexAtScrolled(y); got != r {
				t.Errorf("rowIndexAtScrolled(%v) = %d, want %d (row %d covers [%v,%v))",
					y, got, r, r, top, top+rh)
			}
		}
	}
}

// TestTreeViewCollapseMovesCurrentRowOut: collapsing a node that contains the
// current row must move the current row onto the collapsed node (as Qt does).
// Left alone, currentRow is still the pre-collapse index and now addresses a
// completely unrelated row — Enter would activate it — or falls off the end of
// the shortened row list entirely.
func TestTreeViewCollapseMovesCurrentRowOut(t *testing.T) {
	tv := newKbTree()
	tv.SetCurrentRow(1)           // A0
	tv.OnKeyDown(KeyRight, false) // expand it: [A A0 A0a A0b A1 B]
	if got, want := rowLabels(tv), []string{"A", "A0", "A0a", "A0b", "A1", "B"}; !eqStrings(got, want) {
		t.Fatalf("setup rows = %v, want %v", got, want)
	}
	tv.SetCurrentRow(3) // A0b, inside A0's subtree

	tv.Collapse(tv.rows[1].mi) // collapse A0

	if got, want := rowLabels(tv), []string{"A", "A0", "A1", "B"}; !eqStrings(got, want) {
		t.Fatalf("after collapse rows = %v, want %v", got, want)
	}
	cur := tv.getRow1(tv.CurrentRow())
	if cur == nil {
		t.Fatalf("current row %d is out of range (%d rows)", tv.CurrentRow(), len(tv.rows))
	}
	if got := tv.getCellText(cur, 0); got != "A0" {
		t.Errorf("current row after collapse = %q, want %q (the collapsed node)", got, "A0")
	}
	if !cur.selected {
		t.Errorf("current row %q is not marked selected", tv.getCellText(cur, 0))
	}
}

// TestTreeViewCollapseClearsHiddenSelection: the hidden rows stay alive in rmap
// and are handed back verbatim on the next expand, so their selected flag has
// to be cleared on the way out. Otherwise re-expanding paints a highlight on a
// row that is not the current row.
func TestTreeViewCollapseClearsHiddenSelection(t *testing.T) {
	tv := newKbTree()
	tv.SetCurrentRow(1)
	tv.OnKeyDown(KeyRight, false) // expand A0
	tv.SetCurrentRow(3)           // A0b

	tv.Collapse(tv.rows[1].mi) // collapse A0 (current row moves to A0)
	tv.OnKeyDown(KeyUp, false) // and the user moves the current row elsewhere
	tv.Expand(tv.rows[1].mi)   // now bring the subtree back

	for i, r := range tv.rows {
		want := i == tv.CurrentRow()
		if r.selected != want {
			t.Errorf("after re-expand row %d %q selected = %v, want %v (current row is %d)",
				i, tv.getCellText(r, 0), r.selected, want, tv.CurrentRow())
		}
	}
}

// TestTreeViewCollapseAboveCurrentShiftsIt: collapsing a node that sits above
// the current row (but does not contain it) removes rows before it, so the
// current row index has to slide up by the same amount to keep pointing at the
// same node.
func TestTreeViewCollapseAboveCurrentShiftsIt(t *testing.T) {
	tv := newKbTree()
	tv.SetCurrentRow(1)
	tv.OnKeyDown(KeyRight, false) // [A A0 A0a A0b A1 B]
	tv.SetCurrentRow(5)           // B, below A0's subtree

	tv.Collapse(tv.rows[1].mi) // collapse A0: two rows disappear above B

	cur := tv.getRow1(tv.CurrentRow())
	if cur == nil {
		t.Fatalf("current row %d is out of range (%d rows)", tv.CurrentRow(), len(tv.rows))
	}
	if got := tv.getCellText(cur, 0); got != "B" {
		t.Errorf("current row after collapse = %q, want %q", got, "B")
	}
}

// TestTreeViewExpandAboveCurrentShiftsIt is the mirror case: expanding a node
// above the current row inserts rows before it. The highlight rides on the row
// object and follows it, so an unshifted currentRow leaves the index and the
// painted selection pointing at different rows.
func TestTreeViewExpandAboveCurrentShiftsIt(t *testing.T) {
	tv := newKbTree()
	tv.SetCurrentRow(3) // B, with A0 still collapsed

	tv.Expand(tv.rows[1].mi) // expand A0: [A A0 A0a A0b A1 B]

	cur := tv.getRow1(tv.CurrentRow())
	if cur == nil {
		t.Fatalf("current row %d is out of range (%d rows)", tv.CurrentRow(), len(tv.rows))
	}
	if got := tv.getCellText(cur, 0); got != "B" {
		t.Errorf("current row after expand = %q, want %q", got, "B")
	}
	if !cur.selected {
		t.Errorf("current row %q is not marked selected", tv.getCellText(cur, 0))
	}
}
