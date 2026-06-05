package gui

import "testing"

// kinds builds a []menuItemKind from a boolean selectability pattern, so the
// pure-helper tests read like the menu they describe (true = a normal item,
// false = a separator or disabled item).
func kinds(sel ...bool) []menuItemKind {
	out := make([]menuItemKind, len(sel))
	for i, s := range sel {
		out[i].selectable = s
	}
	return out
}

// TestNextSelectableIndexSkips checks that the pure helper hops over the
// non-selectable items (separators / disabled) in both directions.
func TestNextSelectableIndexSkips(t *testing.T) {
	// index:        0      1      2      3      4
	// item:        btn    sep    dis    btn    btn
	k := kinds(true, false, false, true, true)

	// Down from the first item skips the separator and the disabled item.
	if got := nextSelectableIndex(k, 0, +1, true); got != 3 {
		t.Errorf("down from 0 = %d, want 3 (skip sep+disabled)", got)
	}
	// Up from index 3 skips back over disabled+separator to index 0.
	if got := nextSelectableIndex(k, 3, -1, true); got != 0 {
		t.Errorf("up from 3 = %d, want 0 (skip disabled+sep)", got)
	}
	// Down between two adjacent selectable items.
	if got := nextSelectableIndex(k, 3, +1, true); got != 4 {
		t.Errorf("down from 3 = %d, want 4", got)
	}
}

// TestNextSelectableIndexWrap checks Qt-style wrap-around at both ends.
func TestNextSelectableIndexWrap(t *testing.T) {
	// index:        0      1      2
	// item:        btn    sep    btn
	k := kinds(true, false, true)

	// Down from the last selectable wraps to the first.
	if got := nextSelectableIndex(k, 2, +1, true); got != 0 {
		t.Errorf("down from last = %d, want 0 (wrap)", got)
	}
	// Up from the first selectable wraps to the last.
	if got := nextSelectableIndex(k, 0, -1, true); got != 2 {
		t.Errorf("up from first = %d, want 2 (wrap)", got)
	}
	// From "nothing highlighted" (-1): down -> first, up -> last.
	if got := nextSelectableIndex(k, -1, +1, true); got != 0 {
		t.Errorf("down from -1 = %d, want 0", got)
	}
	if got := nextSelectableIndex(k, -1, -1, true); got != 2 {
		t.Errorf("up from -1 = %d, want 2", got)
	}
}

// TestNextSelectableIndexClamp covers the non-wrap (Home/End) path: walking
// off either end returns -1 rather than wrapping.
func TestNextSelectableIndexClamp(t *testing.T) {
	k := kinds(true, false, true)

	// End: start past the end, step up -> last selectable (index 2).
	if got := nextSelectableIndex(k, len(k), -1, false); got != 2 {
		t.Errorf("End = %d, want 2", got)
	}
	// Home: start before the start, step down -> first selectable (index 0).
	if got := nextSelectableIndex(k, -1, +1, false); got != 0 {
		t.Errorf("Home = %d, want 0", got)
	}
	// No wrap: down from the last selectable finds nothing more -> -1.
	if got := nextSelectableIndex(k, 2, +1, false); got != -1 {
		t.Errorf("down from last (no wrap) = %d, want -1", got)
	}
}

// TestNextSelectableIndexNone covers degenerate inputs.
func TestNextSelectableIndexNone(t *testing.T) {
	// All non-selectable -> no target in any direction.
	allDis := kinds(false, false, false)
	if got := nextSelectableIndex(allDis, -1, +1, true); got != -1 {
		t.Errorf("all-disabled down = %d, want -1", got)
	}
	if got := nextSelectableIndex(allDis, 1, -1, true); got != -1 {
		t.Errorf("all-disabled up = %d, want -1", got)
	}
	// Empty menu.
	if got := nextSelectableIndex(nil, -1, +1, true); got != -1 {
		t.Errorf("empty = %d, want -1", got)
	}
	// dir==0 is a no-op guard.
	if got := nextSelectableIndex(kinds(true), 0, 0, true); got != -1 {
		t.Errorf("dir=0 = %d, want -1", got)
	}
}

// newNavMenu builds a popup menu shaped: btn0, separator, btn2(disabled),
// btn3, returning the menu and its buttons for assertions.
func newNavMenu() (*Menu, *Button, *Button, *Button) {
	m := NewPopupMenu()
	b0 := m.AddButton1("Item0", nil)
	m.AddSeparator()
	b2 := m.AddButton1("Item2", nil)
	b2.SetEnabled(false)
	b3 := m.AddButton1("Item3", nil)
	return m, b0, b2, b3
}

// TestMenuKeyboardDownUpSkips drives the public Menu API: Down/Up must land the
// keyboard highlight only on selectable items, skipping the separator and the
// disabled item, and wrap at the ends.
func TestMenuKeyboardDownUpSkips(t *testing.T) {
	m, _, _, _ := newNavMenu()

	// Down from nothing -> first selectable (index 0).
	m.OnKeyDown(KeyDown, false)
	if m.highlight != 0 {
		t.Fatalf("after Down#1 highlight = %d, want 0", m.highlight)
	}
	// Down -> skip separator(1) and disabled(2) -> index 3.
	m.OnKeyDown(KeyDown, false)
	if m.highlight != 3 {
		t.Fatalf("after Down#2 highlight = %d, want 3 (skip sep+disabled)", m.highlight)
	}
	// Down from last -> wrap to first.
	m.OnKeyDown(KeyDown, false)
	if m.highlight != 0 {
		t.Fatalf("after Down#3 highlight = %d, want 0 (wrap)", m.highlight)
	}
	// Up from first -> wrap to last selectable (3).
	m.OnKeyDown(KeyUp, false)
	if m.highlight != 3 {
		t.Fatalf("after Up highlight = %d, want 3 (wrap)", m.highlight)
	}
}

// TestMenuKeyboardHomeEnd checks Home/End jump to the first/last selectable.
func TestMenuKeyboardHomeEnd(t *testing.T) {
	m, _, _, _ := newNavMenu()

	m.OnKeyDown(KeyEnd, false)
	if m.highlight != 3 {
		t.Fatalf("after End highlight = %d, want 3", m.highlight)
	}
	m.OnKeyDown(KeyHome, false)
	if m.highlight != 0 {
		t.Fatalf("after Home highlight = %d, want 0", m.highlight)
	}
}

// TestMenuKeyboardEnterTriggers verifies Enter fires the highlighted item's
// action callback via the same emit path a click uses.
func TestMenuKeyboardEnterTriggers(t *testing.T) {
	m, b0, _, b3 := newNavMenu()

	fired0, fired3 := 0, 0
	b0.Action().BindFunc0(func() { fired0++ })
	b3.Action().BindFunc0(func() { fired3++ })

	// Highlight the last item (index 3) and press Enter.
	m.OnKeyDown(KeyEnd, false)
	m.OnKeyDown(KeyEnter, false)
	if fired3 != 1 {
		t.Fatalf("Enter on Item3: callback fired %d times, want 1", fired3)
	}
	if fired0 != 0 {
		t.Fatalf("Item0 callback fired %d times, want 0", fired0)
	}

	// Space activates too: move highlight to the first item and press Space.
	m.OnKeyDown(KeyHome, false)
	m.OnKeyDown(KeySpace, false)
	if fired0 != 1 {
		t.Fatalf("Space on Item0: callback fired %d times, want 1", fired0)
	}
}

// TestMenuKeyboardEnterOnDisabledNoop confirms a disabled item cannot be
// activated and is never the keyboard highlight target.
func TestMenuKeyboardEnterOnDisabledNoop(t *testing.T) {
	m, _, b2, _ := newNavMenu()

	fired := 0
	b2.Action().BindFunc0(func() { fired++ })

	// Force the (disabled) index and press Enter — must not fire.
	m.highlight = 2
	m.OnKeyDown(KeyEnter, false)
	if fired != 0 {
		t.Fatalf("Enter on disabled item fired %d times, want 0", fired)
	}
}
