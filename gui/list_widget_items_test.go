package gui

import "testing"

// newItemList builds a ListWidget holding the given labels in order.
func newItemList(texts ...string) *ListWidget {
	lw := NewListWidget()
	for _, s := range texts {
		lw.Append(ListItem{Text: s})
	}
	return lw
}

// itemTexts reads the widget's items back out as labels, top to bottom.
func itemTexts(lw *ListWidget) []string {
	out := make([]string, lw.Count())
	for i := 0; i < lw.Count(); i++ {
		out[i] = lw.Item(i).Text
	}
	return out
}

func eqTexts(a, b []string) bool {
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

// TestListWidgetInsertKeepsNeighbours: Insert must shift the tail down, not
// overwrite it. The old implementation appended into this.items[:idx], which
// wrote through the shared backing array and turned [A B C] + X@1 into
// [A X X C] — B destroyed, X duplicated.
func TestListWidgetInsertKeepsNeighbours(t *testing.T) {
	cases := []struct {
		name string
		init []string
		idx  int
		want []string
	}{
		{"middle", []string{"A", "B", "C"}, 1, []string{"A", "X", "B", "C"}},
		{"front", []string{"A", "B", "C"}, 0, []string{"X", "A", "B", "C"}},
		{"end", []string{"A", "B", "C"}, 3, []string{"A", "B", "C", "X"}},
		{"into empty", nil, 0, []string{"X"}},
	}
	for _, c := range cases {
		lw := newItemList(c.init...)
		lw.Insert(c.idx, ListItem{Text: "X"})
		if got := itemTexts(lw); !eqTexts(got, c.want) {
			t.Errorf("%s: Insert(%d,X) into %v = %v, want %v", c.name, c.idx, c.init, got, c.want)
		}
	}
}

// TestListWidgetRemoveLastDropsExactlyOne: RemoveLast used to truncate to
// len-2, silently taking the second-to-last item with it, and to panic
// outright on a one-item list (items[:-1]). ComboBox forwards straight to it.
func TestListWidgetRemoveLastDropsExactlyOne(t *testing.T) {
	lw := newItemList("A", "B", "C")
	got := lw.RemoveLast()
	if got.Text != "C" {
		t.Errorf("RemoveLast returned %q, want %q", got.Text, "C")
	}
	if want := []string{"A", "B"}; !eqTexts(itemTexts(lw), want) {
		t.Errorf("after RemoveLast items = %v, want %v", itemTexts(lw), want)
	}
}

func TestListWidgetRemoveLastOnSingleItem(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RemoveLast on a one-item list panicked: %v", r)
		}
	}()
	lw := newItemList("A")
	if got := lw.RemoveLast(); got.Text != "A" {
		t.Errorf("RemoveLast returned %q, want %q", got.Text, "A")
	}
	if lw.Count() != 0 {
		t.Errorf("Count after RemoveLast = %d, want 0", lw.Count())
	}
}

// TestListWidgetItemListCopiesItems: ItemList copied into a nil slice, so it
// always handed back an empty list no matter how many items the widget held.
func TestListWidgetItemListCopiesItems(t *testing.T) {
	lw := newItemList("A", "B", "C")
	got := lw.ItemList()
	if len(got) != 3 {
		t.Fatalf("ItemList() len = %d, want 3", len(got))
	}
	for i, want := range []string{"A", "B", "C"} {
		if got[i].Text != want {
			t.Errorf("ItemList()[%d].Text = %q, want %q", i, got[i].Text, want)
		}
	}
	// The copy must be independent of the widget's storage.
	got[0].Text = "mutated"
	if lw.Item(0).Text != "A" {
		t.Errorf("mutating the returned slice changed the widget: item 0 = %q", lw.Item(0).Text)
	}
}

// TestListWidgetIndexAfterRemove exercises the pure index-lifetime helper:
// where a kept index has to land once row `removed` is gone.
func TestListWidgetIndexAfterRemove(t *testing.T) {
	cases := []struct {
		name                string
		idx, removed, count int
		want                int
	}{
		{"below the removed row is unaffected", 0, 2, 4, 0},
		{"above the removed row slides down", 3, 1, 4, 2},
		{"removed row takes over its successor", 1, 1, 4, 1},
		{"removed last row falls back one", 3, 3, 3, 2},
		{"last row of a now empty list", 0, 0, 0, -1},
		{"no selection stays none", -1, 2, 4, -1},
	}
	for _, c := range cases {
		if got := indexAfterRemove(c.idx, c.removed, c.count); got != c.want {
			t.Errorf("%s: indexAfterRemove(%d,%d,%d) = %d, want %d",
				c.name, c.idx, c.removed, c.count, got, c.want)
		}
	}
}

// TestListWidgetActiveIndexSurvivesMutation: the selected index is kept across
// Append/Insert/Remove/Clear, so it must be re-mapped when rows move. A stale
// index is what ActiveItem and every host that reads ActiveIndex then use to
// index a slice that just got shorter.
func TestListWidgetActiveIndexSurvivesMutation(t *testing.T) {
	// Removing the selected last row must not leave the index past the end.
	lw := newItemList("A", "B", "C")
	lw.setActiveIndex(2)
	lw.Remove(2)
	if got := lw.ActiveIndex(); got >= lw.Count() {
		t.Errorf("after Remove(2): ActiveIndex = %d with Count = %d (out of range)", got, lw.Count())
	}
	if got, want := lw.ActiveItem().Text, "B"; got != want {
		t.Errorf("after Remove(2): ActiveItem = %q, want %q", got, want)
	}

	// Removing a row above the selection must keep the selection on its item.
	lw = newItemList("A", "B", "C")
	lw.setActiveIndex(2) // "C"
	lw.Remove(0)
	if got, want := lw.ActiveItem().Text, "C"; got != want {
		t.Errorf("after Remove(0): ActiveItem = %q, want %q", got, want)
	}

	// Inserting above the selection must keep the selection on its item.
	lw = newItemList("A", "B", "C")
	lw.setActiveIndex(2) // "C"
	lw.Insert(0, ListItem{Text: "X"})
	if got, want := lw.ActiveItem().Text, "C"; got != want {
		t.Errorf("after Insert(0): ActiveItem = %q, want %q", got, want)
	}

	// Clear drops the selection entirely.
	lw = newItemList("A", "B", "C")
	lw.setActiveIndex(2)
	lw.Clear()
	if got := lw.ActiveIndex(); got != -1 {
		t.Errorf("after Clear: ActiveIndex = %d, want -1", got)
	}
}

// TestListWidgetNoHoverBeforeMouseMove: hoverIndex is compared against a row
// number in Draw, so its "nothing hovered" value has to be -1 like activeIndex.
// Left at the zero value it claims row 0 is hovered on a widget the pointer has
// never touched.
func TestListWidgetNoHoverBeforeMouseMove(t *testing.T) {
	lw := NewListWidget()
	if lw.hoverIndex != -1 {
		t.Errorf("fresh list hoverIndex = %d, want -1", lw.hoverIndex)
	}
}

// TestListWidgetHitTestMatchesPaintedRows: a click must land on the row painted
// at that y. Draw translates by padding.T and then by -scrollY*rowHeight, so
// row r covers [padding.T + (r-scrollY)*rh, +rh) and nothing is painted above
// padding.T. HitTest truncated toward zero without that lower bound, so the
// padding strip resolved to the row one step above the top of the view — with
// the list scrolled, a row that is not on screen at all.
func TestListWidgetHitTestMatchesPaintedRows(t *testing.T) {
	const rh = 24.0 // Theme().ItemHeight
	const padT = 5.0
	lw := newItemList("A", "B", "C", "D", "E", "F", "G", "H", "I", "J")
	lw.SetBounds(0, 0, 200, padT+4*rh+padT) // exactly 4 whole rows in the viewport
	lw.Layout()
	if got := lw.RowHeight(); got != rh {
		t.Fatalf("fixture assumes rowHeight %v, got %v", rh, got)
	}
	if got := lw.padding.T; got != padT {
		t.Fatalf("fixture assumes padding.T %v, got %v", padT, got)
	}

	check := func(scroll, y float64, want int) {
		t.Helper()
		if got, _ := lw.HitTest(10, y); got != want {
			t.Errorf("scroll=%v HitTest(10,%v) = row %d, want %d", scroll, y, got, want)
		}
	}

	// Unscrolled: row 0 starts at padding.T; above it is the frame.
	check(0, 0, -1)
	check(0, padT-0.5, -1)
	check(0, padT, 0)
	check(0, padT+rh-0.5, 0)
	check(0, padT+rh, 1)

	// Scrolled by three rows: rows 3..6 are the ones painted.
	lw.SetScrollY(3)
	if got := lw.ScrollY(); got != 3 {
		t.Fatalf("fixture: ScrollY = %v, want 3", got)
	}
	check(3, 0, -1)
	check(3, padT-0.5, -1)
	check(3, padT, 3)
	check(3, padT+rh-0.5, 3)
	check(3, padT+rh, 4)
	check(3, padT+3*rh, 6)
}

// TestListWidgetCheckClickOutsideAnyRow: HitTest resolves the column from x
// alone, so a press-and-release in the checkbox column at a y that holds no row
// (an empty list, the strip above the first row, the space past the last one)
// arrives at OnLeftUp as row -1 / col 1. downRow == row is then trivially true
// and the toggle indexes items[-1].
func TestListWidgetCheckClickOutsideAnyRow(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("clicking the checkbox column outside any row panicked: %v", r)
		}
	}()
	lw := NewListWidget()
	lw.SetCheckBoxVisible(true)
	lw.SetBounds(0, 0, 200, 100)
	lw.Layout()

	// Empty list: every y is outside a row.
	x := 2.0 // inside the checkbox column, left of the text column
	lw.OnLeftDown(x, 50)
	lw.OnLeftUp(x, 50)

	// Populated, but clicked below the last row.
	lw.Append(ListItem{Text: "A"})
	lw.OnLeftDown(x, 90)
	lw.OnLeftUp(x, 90)
}
