package gui

import (
	"reflect"
	"testing"
)

// column extracts the values of a single column across rows, in row order.
func column(rows [][]string, col int) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		if col >= 0 && col < len(r) {
			out[i] = r[col]
		}
	}
	return out
}

// TestColumnIsNumeric: a column counts as numeric only when every non-empty
// cell parses as a float64; empty cells are ignored, a non-number poisons it,
// and an all-empty column is not numeric.
func TestColumnIsNumeric(t *testing.T) {
	cases := []struct {
		name string
		rows [][]string
		col  int
		want bool
	}{
		{"ints", [][]string{{"3"}, {"1"}, {"2"}}, 0, true},
		{"floats", [][]string{{"3.5"}, {"-1"}, {"2e3"}}, 0, true},
		{"empties skipped", [][]string{{"3"}, {""}, {"2"}}, 0, true},
		{"text poisons", [][]string{{"3"}, {"x"}, {"2"}}, 0, false},
		{"all empty", [][]string{{""}, {""}}, 0, false},
	}
	for _, c := range cases {
		if got := columnIsNumeric(c.rows, c.col); got != c.want {
			t.Errorf("%s: columnIsNumeric = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestCompareTableCells: string compare is case-insensitive and sign-correct.
func TestCompareTableCells(t *testing.T) {
	if compareTableCells("Apple", "banana") != -1 {
		t.Errorf("Apple < banana expected -1 (case-insensitive)")
	}
	if compareTableCells("banana", "Apple") != 1 {
		t.Errorf("banana > Apple expected 1")
	}
	if compareTableCells("Foo", "foo") != 0 {
		t.Errorf("Foo == foo expected 0 (case-insensitive)")
	}
}

// TestCompareTableCellsNumeric: numeric compare orders by value, and treats an
// unparseable cell as sorting before any number.
func TestCompareTableCellsNumeric(t *testing.T) {
	if compareTableCellsNumeric("2", "10") != -1 {
		t.Errorf("2 < 10 numerically expected -1 (got string-style ordering?)")
	}
	if compareTableCellsNumeric("10", "2") != 1 {
		t.Errorf("10 > 2 numerically expected 1")
	}
	if compareTableCellsNumeric("5", "5") != 0 {
		t.Errorf("5 == 5 expected 0")
	}
	if compareTableCellsNumeric("", "1") != -1 {
		t.Errorf("empty should sort before a number expected -1")
	}
}

// TestSortRowsByColumnString: a non-numeric column sorts case-insensitively,
// ascending and descending.
func TestSortRowsByColumnString(t *testing.T) {
	rows := [][]string{{"banana"}, {"Apple"}, {"cherry"}}
	sortRowsByColumn(rows, 0, true)
	if got, want := column(rows, 0), []string{"Apple", "banana", "cherry"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ascending string sort = %v, want %v", got, want)
	}
	sortRowsByColumn(rows, 0, false)
	if got, want := column(rows, 0), []string{"cherry", "banana", "Apple"}; !reflect.DeepEqual(got, want) {
		t.Errorf("descending string sort = %v, want %v", got, want)
	}
}

// TestSortRowsByColumnNumeric: a fully-numeric column sorts by value, not by
// lexical order (so 9 comes before 10).
func TestSortRowsByColumnNumeric(t *testing.T) {
	rows := [][]string{{"10"}, {"9"}, {"100"}, {"2"}}
	sortRowsByColumn(rows, 0, true)
	if got, want := column(rows, 0), []string{"2", "9", "10", "100"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ascending numeric sort = %v, want %v", got, want)
	}
	sortRowsByColumn(rows, 0, false)
	if got, want := column(rows, 0), []string{"100", "10", "9", "2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("descending numeric sort = %v, want %v", got, want)
	}
}

// TestSortRowsByColumnStable: rows that tie on the sort column keep their
// original relative order. Sorting by column 0 (all equal) must leave the
// second column in its initial sequence.
func TestSortRowsByColumnStable(t *testing.T) {
	rows := [][]string{
		{"x", "1"},
		{"x", "2"},
		{"x", "3"},
		{"x", "4"},
	}
	sortRowsByColumn(rows, 0, true)
	if got, want := column(rows, 1), []string{"1", "2", "3", "4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("stable tiebreak broke order: col1 = %v, want %v", got, want)
	}
}

// TestSimpleModelSortAndRestore: the model sorts in place and RestoreOrder
// returns the rows to their original insertion order.
func TestSimpleModelSortAndRestore(t *testing.T) {
	m := NewSimpleTableModel([]string{"name", "age"})
	m.AddRow("Carol", "30")
	m.AddRow("alice", "25")
	m.AddRow("Bob", "40")

	m.SortByColumn(0, true)
	if got, want := []string{m.CellText(0, 0), m.CellText(1, 0), m.CellText(2, 0)},
		[]string{"alice", "Bob", "Carol"}; !reflect.DeepEqual(got, want) {
		t.Errorf("model ascending sort by name = %v, want %v", got, want)
	}

	m.SortByColumn(1, true) // age is numeric: 25 < 30 < 40
	if got, want := []string{m.CellText(0, 1), m.CellText(1, 1), m.CellText(2, 1)},
		[]string{"25", "30", "40"}; !reflect.DeepEqual(got, want) {
		t.Errorf("model numeric sort by age = %v, want %v", got, want)
	}

	m.RestoreOrder()
	if got, want := []string{m.CellText(0, 0), m.CellText(1, 0), m.CellText(2, 0)},
		[]string{"Carol", "alice", "Bob"}; !reflect.DeepEqual(got, want) {
		t.Errorf("RestoreOrder = %v, want original insertion order %v", got, want)
	}
}

// TestTableHeaderClickSorts drives the public Table API: a header click (via
// the same OnLeftDown path the mouse handler uses) reorders the model rows,
// a second click on the same column toggles to descending, and a third click
// restores the original order. SigSortChanged is observed across the cycle.
func TestTableHeaderClickSorts(t *testing.T) {
	tbl := NewTable()
	m := NewSimpleTableModel([]string{"name", "age"})
	m.AddRow("Carol", "30")
	m.AddRow("alice", "25")
	m.AddRow("Bob", "40")
	tbl.SetModel(m)

	var lastCol int
	var lastAsc bool
	calls := 0
	tbl.SigSortChanged(func(col int, ascending bool) {
		lastCol, lastAsc = col, ascending
		calls++
	})

	if tbl.SortColumn() != -1 {
		t.Fatalf("fresh table should be unsorted, got column %d", tbl.SortColumn())
	}

	// Click header of column 0 (y inside header height, x inside first column).
	yHead := tbl.HeaderHeight() * 0.5
	tbl.OnLeftDown(10, yHead)
	if tbl.SortColumn() != 0 || !tbl.SortAscending() {
		t.Fatalf("after first click: column=%d ascending=%v, want 0/true", tbl.SortColumn(), tbl.SortAscending())
	}
	if got, want := []string{m.CellText(0, 0), m.CellText(1, 0), m.CellText(2, 0)},
		[]string{"alice", "Bob", "Carol"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after first click rows = %v, want ascending %v", got, want)
	}
	if lastCol != 0 || !lastAsc {
		t.Errorf("SigSortChanged after first click = (%d,%v), want (0,true)", lastCol, lastAsc)
	}

	// Second click on the same column toggles to descending.
	tbl.OnLeftDown(10, yHead)
	if tbl.SortColumn() != 0 || tbl.SortAscending() {
		t.Fatalf("after second click: column=%d ascending=%v, want 0/false", tbl.SortColumn(), tbl.SortAscending())
	}
	if got, want := []string{m.CellText(0, 0), m.CellText(1, 0), m.CellText(2, 0)},
		[]string{"Carol", "Bob", "alice"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after second click rows = %v, want descending %v", got, want)
	}

	// Third click on the same column restores the original insertion order.
	tbl.OnLeftDown(10, yHead)
	if tbl.SortColumn() != -1 {
		t.Fatalf("after third click should be unsorted, got column %d", tbl.SortColumn())
	}
	if got, want := []string{m.CellText(0, 0), m.CellText(1, 0), m.CellText(2, 0)},
		[]string{"Carol", "alice", "Bob"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after third click rows = %v, want original %v", got, want)
	}

	if calls != 3 {
		t.Errorf("SigSortChanged fired %d times, want 3", calls)
	}
}

// TestTableHeaderClickSwitchColumn: clicking a different column starts a fresh
// ascending sort on that column rather than toggling the previous one.
func TestTableHeaderClickSwitchColumn(t *testing.T) {
	tbl := NewTable()
	m := NewSimpleTableModel([]string{"name", "age"})
	m.AddRow("Carol", "30")
	m.AddRow("alice", "25")
	m.AddRow("Bob", "40")
	tbl.SetModel(m)

	// First column 0 ascending.
	tbl.OnLeftDown(10, tbl.HeaderHeight()*0.5)
	// Now click column 1 (age). Column widths default to 120, so x past 120
	// lands in the second column.
	tbl.OnLeftDown(130, tbl.HeaderHeight()*0.5)
	if tbl.SortColumn() != 1 || !tbl.SortAscending() {
		t.Fatalf("switching columns: column=%d ascending=%v, want 1/true", tbl.SortColumn(), tbl.SortAscending())
	}
	if got, want := []string{m.CellText(0, 1), m.CellText(1, 1), m.CellText(2, 1)},
		[]string{"25", "30", "40"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after switching to age column rows = %v, want %v", got, want)
	}
}

// TestTableHeaderClickColumnHitTest: the header hit-test maps an x past the
// last column to -1 and never triggers a sort.
func TestTableHeaderClickColumnHitTest(t *testing.T) {
	tbl := NewTable()
	m := NewSimpleTableModel([]string{"a", "b"})
	m.AddRow("1", "2")
	tbl.SetModel(m)

	if got := tbl.headerColumnAt(0); got != 0 {
		t.Errorf("headerColumnAt(0) = %d, want 0", got)
	}
	if got := tbl.headerColumnAt(130); got != 1 {
		t.Errorf("headerColumnAt(130) = %d, want 1", got)
	}
	// Two columns of width 120 = 240; past that is out of range.
	if got := tbl.headerColumnAt(500); got != -1 {
		t.Errorf("headerColumnAt(500) = %d, want -1", got)
	}
	// Clicking past the columns must leave the table unsorted.
	tbl.OnLeftDown(500, tbl.HeaderHeight()*0.5)
	if tbl.SortColumn() != -1 {
		t.Errorf("click past columns set sort column to %d, want -1", tbl.SortColumn())
	}
}
