package gui

import "testing"

// newKbList builds a ListWidget populated with n simple rows ("0".."n-1") and
// gives it a fixed pixel size so rowsPerPage()/PageUp/PageDown have a real
// viewport to work against. Default row height is Theme().ItemHeight.
func newKbList(n int) *ListWidget {
	lw := NewListWidget()
	for i := 0; i < n; i++ {
		lw.Append(ListItem{Text: string(rune('0' + i%10))})
	}
	return lw
}

// TestListWidgetPageStepIndex exercises the pure page-step helper directly,
// independent of any widget state or GL render: stepping by a page in either
// direction, clamping at both ends, the empty list, and the "no current row"
// (-1) entry behavior.
func TestListWidgetPageStepIndex(t *testing.T) {
	cases := []struct {
		name              string
		cur, n, page, dir int
		want              int
	}{
		{"down one page", 0, 100, 10, +1, 10},
		{"up one page", 50, 100, 10, -1, 40},
		{"down clamps at end", 95, 100, 10, +1, 99},
		{"up clamps at start", 5, 100, 10, -1, 0},
		{"already last", 99, 100, 10, +1, 99},
		{"already first", 0, 100, 10, -1, 0},
		{"no current, down -> first", -1, 100, 10, +1, 0},
		{"no current, up -> last", -1, 100, 10, -1, 99},
		{"empty list", 0, 0, 10, +1, -1},
		{"page floored to 1", 3, 100, 0, +1, 4},
	}
	for _, c := range cases {
		if got := pageStepIndex(c.cur, c.n, c.page, c.dir); got != c.want {
			t.Errorf("%s: pageStepIndex(%d,%d,%d,%d)=%d want %d",
				c.name, c.cur, c.n, c.page, c.dir, got, c.want)
		}
	}
}

// TestListWidgetArrowKeysMoveAndClamp: Down advances the active row through the
// list and clamps at the last row; Up walks back and clamps at row 0. Starting
// from "no active row" (-1), the first Down lands on row 0.
func TestListWidgetArrowKeysMoveAndClamp(t *testing.T) {
	lw := newKbList(5)
	if lw.ActiveIndex() != -1 {
		t.Fatalf("fresh list active = %d, want -1", lw.ActiveIndex())
	}

	lw.OnKeyDown(KeyDown, false)
	if lw.ActiveIndex() != 0 {
		t.Fatalf("first Down: active = %d, want 0", lw.ActiveIndex())
	}
	for i := 1; i < 5; i++ {
		lw.OnKeyDown(KeyDown, false)
		if lw.ActiveIndex() != i {
			t.Fatalf("Down step %d: active = %d, want %d", i, lw.ActiveIndex(), i)
		}
	}
	// Extra Down at the bottom clamps.
	lw.OnKeyDown(KeyDown, false)
	if lw.ActiveIndex() != 4 {
		t.Fatalf("Down past end: active = %d, want 4 (clamped)", lw.ActiveIndex())
	}

	// Walk back up and clamp at 0.
	for i := 3; i >= 0; i-- {
		lw.OnKeyDown(KeyUp, false)
		if lw.ActiveIndex() != i {
			t.Fatalf("Up step: active = %d, want %d", lw.ActiveIndex(), i)
		}
	}
	lw.OnKeyDown(KeyUp, false)
	if lw.ActiveIndex() != 0 {
		t.Fatalf("Up past start: active = %d, want 0 (clamped)", lw.ActiveIndex())
	}
}

// TestListWidgetHomeEnd: Home jumps to the first row, End to the last.
func TestListWidgetHomeEnd(t *testing.T) {
	lw := newKbList(8)

	lw.OnKeyDown(KeyEnd, false)
	if lw.ActiveIndex() != 7 {
		t.Fatalf("End: active = %d, want 7", lw.ActiveIndex())
	}
	lw.OnKeyDown(KeyHome, false)
	if lw.ActiveIndex() != 0 {
		t.Fatalf("Home: active = %d, want 0", lw.ActiveIndex())
	}
}

// TestListWidgetPageKeys: with a viewport sized to a known number of rows,
// PageDown advances by roughly one page and PageUp walks back, both clamping at
// the list bounds.
func TestListWidgetPageKeys(t *testing.T) {
	lw := newKbList(100)
	rh := lw.RowHeight()
	// Size the viewport so exactly 10 rows fit (account for top+bottom padding).
	_, _, top, bottom := lw.Padding()
	lw.SetSize(120, rh*10+top+bottom)

	per := lw.rowsPerPage()
	if per != 10 {
		t.Fatalf("rowsPerPage = %d, want 10", per)
	}

	lw.OnKeyDown(KeyHome, false) // active = 0
	lw.OnKeyDown(KeyPageDown, false)
	if got := lw.ActiveIndex(); got != per {
		t.Fatalf("PageDown from 0: active = %d, want %d", got, per)
	}
	lw.OnKeyDown(KeyPageUp, false)
	if got := lw.ActiveIndex(); got != 0 {
		t.Fatalf("PageUp back: active = %d, want 0", got)
	}

	// PageUp at the top clamps at 0; PageDown near the end clamps at last row.
	lw.OnKeyDown(KeyPageUp, false)
	if got := lw.ActiveIndex(); got != 0 {
		t.Fatalf("PageUp at top: active = %d, want 0 (clamped)", got)
	}
	lw.OnKeyDown(KeyEnd, false)
	lw.OnKeyDown(KeyPageDown, false)
	if got := lw.ActiveIndex(); got != lw.Count()-1 {
		t.Fatalf("PageDown at end: active = %d, want %d (clamped)", got, lw.Count()-1)
	}
}

// TestListWidgetEnterSpaceActivate: Enter and Space both fire the Submit
// callback for the current row — the same callback the mouse-release path uses.
// An empty list fires nothing.
func TestListWidgetEnterSpaceActivate(t *testing.T) {
	lw := newKbList(4)

	var fired int
	lw.SigSubmit(func(o interface{}) { fired++ })

	// No active row yet: activate must not fire.
	lw.OnKeyDown(KeyEnter, false)
	if fired != 0 {
		t.Fatalf("Enter with no active row fired %d times, want 0", fired)
	}

	lw.OnKeyDown(KeyDown, false) // active = 0
	lw.OnKeyDown(KeyEnter, false)
	lw.OnKeyDown(KeySpace, false)
	if fired != 2 {
		t.Fatalf("Enter+Space fired %d times, want 2", fired)
	}
}

// TestListWidgetEmptyKeyboardNoop: keyboard events on an empty list are no-ops
// and never panic or move the (absent) active row.
func TestListWidgetEmptyKeyboardNoop(t *testing.T) {
	lw := NewListWidget()
	for _, k := range []int{KeyDown, KeyUp, KeyHome, KeyEnd, KeyPageDown, KeyPageUp, KeyEnter, KeySpace} {
		lw.OnKeyDown(k, false)
	}
	if lw.ActiveIndex() != -1 {
		t.Fatalf("empty list active = %d, want -1", lw.ActiveIndex())
	}
}
