package gui

import "testing"

// newAccordion3 builds an Accordion with three sections for the keyboard tests.
// AddSection expands the first section by default and collapses the rest, which
// matches the production constructor behaviour the navigation tests rely on.
func newAccordion3() *Accordion {
	a := NewAccordion()
	a.AddSection("One", NewLabel("c1"))
	a.AddSection("Two", NewLabel("c2"))
	a.AddSection("Three", NewLabel("c3"))
	return a
}

// expandedFlags snapshots the per-section Expanded state so tests can assert on
// the whole accordion at once (single-expand keeps exactly one true).
func expandedFlags(a *Accordion) []bool {
	out := make([]bool, len(a.sections))
	for i := range a.sections {
		out[i] = a.sections[i].Expanded
	}
	return out
}

// TestAccordionMoveCurrent covers the pure navigation math used by OnKeyDown:
// Down/Up step within [0,n-1] without wrapping and clamp at the ends, Home/End
// jump to first/last, a -1 (no current) start resolves sensibly per key, and an
// empty accordion yields -1 for every key.
func TestAccordionMoveCurrent(t *testing.T) {
	cases := []struct {
		name string
		cur  int
		n    int
		key  int
		want int
	}{
		{"down middle", 0, 3, KeyDown, 1},
		{"down clamps at last", 2, 3, KeyDown, 2},
		{"up middle", 2, 3, KeyUp, 1},
		{"up clamps at first", 0, 3, KeyUp, 0},
		{"home", 2, 3, KeyHome, 0},
		{"end", 0, 3, KeyEnd, 2},
		{"down from none", -1, 3, KeyDown, 0},
		{"up from none", -1, 3, KeyUp, 2},
		{"home from none", -1, 3, KeyHome, 0},
		{"end from none", -1, 3, KeyEnd, 2},
		{"unrelated key keeps current", 1, 3, KeyEnter, 1},
		{"empty down", 0, 0, KeyDown, -1},
		{"empty home", -1, 0, KeyHome, -1},
	}
	for _, c := range cases {
		if got := accordionMoveCurrent(c.cur, c.n, c.key); got != c.want {
			t.Errorf("%s: accordionMoveCurrent(%d,%d,%#x) = %d, want %d",
				c.name, c.cur, c.n, c.key, got, c.want)
		}
	}
}

// TestAccordionKeyMovesCurrent drives the public OnKeyDown entry point and
// checks that Down/Up/Home/End update curIdx with end-clamping (no wrap).
func TestAccordionKeyMovesCurrent(t *testing.T) {
	a := newAccordion3()
	a.curIdx = 0

	a.OnKeyDown(KeyDown, false)
	if a.curIdx != 1 {
		t.Fatalf("after Down: curIdx = %d, want 1", a.curIdx)
	}
	a.OnKeyDown(KeyDown, false)
	a.OnKeyDown(KeyDown, false) // clamp at last
	if a.curIdx != 2 {
		t.Fatalf("Down must clamp at last: curIdx = %d, want 2", a.curIdx)
	}
	a.OnKeyDown(KeyUp, false)
	if a.curIdx != 1 {
		t.Fatalf("after Up: curIdx = %d, want 1", a.curIdx)
	}
	a.OnKeyDown(KeyHome, false)
	if a.curIdx != 0 {
		t.Fatalf("Home must go to first: curIdx = %d, want 0", a.curIdx)
	}
	a.OnKeyDown(KeyUp, false) // clamp at first
	if a.curIdx != 0 {
		t.Fatalf("Up must clamp at first: curIdx = %d, want 0", a.curIdx)
	}
	a.OnKeyDown(KeyEnd, false)
	if a.curIdx != 2 {
		t.Fatalf("End must go to last: curIdx = %d, want 2", a.curIdx)
	}
}

// TestAccordionEnterTogglesSingleExpand verifies Enter/Space collapse and expand
// the current section through ToggleSection, preserving single-expand semantics:
// expanding a new section collapses whatever was open before.
func TestAccordionEnterTogglesSingleExpand(t *testing.T) {
	a := newAccordion3() // single-expand by default; section 0 starts expanded

	// Section 0 is current and expanded -> Enter collapses it.
	a.curIdx = 0
	a.OnKeyDown(KeyEnter, false)
	if a.sections[0].Expanded {
		t.Fatalf("Enter on expanded section 0 should collapse it, flags=%v", expandedFlags(a))
	}

	// Move to section 1 and Space-expand it.
	a.curIdx = 1
	a.OnKeyDown(KeySpace, false)
	if !a.sections[1].Expanded {
		t.Fatalf("Space on collapsed section 1 should expand it, flags=%v", expandedFlags(a))
	}

	// Single-expand: opening section 2 must collapse section 1.
	a.curIdx = 2
	a.OnKeyDown(KeyEnter, false)
	flags := expandedFlags(a)
	if !flags[2] || flags[1] || flags[0] {
		t.Fatalf("single-expand: only section 2 should be open, flags=%v", flags)
	}
}

// TestAccordionEnterTogglesMultiExpand verifies that with multi-expand on,
// Enter/Space toggle each current section independently (others stay put).
func TestAccordionEnterTogglesMultiExpand(t *testing.T) {
	a := newAccordion3()
	a.SetMultiExpand(true) // section 0 still starts expanded from AddSection

	// Open section 1 alongside the already-open section 0.
	a.curIdx = 1
	a.OnKeyDown(KeyEnter, false)
	if !a.sections[1].Expanded || !a.sections[0].Expanded {
		t.Fatalf("multi-expand: sections 0 and 1 should both be open, flags=%v", expandedFlags(a))
	}

	// Toggle section 0 closed; section 1 must remain open.
	a.curIdx = 0
	a.OnKeyDown(KeySpace, false)
	flags := expandedFlags(a)
	if flags[0] || !flags[1] {
		t.Fatalf("multi-expand: section 0 closed, section 1 still open expected, flags=%v", flags)
	}
}

// TestAccordionEnterFromNoCurrent confirms that pressing Enter before any
// keyboard navigation falls back to the first section and toggles it.
func TestAccordionEnterFromNoCurrent(t *testing.T) {
	a := newAccordion3() // section 0 starts expanded
	a.curIdx = -1

	a.OnKeyDown(KeyEnter, false)
	if a.curIdx != 0 {
		t.Fatalf("Enter from no current should adopt section 0, curIdx = %d", a.curIdx)
	}
	if a.sections[0].Expanded {
		t.Fatalf("Enter should have collapsed the initially-open section 0, flags=%v", expandedFlags(a))
	}
}

// TestAccordionKeyEmptyNoPanic ensures key handling on an empty accordion is a
// safe no-op (no panic, current stays -1).
func TestAccordionKeyEmptyNoPanic(t *testing.T) {
	a := NewAccordion()
	a.OnKeyDown(KeyDown, false)
	a.OnKeyDown(KeyEnter, false)
	if a.curIdx != -1 {
		t.Fatalf("empty accordion should keep curIdx = -1, got %d", a.curIdx)
	}
}
