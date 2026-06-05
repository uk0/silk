package gui

import (
	"testing"
)

// newKeyboardTable builds a two-column table with several rows, the fixture
// shared by the keyboard-navigation tests.
func newKeyboardTable() (*Table, *SimpleTableModel) {
	tbl := NewTable()
	m := NewSimpleTableModel([]string{"name", "age"})
	m.AddRow("Carol", "30")
	m.AddRow("alice", "25")
	m.AddRow("Bob", "40")
	m.AddRow("Dave", "22")
	m.AddRow("Eve", "55")
	tbl.SetModel(m)
	return tbl, m
}

// TestTableRowStep: the pure row-movement helper clamps to [0, n-1], pages by
// an arbitrary delta, and resolves a "no current row" (cur<0) start to the
// first row on a downward step but stays put on an upward one.
func TestTableRowStep(t *testing.T) {
	cases := []struct {
		name  string
		cur   int
		n     int
		delta int
		want  int
	}{
		{"down one", 2, 5, 1, 3},
		{"up one", 2, 5, -1, 1},
		{"down clamps at bottom", 4, 5, 1, 4},
		{"up clamps at top", 0, 5, -1, 0},
		{"page down", 0, 5, 3, 3},
		{"page down past end clamps", 3, 5, 3, 4},
		{"page up past start clamps", 1, 5, -3, 0},
		{"no current row, down lands on first", -1, 5, 1, 0},
		{"no current row, page down lands on page-1", -1, 5, 3, 2},
		{"no current row, up stays at first", -1, 5, -1, 0},
		{"empty list yields -1", -1, 0, 1, -1},
	}
	for _, c := range cases {
		if got := tableRowStep(c.cur, c.n, c.delta); got != c.want {
			t.Errorf("%s: tableRowStep(%d,%d,%d) = %d, want %d", c.name, c.cur, c.n, c.delta, got, c.want)
		}
	}
}

// TestTableKeyboardUpDown: Down/Up move the current row by one and clamp at the
// ends. CurrentRow and SelectedRow are the same concept and move together.
func TestTableKeyboardUpDown(t *testing.T) {
	tbl, _ := newKeyboardTable()

	// From "no current row", Down lands on the first row.
	tbl.OnKeyDown(KeyDown, false)
	if got := tbl.CurrentRow(); got != 0 {
		t.Fatalf("first Down: CurrentRow = %d, want 0", got)
	}
	if tbl.SelectedRow() != tbl.CurrentRow() {
		t.Errorf("SelectedRow %d != CurrentRow %d (should alias)", tbl.SelectedRow(), tbl.CurrentRow())
	}

	tbl.OnKeyDown(KeyDown, false)
	tbl.OnKeyDown(KeyDown, false)
	if got := tbl.CurrentRow(); got != 2 {
		t.Fatalf("after three Downs: CurrentRow = %d, want 2", got)
	}

	tbl.OnKeyDown(KeyUp, false)
	if got := tbl.CurrentRow(); got != 1 {
		t.Fatalf("after Up: CurrentRow = %d, want 1", got)
	}

	// Up at the top clamps to row 0.
	tbl.OnKeyDown(KeyUp, false)
	tbl.OnKeyDown(KeyUp, false)
	if got := tbl.CurrentRow(); got != 0 {
		t.Errorf("Up at top: CurrentRow = %d, want clamp 0", got)
	}
}

// TestTableKeyboardHomeEnd: Home/End jump to the first/last row.
func TestTableKeyboardHomeEnd(t *testing.T) {
	tbl, m := newKeyboardTable()

	tbl.OnKeyDown(KeyEnd, false)
	if got, want := tbl.CurrentRow(), m.RowCount()-1; got != want {
		t.Fatalf("End: CurrentRow = %d, want %d (last)", got, want)
	}

	tbl.OnKeyDown(KeyHome, false)
	if got := tbl.CurrentRow(); got != 0 {
		t.Fatalf("Home: CurrentRow = %d, want 0 (first)", got)
	}
}

// TestTableKeyboardPageDown: PageDown advances by a viewport page of rows and
// clamps at the last row. The table is sized so a page is two rows.
func TestTableKeyboardPageDown(t *testing.T) {
	tbl, m := newKeyboardTable()
	// Header height defaults to ItemHeight+2; size the body to exactly two rows.
	tbl.SetBounds(0, 0, 200, tbl.HeaderHeight()+tbl.RowHeight()*2)
	if got := tbl.visibleRowsPerPage(); got != 2 {
		t.Fatalf("visibleRowsPerPage = %d, want 2 for this fixture", got)
	}

	tbl.SetCurrentRow(0)
	tbl.OnKeyDown(KeyPageDown, false)
	if got := tbl.CurrentRow(); got != 2 {
		t.Fatalf("PageDown from 0: CurrentRow = %d, want 2 (page of 2)", got)
	}

	// A second PageDown would reach row 4 (the last); a third clamps there.
	tbl.OnKeyDown(KeyPageDown, false)
	tbl.OnKeyDown(KeyPageDown, false)
	if got, want := tbl.CurrentRow(), m.RowCount()-1; got != want {
		t.Errorf("PageDown past end: CurrentRow = %d, want clamp %d", got, want)
	}

	// PageUp walks back by a page.
	tbl.OnKeyDown(KeyPageUp, false)
	if got := tbl.CurrentRow(); got != 2 {
		t.Errorf("PageUp from last: CurrentRow = %d, want 2", got)
	}
}

// TestTableKeyboardActivate: Enter (and Space) fire the row-activated callback
// with the current row; navigation keys do not fire it.
func TestTableKeyboardActivate(t *testing.T) {
	tbl, _ := newKeyboardTable()

	activated := -1
	calls := 0
	tbl.SigRowActivated(func(_ interface{}, row int) {
		activated = row
		calls++
	})

	tbl.SetCurrentRow(2)

	// A move must not activate.
	tbl.OnKeyDown(KeyDown, false)
	if calls != 0 {
		t.Fatalf("navigation fired activate %d times, want 0", calls)
	}

	tbl.OnKeyDown(KeyEnter, false)
	if calls != 1 || activated != tbl.CurrentRow() {
		t.Fatalf("Enter: calls=%d activated=%d, want 1 call on row %d", calls, activated, tbl.CurrentRow())
	}

	tbl.OnKeyDown(KeySpace, false)
	if calls != 2 {
		t.Errorf("Space: calls=%d, want 2 (Space also activates)", calls)
	}
}

// TestTableKeyboardNoModel: key handling on an empty/model-less table is a safe
// no-op (no panic, no current row).
func TestTableKeyboardNoModel(t *testing.T) {
	tbl := NewTable()
	tbl.OnKeyDown(KeyDown, false)
	tbl.OnKeyDown(KeyEnter, false)
	if got := tbl.CurrentRow(); got != -1 {
		t.Errorf("model-less table: CurrentRow = %d, want -1", got)
	}

	// A model with zero rows is also a no-op.
	tbl.SetModel(NewSimpleTableModel([]string{"a"}))
	tbl.OnKeyDown(KeyDown, false)
	if got := tbl.CurrentRow(); got != -1 {
		t.Errorf("empty model: CurrentRow = %d, want -1", got)
	}
}

// TestTableKeyboardCoexistsWithSort is a regression guard: keyboard navigation
// must not disturb the header click-to-sort path. After moving the current row
// with the keyboard, a header body click still cycles the sort.
func TestTableKeyboardCoexistsWithSort(t *testing.T) {
	tbl, m := newKeyboardTable()

	// Drive the keyboard first.
	tbl.OnKeyDown(KeyDown, false)
	tbl.OnKeyDown(KeyDown, false)
	if tbl.CurrentRow() != 1 {
		t.Fatalf("keyboard setup: CurrentRow = %d, want 1", tbl.CurrentRow())
	}

	// A header body click (x=10 inside col 0, away from the x=120 boundary) must
	// still sort ascending by name.
	yHead := tbl.HeaderHeight() * 0.5
	tbl.OnLeftDown(10, yHead)
	if tbl.SortColumn() != 0 || !tbl.SortAscending() {
		t.Fatalf("header click after keyboard nav: column=%d ascending=%v, want 0/true",
			tbl.SortColumn(), tbl.SortAscending())
	}
	if got := m.CellText(0, 0); got != "alice" {
		t.Errorf("ascending name sort, row 0 = %q, want %q", got, "alice")
	}

	// Keyboard navigation still works after the sort.
	tbl.OnKeyDown(KeyEnd, false)
	if got, want := tbl.CurrentRow(), m.RowCount()-1; got != want {
		t.Errorf("End after sort: CurrentRow = %d, want %d", got, want)
	}
}

// TestTableKeyboardCoexistsWithResize is a regression guard: keyboard
// navigation must not disturb the column-resize drag path. A boundary drag
// still resizes and does not trigger a sort, with keyboard nav interleaved.
func TestTableKeyboardCoexistsWithResize(t *testing.T) {
	tbl, _ := newKeyboardTable()
	yHead := tbl.HeaderHeight() * 0.5

	tbl.OnKeyDown(KeyDown, false) // current row 0

	// Drag col 0's boundary (x=120) 40px right.
	tbl.OnLeftDown(120, yHead)
	if tbl.resizeCol != 0 {
		t.Fatalf("boundary press: resizeCol = %d, want 0", tbl.resizeCol)
	}
	if tbl.SortColumn() != -1 {
		t.Errorf("boundary press triggered a sort: SortColumn = %d, want -1", tbl.SortColumn())
	}
	tbl.OnMouseMove(160, yHead)
	tbl.OnLeftUp(160, yHead)
	if got := tbl.columnWidth(0); got != 160 {
		t.Errorf("after resize drag: columnWidth(0) = %.0f, want 160", got)
	}

	// Keyboard navigation still works after the resize.
	tbl.OnKeyDown(KeyDown, false)
	if got := tbl.CurrentRow(); got != 1 {
		t.Errorf("Down after resize: CurrentRow = %d, want 1", got)
	}
}
