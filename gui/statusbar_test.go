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

// TestStatusIconLabelStored verifies an icon-label cell added via AddIconLabel is
// stored as a permanent widget and that its icon + text round-trip through the
// getters.
func TestStatusIconLabelStored(t *testing.T) {
	sb := NewStatusBar()
	cell := sb.AddIconLabel("git-branch", "main")

	if got := len(sb.PermanentWidgets()); got != 1 {
		t.Fatalf("PermanentWidgets len = %d, want 1", got)
	}
	if sb.PermanentWidgets()[0] != cell {
		t.Fatalf("stored widget is not the returned cell")
	}
	if cell.Icon() != "git-branch" {
		t.Fatalf("Icon() = %q, want %q", cell.Icon(), "git-branch")
	}
	if cell.Text() != "main" {
		t.Fatalf("Text() = %q, want %q", cell.Text(), "main")
	}
}

// TestStatusIconLabelSetters verifies SetIcon / SetText update the model.
func TestStatusIconLabelSetters(t *testing.T) {
	cell := NewStatusIconLabel("warning", "2")
	cell.SetIcon("error")
	cell.SetText("3")
	if cell.Icon() != "error" || cell.Text() != "3" {
		t.Fatalf("after setters Icon/Text = %q/%q, want error/3", cell.Icon(), cell.Text())
	}
}

// TestStatusIconLabelCoexistsWithPlainWidget verifies icon-label cells and plain
// permanent widgets share the same list without disturbing each other.
func TestStatusIconLabelCoexistsWithPlainWidget(t *testing.T) {
	sb := NewStatusBar()
	plain := newSBTestWidget(40, 16)
	sb.AddPermanentWidget(plain)
	cell := sb.AddIconLabel("error", "1")

	pw := sb.PermanentWidgets()
	if len(pw) != 2 {
		t.Fatalf("PermanentWidgets len = %d, want 2", len(pw))
	}
	if pw[0] != plain || pw[1] != cell {
		t.Fatalf("permanent widget order/identity wrong")
	}
}

// TestStatusIconLabelWidth checks the pure width helper across the presence
// combinations: the icon→text gap is added only when both parts are present.
func TestStatusIconLabelWidth(t *testing.T) {
	const iconW, gap, textW = 14.0, 4.0, 30.0
	cases := []struct {
		name             string
		hasIcon, hasText bool
		want             float64
	}{
		{"both", true, true, iconW + gap + textW},
		{"text only", false, true, textW},
		{"icon only", true, false, iconW},
		{"neither", false, false, 0},
	}
	for _, c := range cases {
		if got := statusIconLabelWidth(iconW, gap, textW, c.hasIcon, c.hasText); got != c.want {
			t.Fatalf("%s: statusIconLabelWidth = %v, want %v", c.name, got, c.want)
		}
	}
}
