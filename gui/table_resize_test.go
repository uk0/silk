package gui

import (
	"testing"
)

// newResizeTable builds a two-column table (default widths 120 each) with a few
// rows, the fixture shared by the resize tests.
func newResizeTable() (*Table, *SimpleTableModel) {
	tbl := NewTable()
	m := NewSimpleTableModel([]string{"name", "age"})
	m.AddRow("Carol", "30")
	m.AddRow("alice", "25")
	m.AddRow("Bob", "40")
	tbl.SetModel(m)
	return tbl, m
}

// TestColumnBoundaryAt: the boundary hit-test reports the column whose right
// edge is within the grab tolerance, and -1 anywhere in a column body. With two
// 120px columns the boundaries sit at x=120 (col 0) and x=240 (col 1).
func TestColumnBoundaryAt(t *testing.T) {
	tbl, _ := newResizeTable()

	cases := []struct {
		name string
		x    float64
		want int
	}{
		{"left edge of col0 body", 0, -1},
		{"mid col0 body", 60, -1},
		{"on col0 right boundary", 120, 0},
		{"just left of col0 boundary", 120 - columnResizeGrab, 0},
		{"just right of col0 boundary", 120 + columnResizeGrab, 0},
		{"past col0 grab zone", 120 + columnResizeGrab + 1, -1},
		{"mid col1 body", 180, -1},
		{"on col1 right boundary", 240, 1},
		{"past all columns", 400, -1},
	}
	for _, c := range cases {
		if got := tbl.columnBoundaryAt(c.x); got != c.want {
			t.Errorf("%s: columnBoundaryAt(%.0f) = %d, want %d", c.name, c.x, got, c.want)
		}
	}
}

// TestColumnBoundaryAtScrolled: the grab zone tracks the visible boundary, so a
// horizontal scroll shifts where columnBoundaryAt fires by the scroll offset.
func TestColumnBoundaryAtScrolled(t *testing.T) {
	tbl, _ := newResizeTable()
	// Force a horizontal scroll of 30px via the table's scroll area.
	tbl.scrollArea.HorzScrollBar().SetRange(0, 200)
	tbl.scrollArea.HorzScrollBar().SetValue(30)
	if got := tbl.scrollArea.ScrollX(); got != 30 {
		t.Fatalf("scroll setup: ScrollX = %.0f, want 30", got)
	}
	// Col 0's boundary at model-x 120 now appears at widget-x 90.
	if got := tbl.columnBoundaryAt(90); got != 0 {
		t.Errorf("scrolled: columnBoundaryAt(90) = %d, want 0", got)
	}
	// The un-scrolled position 120 is now past the boundary (model-x 150).
	if got := tbl.columnBoundaryAt(120); got != -1 {
		t.Errorf("scrolled: columnBoundaryAt(120) = %d, want -1", got)
	}
}

// TestColumnWidthOverride: columnWidth seeds from the model, setColumnWidth
// overrides it without mutating the model, and the override clamps to the
// minimum.
func TestColumnWidthOverride(t *testing.T) {
	tbl, m := newResizeTable()

	if got := tbl.columnWidth(0); got != 120 {
		t.Errorf("columnWidth(0) seeded = %.0f, want 120 (from model)", got)
	}

	tbl.setColumnWidth(0, 200)
	if got := tbl.columnWidth(0); got != 200 {
		t.Errorf("after setColumnWidth: columnWidth(0) = %.0f, want 200", got)
	}
	if got := m.ColumnWidth(0); got != 120 {
		t.Errorf("override leaked into model: model width = %.0f, want 120 unchanged", got)
	}

	// Below the minimum clamps to columnMinWidth.
	tbl.setColumnWidth(0, 5)
	if got := tbl.columnWidth(0); got != columnMinWidth {
		t.Errorf("clamp: columnWidth(0) = %.0f, want %.0f", got, columnMinWidth)
	}
}

// TestTableResizeDrag: an OnLeftDown that lands on a column boundary starts a
// resize drag, OnMouseMove widens the column by the pointer delta, OnLeftUp
// ends the drag, and a header click on the boundary never triggers a sort.
func TestTableResizeDrag(t *testing.T) {
	tbl, _ := newResizeTable()
	yHead := tbl.HeaderHeight() * 0.5

	// Press on col 0's boundary (x=120) and drag 40px to the right.
	tbl.OnLeftDown(120, yHead)
	if tbl.resizeCol != 0 {
		t.Fatalf("after boundary press: resizeCol = %d, want 0 (drag should start)", tbl.resizeCol)
	}
	if tbl.SortColumn() != -1 {
		t.Errorf("boundary press triggered a sort: SortColumn = %d, want -1", tbl.SortColumn())
	}

	tbl.OnMouseMove(160, yHead)
	if got := tbl.columnWidth(0); got != 160 {
		t.Errorf("after 40px drag: columnWidth(0) = %.0f, want 160", got)
	}

	tbl.OnLeftUp(160, yHead)
	if tbl.resizeCol != -1 {
		t.Errorf("after OnLeftUp: resizeCol = %d, want -1 (drag should end)", tbl.resizeCol)
	}
	if got := tbl.columnWidth(0); got != 160 {
		t.Errorf("after drag end: columnWidth(0) = %.0f, want 160 (width should persist)", got)
	}
}

// TestTableResizeDragClampsMin: dragging a column boundary far to the left
// clamps the column to the minimum width rather than going negative.
func TestTableResizeDragClampsMin(t *testing.T) {
	tbl, _ := newResizeTable()
	yHead := tbl.HeaderHeight() * 0.5

	tbl.OnLeftDown(120, yHead) // grab col 0 boundary (start width 120)
	tbl.OnMouseMove(0, yHead)  // drag 120px left -> would be 0, clamps to min
	if got := tbl.columnWidth(0); got != columnMinWidth {
		t.Errorf("over-drag left: columnWidth(0) = %.0f, want clamp %.0f", got, columnMinWidth)
	}
	tbl.OnLeftUp(0, yHead)
}

// TestTableHeaderBodyClickStillSorts is the regression guard: a header click in
// a column body (not on a boundary) must still cycle the sort, and must not
// start a resize drag. It mirrors the existing sort test's first two clicks.
func TestTableHeaderBodyClickStillSorts(t *testing.T) {
	tbl, m := newResizeTable()
	yHead := tbl.HeaderHeight() * 0.5

	// x=10 is well inside col 0's body, away from the x=120 boundary.
	tbl.OnLeftDown(10, yHead)
	if tbl.resizeCol != -1 {
		t.Fatalf("body click started a resize: resizeCol = %d, want -1", tbl.resizeCol)
	}
	if tbl.SortColumn() != 0 || !tbl.SortAscending() {
		t.Fatalf("body click did not sort: column=%d ascending=%v, want 0/true", tbl.SortColumn(), tbl.SortAscending())
	}
	if got := m.CellText(0, 0); got != "alice" {
		t.Errorf("after ascending sort, row 0 name = %q, want %q", got, "alice")
	}

	// Second body click toggles to descending (sort still works after resize wiring).
	tbl.OnLeftDown(10, yHead)
	if tbl.SortColumn() != 0 || tbl.SortAscending() {
		t.Fatalf("second body click: column=%d ascending=%v, want 0/false", tbl.SortColumn(), tbl.SortAscending())
	}
	if got := m.CellText(0, 0); got != "Carol" {
		t.Errorf("after descending sort, row 0 name = %q, want %q", got, "Carol")
	}
}

// TestTableResizeThenSortCoexist: resizing a column and then clicking a header
// body both work in the same session, in either order.
func TestTableResizeThenSortCoexist(t *testing.T) {
	tbl, m := newResizeTable()
	yHead := tbl.HeaderHeight() * 0.5

	// Resize col 0 to 160.
	tbl.OnLeftDown(120, yHead)
	tbl.OnMouseMove(160, yHead)
	tbl.OnLeftUp(160, yHead)
	if got := tbl.columnWidth(0); got != 160 {
		t.Fatalf("resize: columnWidth(0) = %.0f, want 160", got)
	}

	// The boundary moved with the new width: col 0's edge is now at x=160.
	if got := tbl.columnBoundaryAt(160); got != 0 {
		t.Errorf("after resize, columnBoundaryAt(160) = %d, want 0 (boundary follows width)", got)
	}

	// Now a body click on col 1 (its body shifted right by 40px; x=200 is inside
	// col 1, which spans 160..280, and clear of both boundaries).
	tbl.OnLeftDown(200, yHead)
	if tbl.SortColumn() != 1 || !tbl.SortAscending() {
		t.Fatalf("post-resize header click: column=%d ascending=%v, want 1/true", tbl.SortColumn(), tbl.SortAscending())
	}
	if got := m.CellText(0, 1); got != "25" {
		t.Errorf("post-resize sort by age, row 0 age = %q, want %q", got, "25")
	}
}
