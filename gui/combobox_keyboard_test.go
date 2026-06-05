package gui

import "testing"

// newTestCombo builds a ComboBox populated with the given item texts.
func newTestCombo(texts ...string) *ComboBox {
	cb := NewComboBox()
	for _, s := range texts {
		cb.Append(ListItem{Text: s})
	}
	return cb
}

func TestComboBoxNextMatchIndex(t *testing.T) {
	items := []string{"Apple", "Apricot", "Banana", "Cherry"}

	// First match strictly after start.
	if got := nextMatchIndex(items, -1, 'a'); got != 0 {
		t.Errorf("first 'a' from -1 = %d, want 0", got)
	}
	if got := nextMatchIndex(items, 0, 'a'); got != 1 {
		t.Errorf("next 'a' from 0 = %d, want 1", got)
	}

	// Wrap-around / cycling: from the last 'a' match, the next 'a' is index 0.
	if got := nextMatchIndex(items, 1, 'a'); got != 0 {
		t.Errorf("next 'a' from 1 = %d, want 0 (wrap)", got)
	}

	// Case-insensitive on both sides.
	if got := nextMatchIndex(items, -1, 'B'); got != 2 {
		t.Errorf("'B' from -1 = %d, want 2", got)
	}
	if got := nextMatchIndex([]string{"apple"}, -1, 'A'); got != 0 {
		t.Errorf("'A' vs lowercase item = %d, want 0", got)
	}

	// No match returns -1.
	if got := nextMatchIndex(items, -1, 'z'); got != -1 {
		t.Errorf("no-match 'z' = %d, want -1", got)
	}

	// Empty list returns -1.
	if got := nextMatchIndex(nil, 0, 'a'); got != -1 {
		t.Errorf("empty list = %d, want -1", got)
	}
}

func TestComboBoxClampIndex(t *testing.T) {
	cases := []struct{ i, n, want int }{
		{-5, 3, 0},
		{0, 3, 0},
		{2, 3, 2},
		{9, 3, 2},
		{0, 0, -1},
	}
	for _, c := range cases {
		if got := clampIndex(c.i, c.n); got != c.want {
			t.Errorf("clampIndex(%d,%d) = %d, want %d", c.i, c.n, got, c.want)
		}
	}
}

// Closed dropdown: Down/Up change the committed selection directly and clamp.
func TestComboBoxKeyNavClosed(t *testing.T) {
	cb := newTestCombo("Apple", "Banana", "Cherry")
	if cb.ActiveIndex() != -1 {
		t.Fatalf("initial ActiveIndex = %d, want -1", cb.ActiveIndex())
	}

	cb.OnKeyDown(KeyDown, false) // -1 -> 0
	if cb.ActiveIndex() != 0 {
		t.Errorf("after Down ActiveIndex = %d, want 0", cb.ActiveIndex())
	}
	cb.OnKeyDown(KeyDown, false) // 0 -> 1
	if cb.ActiveIndex() != 1 {
		t.Errorf("after Down ActiveIndex = %d, want 1", cb.ActiveIndex())
	}
	cb.OnKeyDown(KeyUp, false) // 1 -> 0
	if cb.ActiveIndex() != 0 {
		t.Errorf("after Up ActiveIndex = %d, want 0", cb.ActiveIndex())
	}
	cb.OnKeyDown(KeyUp, false) // clamp at 0
	if cb.ActiveIndex() != 0 {
		t.Errorf("Up at top ActiveIndex = %d, want 0 (clamped)", cb.ActiveIndex())
	}

	cb.OnKeyDown(KeyEnd, false) // jump to last
	if cb.ActiveIndex() != 2 {
		t.Errorf("after End ActiveIndex = %d, want 2", cb.ActiveIndex())
	}
	cb.OnKeyDown(KeyDown, false) // clamp at last
	if cb.ActiveIndex() != 2 {
		t.Errorf("Down at bottom ActiveIndex = %d, want 2 (clamped)", cb.ActiveIndex())
	}
	cb.OnKeyDown(KeyHome, false) // jump to first
	if cb.ActiveIndex() != 0 {
		t.Errorf("after Home ActiveIndex = %d, want 0", cb.ActiveIndex())
	}
}

// Open dropdown: Up/Down move the highlight without committing the selection.
func TestComboBoxKeyNavOpen(t *testing.T) {
	cb := newTestCombo("Apple", "Banana", "Cherry")
	cb.sub.SetVisible(true) // simulate an open popup without a real window
	cb.sub.activeIndex = 0

	cb.OnKeyDown(KeyDown, false) // highlight 0 -> 1
	if cb.sub.ActiveIndex() != 1 {
		t.Errorf("highlight after Down = %d, want 1", cb.sub.ActiveIndex())
	}
	if cb.ActiveIndex() != -1 {
		t.Errorf("committed selection moved while open = %d, want -1", cb.ActiveIndex())
	}
	cb.OnKeyDown(KeyEnd, false)
	if cb.sub.ActiveIndex() != 2 {
		t.Errorf("highlight after End = %d, want 2", cb.sub.ActiveIndex())
	}
}

// Enter commits the highlighted row, fires the change callback and closes.
func TestComboBoxKeyEnterCommits(t *testing.T) {
	cb := newTestCombo("Apple", "Banana", "Cherry")
	cb.sub.SetVisible(true)
	cb.sub.activeIndex = 2

	fired := -1
	cb.SigSelectionChanged(func(o interface{}, idx int) { fired = idx })

	cb.OnKeyDown(KeyEnter, false)

	if cb.ActiveIndex() != 2 {
		t.Errorf("after Enter ActiveIndex = %d, want 2", cb.ActiveIndex())
	}
	if fired != 2 {
		t.Errorf("change callback fired with %d, want 2", fired)
	}
	if cb.IsSubPopupVisible() {
		t.Error("popup still visible after Enter, want closed")
	}
}

// Esc closes the popup without changing the committed selection.
func TestComboBoxKeyEscNoChange(t *testing.T) {
	cb := newTestCombo("Apple", "Banana", "Cherry")
	cb.OnKeyDown(KeyDown, false) // commit index 0 while closed
	cb.sub.SetVisible(true)
	cb.sub.activeIndex = 2 // highlight a different row

	fired := false
	cb.SigSelectionChanged(func(o interface{}, idx int) { fired = true })

	cb.OnKeyDown(KeyEsc, false)

	if cb.IsSubPopupVisible() {
		t.Error("popup still visible after Esc, want closed")
	}
	if cb.ActiveIndex() != 0 {
		t.Errorf("selection changed by Esc = %d, want 0", cb.ActiveIndex())
	}
	if fired {
		t.Error("change callback fired on Esc, want no fire")
	}
}

// Type-ahead jumps to the next item starting with the typed letter.
func TestComboBoxTypeAhead(t *testing.T) {
	cb := newTestCombo("Apple", "Apricot", "Banana", "Cherry")

	cb.OnKeyDown('B', false) // closed: jump to "Banana"
	if cb.ActiveIndex() != 2 {
		t.Errorf("type-ahead 'B' ActiveIndex = %d, want 2", cb.ActiveIndex())
	}

	cb.OnKeyDown('A', false) // case-insensitive, first "A*" after current -> wrap to 0
	if cb.ActiveIndex() != 0 {
		t.Errorf("type-ahead 'A' ActiveIndex = %d, want 0", cb.ActiveIndex())
	}
	cb.OnKeyDown('A', false) // repeated -> next "A*" match (1)
	if cb.ActiveIndex() != 1 {
		t.Errorf("repeated 'A' ActiveIndex = %d, want 1", cb.ActiveIndex())
	}

	// No match leaves the selection untouched.
	cb.OnKeyDown('Z', false)
	if cb.ActiveIndex() != 1 {
		t.Errorf("no-match 'Z' ActiveIndex = %d, want 1 (unchanged)", cb.ActiveIndex())
	}
}
