package gui

import (
	"reflect"
	"testing"
)

// newEditableTable builds a three-column, five-row table shared by the cell
// editing and selection tests.
func newEditableTable() (*Table, *SimpleTableModel) {
	tbl := NewTable()
	m := NewSimpleTableModel([]string{"id", "name", "city"})
	m.AddRow("1", "Carol", "Rome")
	m.AddRow("2", "Alice", "Paris")
	m.AddRow("3", "Bob", "Lyon")
	m.AddRow("4", "Dave", "Nice")
	m.AddRow("5", "Eve", "Metz")
	tbl.SetModel(m)
	return tbl, m
}

// geometryTable returns a fixture with deterministic header/row heights and a
// size large enough that neither scrollbar shows, so hit-test coordinates are
// stable: header = 24px, rows = 20px, three 120px columns.
func geometryTable() (*Table, *SimpleTableModel) {
	tbl, m := newEditableTable()
	tbl.SetHeaderHeight(24)
	tbl.SetRowHeight(20)
	tbl.SetBounds(0, 0, 400, 200)
	return tbl, m
}

// TestTableCellsEditableDefaultOff: editing is opt-in. A fresh table reports
// read-only and the gated begin-edit entry is a no-op that never spawns an
// editor.
func TestTableCellsEditableDefaultOff(t *testing.T) {
	tbl, m := newEditableTable()
	if tbl.IsCellsEditable() {
		t.Fatal("cell editing should default to off")
	}
	before := m.CellText(0, 0)

	tbl.tryBeginEdit(0, 0)
	if tbl.editing || tbl.editor != nil {
		t.Errorf("tryBeginEdit on a read-only table must not start an editor (editing=%v editor=%v)",
			tbl.editing, tbl.editor)
	}
	if got := m.CellText(0, 0); got != before {
		t.Errorf("read-only tryBeginEdit changed the model: %q, want %q", got, before)
	}
}

// TestTableReadOnlyDoubleClickNoEdit: with editing off, a genuine double-click
// (two quick OnLeftDown on the same cell) selects the row but does not edit or
// mutate the model.
func TestTableReadOnlyDoubleClickNoEdit(t *testing.T) {
	tbl, m := geometryTable()
	before := m.CellText(1, 0)

	// Row 1, column 0: y in [44,64), x in [0,120).
	tbl.OnLeftDown(10, 50)
	tbl.OnLeftDown(10, 50)

	if tbl.editing || tbl.editor != nil {
		t.Errorf("double-click on a read-only table started an editor")
	}
	if tbl.SelectedRow() != 1 {
		t.Errorf("double-click should still select the row: SelectedRow=%d, want 1", tbl.SelectedRow())
	}
	if got := m.CellText(1, 0); got != before {
		t.Errorf("read-only double-click changed the model: %q, want %q", got, before)
	}
}

// TestTableCellEditCommit: enabling editing, opening an editor and committing
// writes the new text back to the model and fires SigCellEdited once with the
// edited (row, col, text).
func TestTableCellEditCommit(t *testing.T) {
	tbl, m := newEditableTable()
	tbl.SetCellsEditable(true)

	var gotRow, gotCol = -1, -1
	var gotText string
	calls := 0
	tbl.SigCellEdited(func(row, col int, newText string) {
		gotRow, gotCol, gotText = row, col, newText
		calls++
	})

	tbl.beginEdit(1, 2)
	if !tbl.editing || tbl.editor == nil {
		t.Fatalf("beginEdit did not activate the editor (editing=%v)", tbl.editing)
	}
	if got := tbl.editor.Text(); got != "Paris" {
		t.Errorf("editor seeded with %q, want %q", got, "Paris")
	}

	tbl.editor.SetText("Berlin")
	tbl.commitEdit()

	if got := m.CellText(1, 2); got != "Berlin" {
		t.Errorf("commit wrote %q to the model, want %q", got, "Berlin")
	}
	if calls != 1 {
		t.Fatalf("SigCellEdited fired %d times, want 1", calls)
	}
	if gotRow != 1 || gotCol != 2 || gotText != "Berlin" {
		t.Errorf("SigCellEdited got (%d,%d,%q), want (1,2,%q)", gotRow, gotCol, gotText, "Berlin")
	}
	if tbl.editing {
		t.Errorf("editing flag still set after commit")
	}
}

// TestTableCellEditCancel: cancelling an edit discards the editor's text; the
// model is untouched and no SigCellEdited fires.
func TestTableCellEditCancel(t *testing.T) {
	tbl, m := newEditableTable()
	tbl.SetCellsEditable(true)

	calls := 0
	tbl.SigCellEdited(func(row, col int, newText string) { calls++ })

	before := m.CellText(2, 1)
	tbl.beginEdit(2, 1)
	tbl.editor.SetText("discard me")
	tbl.cancelEdit()

	if got := m.CellText(2, 1); got != before {
		t.Errorf("cancel wrote %q to the model, want unchanged %q", got, before)
	}
	if calls != 0 {
		t.Errorf("cancel fired SigCellEdited %d times, want 0", calls)
	}
	if tbl.editing {
		t.Errorf("editing flag still set after cancel")
	}
}

// TestTableDoubleClickBeginsEdit: end-to-end through the mouse path. With
// editing enabled, two quick clicks on the same cell open an editor seeded from
// that cell, and committing writes back.
func TestTableDoubleClickBeginsEdit(t *testing.T) {
	tbl, m := geometryTable()
	tbl.SetCellsEditable(true)

	// Row 1, column 0: y in [44,64), x in [0,120).
	tbl.OnLeftDown(10, 50)
	if tbl.SelectedRow() != 1 {
		t.Fatalf("first click: SelectedRow=%d, want 1", tbl.SelectedRow())
	}
	tbl.OnLeftDown(10, 50)

	if !tbl.editing || tbl.editor == nil {
		t.Fatalf("double-click did not begin an edit (editing=%v)", tbl.editing)
	}
	if tbl.editRow != 1 || tbl.editCol != 0 {
		t.Errorf("editing cell (%d,%d), want (1,0)", tbl.editRow, tbl.editCol)
	}
	if got, want := tbl.editor.Text(), m.CellText(1, 0); got != want {
		t.Errorf("editor seeded %q, want cell text %q", got, want)
	}

	tbl.editor.SetText("Edited")
	tbl.commitEdit()
	if got := m.CellText(1, 0); got != "Edited" {
		t.Errorf("after double-click edit + commit, cell = %q, want %q", got, "Edited")
	}
}

// TestTableOnlyOneEditorActive: opening a second editor while one is already
// open commits the first (writing its pending text back) before switching.
func TestTableOnlyOneEditorActive(t *testing.T) {
	tbl, m := newEditableTable()
	tbl.SetCellsEditable(true)

	tbl.beginEdit(0, 0)
	tbl.editor.SetText("first")
	// Open a second cell without an explicit commit.
	tbl.beginEdit(3, 1)

	if got := m.CellText(0, 0); got != "first" {
		t.Errorf("switching cells did not commit the first edit: cell(0,0)=%q, want %q", got, "first")
	}
	if tbl.editRow != 3 || tbl.editCol != 1 {
		t.Errorf("second editor on (%d,%d), want (3,1)", tbl.editRow, tbl.editCol)
	}
	if got := tbl.editor.Text(); got != "Dave" {
		t.Errorf("second editor seeded %q, want %q", got, "Dave")
	}
}

// TestTableCellAtXY: the pure hit-test maps points to (row, col), rejecting the
// header row and any area past the data or past the last column.
func TestTableCellAtXY(t *testing.T) {
	tbl, _ := geometryTable()

	cases := []struct {
		name             string
		x, y             float64
		wantRow, wantCol int
	}{
		{"header row", 10, 10, -1, -1},
		{"row0 col0", 10, 25, 0, 0},
		{"row0 col1", 130, 30, 0, 1},
		{"row2 col2", 250, 70, 2, 2},
		{"past last column", 400, 30, -1, -1},
		{"past last row", 10, 130, -1, -1},
	}
	for _, c := range cases {
		gotRow, gotCol := tbl.cellAtXY(c.x, c.y)
		if gotRow != c.wantRow || gotCol != c.wantCol {
			t.Errorf("%s: cellAtXY(%.0f,%.0f) = (%d,%d), want (%d,%d)",
				c.name, c.x, c.y, gotRow, gotCol, c.wantRow, c.wantCol)
		}
	}
}

// TestTableCellAtXYNoModel: hit-testing a model-less table is a safe no-op.
func TestTableCellAtXYNoModel(t *testing.T) {
	tbl := NewTable()
	if r, c := tbl.cellAtXY(10, 30); r != -1 || c != -1 {
		t.Errorf("model-less cellAtXY = (%d,%d), want (-1,-1)", r, c)
	}
}

// TestSimpleTableModelSetCellText: the model's write path updates a cell and
// ignores out-of-range indices, and the type satisfies EditableTableModel.
func TestSimpleTableModelSetCellText(t *testing.T) {
	var _ EditableTableModel = (*SimpleTableModel)(nil)

	m := NewSimpleTableModel([]string{"a", "b"})
	m.AddRow("1", "2")

	m.SetCellText(0, 1, "changed")
	if got := m.CellText(0, 1); got != "changed" {
		t.Errorf("SetCellText: cell(0,1) = %q, want %q", got, "changed")
	}

	// Out-of-range writes are ignored (no panic, no change).
	m.SetCellText(-1, 0, "x")
	m.SetCellText(0, 9, "x")
	m.SetCellText(9, 0, "x")
	if got := m.CellText(0, 0); got != "1" {
		t.Errorf("out-of-range writes disturbed cell(0,0) = %q, want %q", got, "1")
	}
}

// TestTableCommitWithNonEditableModel: a model that does not implement
// EditableTableModel stays read-only, but SigCellEdited still fires so a host
// can persist the value itself.
func TestTableCommitWithNonEditableModel(t *testing.T) {
	tbl := NewTable()
	tbl.SetModel(readOnlyModel{})
	tbl.SetCellsEditable(true)

	got := ""
	tbl.SigCellEdited(func(row, col int, newText string) { got = newText })

	tbl.beginEdit(0, 0)
	tbl.editor.SetText("typed")
	tbl.commitEdit() // must not panic despite the model having no SetCellText

	if got != "typed" {
		t.Errorf("SigCellEdited newText = %q, want %q", got, "typed")
	}
}

// readOnlyModel is a minimal TableModel with no write path, used to prove the
// commit path degrades gracefully.
type readOnlyModel struct{}

func (readOnlyModel) RowCount() int                { return 1 }
func (readOnlyModel) ColumnCount() int             { return 1 }
func (readOnlyModel) CellText(row, col int) string { return "ro" }
func (readOnlyModel) HeaderText(col int) string    { return "h" }
func (readOnlyModel) ColumnWidth(col int) float64  { return 120 }

// TestTableMultiSelect: Ctrl-style toggles and Shift-style range selection
// build up SelectedRows; a plain single selection reports just the current row.
func TestTableMultiSelect(t *testing.T) {
	tbl, _ := newEditableTable()

	// Single selection reports the one current row.
	tbl.SetSelectedRow(1)
	assertRows(t, "single", tbl.SelectedRows(), []int{1})

	// Ctrl-add rows 3 and 0 (the set seeds from the current row 1).
	tbl.toggleRowSelection(3)
	tbl.toggleRowSelection(0)
	assertRows(t, "ctrl add", tbl.SelectedRows(), []int{0, 1, 3})
	if !tbl.isRowSelected(3) || !tbl.isRowSelected(0) || !tbl.isRowSelected(1) {
		t.Errorf("isRowSelected disagrees with SelectedRows after ctrl add")
	}

	// Ctrl-remove row 1.
	tbl.toggleRowSelection(1)
	assertRows(t, "ctrl remove", tbl.SelectedRows(), []int{0, 3})
	if tbl.isRowSelected(1) {
		t.Errorf("row 1 still selected after ctrl remove")
	}

	// Shift-range from anchor 0 to 4 replaces the set with the whole range.
	tbl.selectRowRange(0, 4)
	assertRows(t, "shift range", tbl.SelectedRows(), []int{0, 1, 2, 3, 4})

	// A fresh single selection collapses the multi-selection.
	tbl.selectedRows = nil
	tbl.SetSelectedRow(2)
	assertRows(t, "collapse", tbl.SelectedRows(), []int{2})
}

// TestTableMultiSelectCollapsedByKeyboard: keyboard navigation collapses a
// multi-selection back to a single current row.
func TestTableMultiSelectCollapsedByKeyboard(t *testing.T) {
	tbl, _ := newEditableTable()
	tbl.SetSelectedRow(0)
	tbl.selectRowRange(0, 3)
	if len(tbl.SelectedRows()) != 4 {
		t.Fatalf("range select produced %d rows, want 4", len(tbl.SelectedRows()))
	}

	tbl.OnKeyDown(KeyDown, false)
	if got := tbl.SelectedRows(); len(got) != 1 {
		t.Errorf("after keyboard move, SelectedRows = %v, want a single row", got)
	}
}

// assertRows compares a SelectedRows result against an expected ascending slice.
func assertRows(t *testing.T, label string, got, want []int) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: SelectedRows = %v, want %v", label, got, want)
	}
}

// staticModel is a read-only TableModel: it implements neither
// SortableTableModel nor EditableTableModel. It exists to prove the view can
// sort a model that cannot sort itself, via its own display-order permutation,
// without ever mutating the model.
type staticModel struct {
	headers []string
	rows    [][]string
}

func (m *staticModel) RowCount() int    { return len(m.rows) }
func (m *staticModel) ColumnCount() int { return len(m.headers) }
func (m *staticModel) CellText(row, col int) string {
	if row < 0 || row >= len(m.rows) || col < 0 || col >= len(m.rows[row]) {
		return ""
	}
	return m.rows[row][col]
}
func (m *staticModel) HeaderText(col int) string   { return m.headers[col] }
func (m *staticModel) ColumnWidth(col int) float64 { return 120 }

// modelColumn reads a whole model column in raw (unsorted) model-row order, used
// to assert a view-side sort never mutated the underlying model.
func modelColumn(m TableModel, col int) []string {
	out := make([]string, m.RowCount())
	for r := range out {
		out[r] = m.CellText(r, col)
	}
	return out
}

// displayColumn reads a whole column in display order, mapping each display row
// through the table's dispRow permutation.
func displayColumn(tbl *Table, m TableModel, col int) []string {
	out := make([]string, m.RowCount())
	for i := range out {
		out[i] = m.CellText(tbl.dispRow(i), col)
	}
	return out
}

// TestTableViewSortReadOnlyModel: a model that does not implement
// SortableTableModel is still sorted on screen. The view builds a display-order
// permutation, so the displayed order changes while the model's own row order is
// left untouched. Ascending -> descending -> cleared mirrors the sortable-model
// click cycle, and SigSortChanged fires each step.
func TestTableViewSortReadOnlyModel(t *testing.T) {
	tbl := NewTable()
	m := &staticModel{
		headers: []string{"name"},
		rows:    [][]string{{"Carol"}, {"alice"}, {"Bob"}},
	}
	tbl.SetModel(m)

	// The test is only meaningful if the model really cannot sort itself.
	if _, ok := tbl.Model().(SortableTableModel); ok {
		t.Fatal("staticModel unexpectedly implements SortableTableModel")
	}

	original := []string{"Carol", "alice", "Bob"}
	calls := 0
	tbl.SigSortChanged(func(col int, ascending bool) { calls++ })

	// First sort: ascending, case-insensitive.
	tbl.sortByColumn(0)
	if tbl.SortColumn() != 0 || !tbl.SortAscending() {
		t.Fatalf("after first sort: col=%d asc=%v, want 0/true", tbl.SortColumn(), tbl.SortAscending())
	}
	if got, want := displayColumn(tbl, m, 0), []string{"alice", "Bob", "Carol"}; !reflect.DeepEqual(got, want) {
		t.Errorf("view ascending order = %v, want %v", got, want)
	}
	if got := modelColumn(m, 0); !reflect.DeepEqual(got, original) {
		t.Errorf("view sort mutated the model: %v, want untouched %v", got, original)
	}

	// Second sort on the same column: descending.
	tbl.sortByColumn(0)
	if tbl.SortColumn() != 0 || tbl.SortAscending() {
		t.Fatalf("after second sort: col=%d asc=%v, want 0/false", tbl.SortColumn(), tbl.SortAscending())
	}
	if got, want := displayColumn(tbl, m, 0), []string{"Carol", "Bob", "alice"}; !reflect.DeepEqual(got, want) {
		t.Errorf("view descending order = %v, want %v", got, want)
	}

	// Third sort: cleared back to the model's natural order.
	tbl.sortByColumn(0)
	if tbl.SortColumn() != -1 {
		t.Fatalf("after third sort should be unsorted, got column %d", tbl.SortColumn())
	}
	if got := displayColumn(tbl, m, 0); !reflect.DeepEqual(got, original) {
		t.Errorf("view natural order = %v, want %v", got, original)
	}
	if got := modelColumn(m, 0); !reflect.DeepEqual(got, original) {
		t.Errorf("model changed across the sort cycle: %v, want %v", got, original)
	}

	if calls != 3 {
		t.Errorf("SigSortChanged fired %d times, want 3", calls)
	}
}

// TestTableViewSortReadOnlyNumeric: a numeric column of a non-sortable model
// sorts by value, not lexically — "2","10","1" must display as 1,2,10, not
// 1,10,2. Guards the numeric-vs-lexical detection on the view-side sort path.
func TestTableViewSortReadOnlyNumeric(t *testing.T) {
	tbl := NewTable()
	m := &staticModel{
		headers: []string{"n"},
		rows:    [][]string{{"2"}, {"10"}, {"1"}},
	}
	tbl.SetModel(m)

	tbl.sortByColumn(0)
	if got, want := displayColumn(tbl, m, 0), []string{"1", "2", "10"}; !reflect.DeepEqual(got, want) {
		t.Errorf("numeric view sort = %v, want %v (numeric, not lexical)", got, want)
	}

	// A mixed (non-numeric) column falls back to a lexical compare.
	m2 := &staticModel{
		headers: []string{"s"},
		rows:    [][]string{{"2"}, {"10"}, {"1a"}},
	}
	tbl.SetModel(m2)
	tbl.sortByColumn(0)
	if got, want := displayColumn(tbl, m2, 0), []string{"10", "1a", "2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("lexical view sort = %v, want %v", got, want)
	}
}
