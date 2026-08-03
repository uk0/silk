package gui

import "testing"

// strictModel is a read-only TableModel (neither sortable nor editable) that
// indexes its backing slice directly, the way a host model written against the
// TableModel interface naturally would. The interface promises the view will
// only ask for rows in [0, RowCount), so the model does not bounds-check.
type strictModel struct {
	headers []string
	rows    [][]string
}

func (m *strictModel) RowCount() int                { return len(m.rows) }
func (m *strictModel) ColumnCount() int             { return len(m.headers) }
func (m *strictModel) CellText(row, col int) string { return m.rows[row][col] }
func (m *strictModel) HeaderText(col int) string    { return m.headers[col] }
func (m *strictModel) ColumnWidth(col int) float64  { return 120 }

// TestTableDisplayOrderDropsStalePermutation: buildDisplayOrder snapshots the
// row indices at sort time and dispRow keeps using it afterwards. When the host
// shrinks the model the permutation still holds the old, larger indices, so
// every display row can map to a model row that no longer exists — which is
// what Draw and commitEdit then hand to the model.
func TestTableDisplayOrderDropsStalePermutation(t *testing.T) {
	m := &strictModel{
		headers: []string{"name"},
		rows:    [][]string{{"e"}, {"d"}, {"c"}, {"b"}, {"a"}},
	}
	tbl := NewTable()
	tbl.SetModel(m)
	tbl.sortByColumn(0) // ascending: display order becomes [4 3 2 1 0]
	if len(tbl.displayOrder) != 5 {
		t.Fatalf("setup: displayOrder = %v, want 5 entries", tbl.displayOrder)
	}

	// The host drops the last three rows and repaints.
	m.rows = m.rows[:2]

	for r := 0; r < m.RowCount(); r++ {
		if got := tbl.dispRow(r); got < 0 || got >= m.RowCount() {
			t.Errorf("dispRow(%d) = %d, outside the model's %d rows", r, got, m.RowCount())
		}
	}

	// The read Draw performs for every visible row must not fault.
	defer func() {
		if e := recover(); e != nil {
			t.Fatalf("reading cells through dispRow after the model shrank panicked: %v", e)
		}
	}()
	for r := 0; r < m.RowCount(); r++ {
		_ = m.CellText(tbl.dispRow(r), 0)
	}
}

// TestTableDisplayOrderStillSortsUnchangedModel pins the working case: as long
// as the row count matches, the view-side permutation keeps sorting a model
// that cannot sort itself.
func TestTableDisplayOrderStillSortsUnchangedModel(t *testing.T) {
	m := &strictModel{
		headers: []string{"name"},
		rows:    [][]string{{"e"}, {"d"}, {"c"}, {"b"}, {"a"}},
	}
	tbl := NewTable()
	tbl.SetModel(m)
	tbl.sortByColumn(0)

	want := []string{"a", "b", "c", "d", "e"}
	for r := 0; r < m.RowCount(); r++ {
		if got := m.CellText(tbl.dispRow(r), 0); got != want[r] {
			t.Errorf("display row %d = %q, want %q", r, got, want[r])
		}
	}
}
