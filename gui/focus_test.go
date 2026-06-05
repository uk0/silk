package gui

import (
	"testing"
)

// buildFocusTree wires three interactive widgets (Slider, Edit, CheckBox — all
// implement OnKeyDown) under a VBox in this visual order, and returns the root
// plus the three children. The VBox itself does not implement IEventKeyDown, so
// it is not focusable and only the three leaves should land in the focus chain.
// (Button is deliberately not used: it has no OnKeyDown, so the AutoFocus
// heuristic correctly excludes it — that exclusion is exercised separately.)
func buildFocusTree() (root *VBox, b *Slider, e *Edit, c *CheckBox) {
	root = NewVBox()
	b = NewSlider(0, 100)
	e = NewEdit()
	c = NewCheckBox()
	root.AddWidget(b)
	root.AddWidget(e)
	root.AddWidget(c)
	return
}

// sameWidget reports whether two IWidgets refer to the same underlying widget.
func sameWidget(a, b IWidget) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Self() == b.Self()
}

// TestNextFocusableFromNilForward: forward from nil yields the first focusable.
func TestNextFocusableFromNilForward(t *testing.T) {
	root, b, _, _ := buildFocusTree()
	got := nextFocusable(root, nil, true)
	if !sameWidget(got, b) {
		t.Fatalf("forward from nil = %v, want first focusable (Slider)", got)
	}
}

// TestNextFocusableFromNilBackward: backward from nil yields the last focusable.
func TestNextFocusableFromNilBackward(t *testing.T) {
	root, _, _, c := buildFocusTree()
	got := nextFocusable(root, nil, false)
	if !sameWidget(got, c) {
		t.Fatalf("backward from nil = %v, want last focusable (CheckBox)", got)
	}
}

// TestNextFocusableForwardCyclesAndWraps: forward visits Slider -> Edit ->
// CheckBox and then wraps back to Slider.
func TestNextFocusableForwardCyclesAndWraps(t *testing.T) {
	root, b, e, c := buildFocusTree()

	step1 := nextFocusable(root, b, true)
	if !sameWidget(step1, e) {
		t.Fatalf("forward from Slider = %v, want Edit", step1)
	}
	step2 := nextFocusable(root, e, true)
	if !sameWidget(step2, c) {
		t.Fatalf("forward from Edit = %v, want CheckBox", step2)
	}
	step3 := nextFocusable(root, c, true)
	if !sameWidget(step3, b) {
		t.Fatalf("forward from CheckBox = %v, want wrap to Slider", step3)
	}
}

// TestNextFocusableBackwardReverses: backward walks the chain in reverse and
// wraps from the first element to the last.
func TestNextFocusableBackwardReverses(t *testing.T) {
	root, b, e, c := buildFocusTree()

	step1 := nextFocusable(root, c, false)
	if !sameWidget(step1, e) {
		t.Fatalf("backward from CheckBox = %v, want Edit", step1)
	}
	step2 := nextFocusable(root, e, false)
	if !sameWidget(step2, b) {
		t.Fatalf("backward from Edit = %v, want Slider", step2)
	}
	step3 := nextFocusable(root, b, false)
	if !sameWidget(step3, c) {
		t.Fatalf("backward from Slider = %v, want wrap to CheckBox", step3)
	}
}

// TestNextFocusableSkipsNoFocus: a widget set to NoFocus is excluded from the
// chain even though it implements OnKeyDown, so traversal skips over it.
func TestNextFocusableSkipsNoFocus(t *testing.T) {
	root, b, e, c := buildFocusTree()
	e.SetFocusPolicy(NoFocus)

	// Forward from Slider should now skip Edit and land on CheckBox.
	got := nextFocusable(root, b, true)
	if !sameWidget(got, c) {
		t.Fatalf("forward from Slider with Edit=NoFocus = %v, want CheckBox", got)
	}
	// Edit must not appear anywhere: forward from CheckBox wraps to Slider.
	got = nextFocusable(root, c, true)
	if !sameWidget(got, b) {
		t.Fatalf("forward from CheckBox = %v, want wrap to Slider", got)
	}
}

// TestNextFocusableSkipsInvisibleAndDisabled: hidden and disabled widgets are
// not focus targets, so traversal skips them.
func TestNextFocusableSkipsInvisibleAndDisabled(t *testing.T) {
	root, b, e, c := buildFocusTree()
	e.SetVisible(false) // hidden
	c.SetEnabled(false) // disabled

	// Only Slider remains focusable -> forward from nil = Slider, and forward
	// from Slider wraps back to Slider.
	got := nextFocusable(root, nil, true)
	if !sameWidget(got, b) {
		t.Fatalf("only-Slider tree: forward from nil = %v, want Slider", got)
	}
	got = nextFocusable(root, b, true)
	if !sameWidget(got, b) {
		t.Fatalf("only-Slider tree: forward from Slider = %v, want self-wrap", got)
	}
}

// TestNextFocusableCurrentNotInChain: when current is not a focusable member
// (e.g. the container itself), forward returns the first focusable.
func TestNextFocusableCurrentNotInChain(t *testing.T) {
	root, b, _, _ := buildFocusTree()
	got := nextFocusable(root, root, true)
	if !sameWidget(got, b) {
		t.Fatalf("forward from non-member = %v, want first focusable (Slider)", got)
	}
}

// TestNextFocusableNoFocusableReturnsNil: a tree with no focusable widgets
// returns nil in both directions.
func TestNextFocusableNoFocusableReturnsNil(t *testing.T) {
	root := NewVBox()
	root.AddWidget(NewLabel("a")) // Label has no OnKeyDown -> not focusable
	root.AddWidget(NewLabel("b"))

	if got := nextFocusable(root, nil, true); got != nil {
		t.Fatalf("forward in label-only tree = %v, want nil", got)
	}
	if got := nextFocusable(root, nil, false); got != nil {
		t.Fatalf("backward in label-only tree = %v, want nil", got)
	}
}

// TestIsTabFocusablePredicate exercises the focusability predicate directly
// across the AutoFocus heuristic and the explicit TabFocus / NoFocus overrides.
func TestIsTabFocusablePredicate(t *testing.T) {
	// AutoFocus + implements OnKeyDown => focusable.
	if !isTabFocusable(NewSlider(0, 100)) {
		t.Fatalf("Slider (auto, OnKeyDown) should be focusable")
	}
	// AutoFocus + no OnKeyDown (Label) => not focusable.
	if isTabFocusable(NewLabel("x")) {
		t.Fatalf("Label (auto, no OnKeyDown) should not be focusable")
	}
	// AutoFocus + no OnKeyDown (Button) => not focusable. This is what lets us
	// avoid editing every widget: only key-handling widgets opt in by default.
	if isTabFocusable(NewButton()) {
		t.Fatalf("Button (auto, no OnKeyDown) should not be focusable")
	}
	// TabFocus on a Label opts it in even without OnKeyDown.
	lbl := NewLabel("x")
	lbl.SetFocusPolicy(TabFocus)
	if !isTabFocusable(lbl) {
		t.Fatalf("Label (TabFocus) should be focusable")
	}
	// NoFocus opts an interactive widget out.
	b := NewSlider(0, 100)
	b.SetFocusPolicy(NoFocus)
	if isTabFocusable(b) {
		t.Fatalf("Slider (NoFocus) should not be focusable")
	}
	// Hidden / disabled are never focusable.
	hidden := NewSlider(0, 100)
	hidden.SetVisible(false)
	if isTabFocusable(hidden) {
		t.Fatalf("hidden Slider should not be focusable")
	}
	disabled := NewSlider(0, 100)
	disabled.SetEnabled(false)
	if isTabFocusable(disabled) {
		t.Fatalf("disabled Slider should not be focusable")
	}
}
