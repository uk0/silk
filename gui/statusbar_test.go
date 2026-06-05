package gui

import (
	"testing"
)

// sbTestWidget is a fixed-size widget used to make StatusBar layout positions
// deterministic in tests.
type sbTestWidget struct {
	Widget
	w, h float64
}

func newSBTestWidget(w, h float64) *sbTestWidget {
	p := &sbTestWidget{w: w, h: h}
	p.Init(p)
	return p
}

func (this *sbTestWidget) SizeHints() SizeHints {
	return SizeHints{Width: this.w, Height: this.h, Policy: 0}
}

// TestStatusBarLayoutSkipsHidden verifies that a hidden permanent widget
// reserves no space: visible widgets stay contiguous and the hidden one is
// not repositioned.
func TestStatusBarLayoutSkipsHidden(t *testing.T) {
	const ww, wh = 40.0, 16.0
	sb := NewStatusBar()
	sb.SetSize(400, 24)

	a := newSBTestWidget(ww, wh) // i=0, leftmost when laid out
	mid := newSBTestWidget(ww, wh)
	c := newSBTestWidget(ww, wh) // i=2, rightmost
	sb.AddPermanentWidget(a)
	sb.AddPermanentWidget(mid)
	sb.AddPermanentWidget(c)

	// Park the middle widget at a sentinel position so we can prove Layout()
	// leaves it untouched once hidden.
	mid.SetBounds(-999, -999, 1, 1)
	mid.SetVisible(false)

	sb.Layout()

	cx, _, _, _ := c.Bounds()
	ax, _, _, _ := a.Bounds()

	// The two visible widgets must be contiguous: the gap between their left
	// edges equals one widget width plus one spacing (i.e. the hidden widget
	// reserved zero space).
	wantGap := ww + sb.spacing
	if gotGap := cx - ax; gotGap != wantGap {
		t.Fatalf("visible widgets not contiguous: gap a->c = %v, want %v (hidden widget reserved space)", gotGap, wantGap)
	}

	// The hidden widget must not have been advanced/positioned.
	mx, my, mw, mh := mid.Bounds()
	if mx != -999 || my != -999 || mw != 1 || mh != 1 {
		t.Fatalf("hidden widget was repositioned by Layout: bounds=(%v,%v,%v,%v)", mx, my, mw, mh)
	}
}

// TestStatusBarSizeHintsExcludesHidden verifies the permanent-width sum in
// SizeHints() drops by exactly one widget's contribution when it is hidden.
func TestStatusBarSizeHintsExcludesHidden(t *testing.T) {
	const ww, wh = 40.0, 16.0
	sb := NewStatusBar()

	a := newSBTestWidget(ww, wh)
	mid := newSBTestWidget(ww, wh)
	c := newSBTestWidget(ww, wh)
	sb.AddPermanentWidget(a)
	sb.AddPermanentWidget(mid)
	sb.AddPermanentWidget(c)

	all := sb.SizeHints().Width
	mid.SetVisible(false)
	hidden := sb.SizeHints().Width

	// One widget hidden removes its width plus its spacing from the sum.
	wantDrop := ww + sb.spacing
	if gotDrop := all - hidden; gotDrop != wantDrop {
		t.Fatalf("SizeHints width drop = %v, want %v (hidden widget still counted)", gotDrop, wantDrop)
	}
}

// TestStatusBarShowMessageFor verifies the timed transient message is set
// immediately and that a later timed message replaces the first.
//
// The message timeout is driven by the UI-thread Timer (fired from MainLoop's
// processTimers), which cannot be advanced deterministically in a unit test,
// so we assert the synchronous contract: the message is SET right away and a
// second ShowMessageFor replaces it (the pending timer is stopped/replaced).
func TestStatusBarShowMessageFor(t *testing.T) {
	sb := NewStatusBar()

	sb.ShowMessageFor("loading", 5000)
	if got := sb.Message(); got != "loading" {
		t.Fatalf("Message() = %q immediately after ShowMessageFor, want %q", got, "loading")
	}

	// A second timed message replaces the first (and replaces its pending timer).
	sb.ShowMessageFor("done", 5000)
	if got := sb.Message(); got != "done" {
		t.Fatalf("Message() = %q after second ShowMessageFor, want %q", got, "done")
	}

	// An untimed ShowMessage still works and cancels any pending timer.
	sb.ShowMessage("static")
	if got := sb.Message(); got != "static" {
		t.Fatalf("Message() = %q after ShowMessage, want %q", got, "static")
	}
}
