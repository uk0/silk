package gui

import "testing"

// TestNextTabIndexWrap covers the pure wrap math used by keyboard tab
// switching: forward past the last tab returns to 0, backward past 0 returns
// to the last, and a single tab (or empty) is a no-op in both directions.
func TestNextTabIndexWrap(t *testing.T) {
	cases := []struct {
		name    string
		cur     int
		count   int
		forward bool
		want    int
	}{
		{"forward middle", 0, 3, true, 1},
		{"forward last wraps to first", 2, 3, true, 0},
		{"backward middle", 2, 3, false, 1},
		{"backward first wraps to last", 0, 3, false, 2},
		{"single tab forward no-op", 0, 1, true, 0},
		{"single tab backward no-op", 0, 1, false, 0},
		{"empty forward no-op", 0, 0, true, 0},
	}
	for _, c := range cases {
		if got := nextTabIndex(c.cur, c.count, c.forward); got != c.want {
			t.Errorf("%s: nextTabIndex(%d,%d,%v) = %d, want %d",
				c.name, c.cur, c.count, c.forward, got, c.want)
		}
	}
}

// newTabWidget3 builds a TabWidget with three pages for the API-level tests.
func newTabWidget3() *TabWidget {
	tw := NewTabWidget()
	tw.AddTab(NewLabel("one"), "One", nil)
	tw.AddTab(NewLabel("two"), "Two", nil)
	tw.AddTab(NewLabel("three"), "Three", nil)
	return tw
}

// TestStepCurrentForwardWraps drives the public TabWidget API through the
// delegated step method that OnKeyDown calls for Ctrl+PageDown / Ctrl+Tab.
// Going forward from the last tab must wrap back to the first.
//
// Modifier note: OnKeyDown reads live Ctrl/Shift state via IsKeyDown, which a
// unit test cannot reliably set (it polls real OS / window key state). So the
// key->action wiring is verified by exercising stepCurrent, the exact method
// OnKeyDown delegates to once it has decided the direction. See OnKeyDown.
func TestTabWidgetStepForwardWraps(t *testing.T) {
	tw := newTabWidget3()
	tw.SetCurrentIndex(0)

	tw.stepCurrent(true)
	if tw.CurrentIndex() != 1 {
		t.Fatalf("after forward step: CurrentIndex = %d, want 1", tw.CurrentIndex())
	}
	tw.stepCurrent(true)
	if tw.CurrentIndex() != 2 {
		t.Fatalf("after second forward step: CurrentIndex = %d, want 2", tw.CurrentIndex())
	}
	tw.stepCurrent(true) // wrap
	if tw.CurrentIndex() != 0 {
		t.Fatalf("forward from last must wrap: CurrentIndex = %d, want 0", tw.CurrentIndex())
	}
}

// TestStepCurrentBackwardWraps mirrors the forward case for Ctrl+PageUp /
// Ctrl+Shift+Tab: going backward from the first tab wraps to the last.
func TestTabWidgetStepBackwardWraps(t *testing.T) {
	tw := newTabWidget3()
	tw.SetCurrentIndex(0)

	tw.stepCurrent(false) // wrap to last
	if tw.CurrentIndex() != 2 {
		t.Fatalf("backward from first must wrap: CurrentIndex = %d, want 2", tw.CurrentIndex())
	}
	tw.stepCurrent(false)
	if tw.CurrentIndex() != 1 {
		t.Fatalf("after backward step: CurrentIndex = %d, want 1", tw.CurrentIndex())
	}
}

// TestStepCurrentFiresCurrentChanged confirms the keyboard path routes through
// SetCurrentIndex so the current-changed callback fires (with the new index)
// exactly as a click would. SetCurrentIndex is pre-existing and notifies twice
// per change -- once via the TabBar activate callback and once directly -- so
// this asserts the callback fired and reported the right index rather than
// pinning a count that would contradict the click path's own behaviour.
func TestTabWidgetStepFiresCurrentChanged(t *testing.T) {
	tw := newTabWidget3()
	tw.SetCurrentIndex(0)

	var gotIdx int
	var fired bool
	tw.SetCurrentChangedCallback(func(_ interface{}, idx int) {
		gotIdx = idx
		fired = true
	})

	tw.stepCurrent(true)
	if !fired {
		t.Fatalf("current-changed callback did not fire on keyboard step")
	}
	if gotIdx != 1 {
		t.Fatalf("current-changed reported index %d, want 1", gotIdx)
	}
}

// TestStepCurrentSingleTabNoOp ensures the keyboard path does nothing when
// there are fewer than two tabs.
func TestTabWidgetStepSingleTabNoOp(t *testing.T) {
	tw := NewTabWidget()
	tw.AddTab(NewLabel("only"), "Only", nil)
	tw.SetCurrentIndex(0)

	tw.stepCurrent(true)
	if tw.CurrentIndex() != 0 {
		t.Fatalf("single-tab forward step changed index to %d, want 0", tw.CurrentIndex())
	}
	tw.stepCurrent(false)
	if tw.CurrentIndex() != 0 {
		t.Fatalf("single-tab backward step changed index to %d, want 0", tw.CurrentIndex())
	}
}

// TestOnKeyDownArrowsNoCtrl exercises OnKeyDown directly for the plain-arrow
// case (no modifier required), which moves between tabs when the strip holds
// focus. Right advances; Left goes back with wrap. These keys do not consult
// IsKeyDown, so they are fully testable here.
func TestTabWidgetOnKeyDownArrowsNoCtrl(t *testing.T) {
	tw := newTabWidget3()
	tw.SetCurrentIndex(0)

	tw.OnKeyDown(KeyRight, false)
	if tw.CurrentIndex() != 1 {
		t.Fatalf("after KeyRight: CurrentIndex = %d, want 1", tw.CurrentIndex())
	}
	tw.OnKeyDown(KeyLeft, false) // wrap back to last? no: 1 -> 0
	if tw.CurrentIndex() != 0 {
		t.Fatalf("after KeyLeft: CurrentIndex = %d, want 0", tw.CurrentIndex())
	}
	tw.OnKeyDown(KeyLeft, false) // 0 -> wrap to 2
	if tw.CurrentIndex() != 2 {
		t.Fatalf("KeyLeft from first must wrap: CurrentIndex = %d, want 2", tw.CurrentIndex())
	}
}
