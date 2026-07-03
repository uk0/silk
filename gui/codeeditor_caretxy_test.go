package gui

import (
	"math"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Caret coordinate API (pure, no GL / no Draw)
//
// caretLocalXY mirrors the primary-cursor math in Draw(): x is the caret's
// left edge, y is the BOTTOM of the caret's line, viewport-clamped. These
// tests derive expectations from the editor's own font metrics — the same
// lh / topOffset idiom as the breakpoint gutter-click test — so they stay
// robust to font differences.
// ---------------------------------------------------------------------------

func TestCodeEditorCaretLocalXY(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText("hello\nworld")

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()

	e.cursorLine = 1
	e.cursorCol = 3
	x, y := e.caretLocalXY()

	// y is the bottom of line 1: topOff + 2*lh (scrollY == 0).
	wantY := topOff + 2*lh
	if math.Abs(y-wantY) > 0.5 {
		t.Errorf("caretLocalXY y = %v, want ~%v (topOff + 2*lh)", y, wantY)
	}
	// x carries the advance of "wor" past the gutter text start.
	if x <= e.gutterW {
		t.Errorf("caretLocalXY x = %v, want > gutterW (%v)", x, e.gutterW)
	}
	wantX := e.gutterW + 10 + e.measureText("wor")
	if math.Abs(x-wantX) > 0.5 {
		t.Errorf("caretLocalXY x = %v, want ~%v (gutterW + 10 + advance of %q)", x, wantX, "wor")
	}
}

func TestCodeEditorCaretLocalXYOrigin(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText("hello\nworld")

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()

	// SetText leaves the caret at (0,0) with zero scroll.
	x, y := e.caretLocalXY()
	if wantX := e.gutterW + 10; math.Abs(x-wantX) > 0.5 {
		t.Errorf("caret (0,0) x = %v, want %v (gutter text start)", x, wantX)
	}
	if wantY := topOff + lh; math.Abs(y-wantY) > 0.5 {
		t.Errorf("caret (0,0) y = %v, want %v (bottom of line 0)", y, wantY)
	}
}

func TestCodeEditorCaretLocalXYScrollClamp(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText(strings.Repeat("line\n", 99) + "line")

	// Scroll far down, then put the caret back on line 0 so it sits above the
	// viewport (ScrollToLine also moves the cursor; reset it afterwards).
	e.ScrollToLine(90)
	if e.scrollY <= 0 {
		t.Fatalf("setup: ScrollToLine(90) left scrollY = %v, want > 0", e.scrollY)
	}
	e.cursorLine = 0
	e.cursorCol = 0

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()
	x, y := e.caretLocalXY()
	if y < 0 {
		t.Errorf("scrolled-out caret y = %v, want clamped >= 0", y)
	}
	if wantY := topOff + lh; math.Abs(y-wantY) > 0.5 {
		t.Errorf("scrolled-out caret y = %v, want clamped to %v (first visible line bottom)", y, wantY)
	}
	if x < e.gutterW {
		t.Errorf("scrolled-out caret x = %v, want >= gutterW (%v)", x, e.gutterW)
	}
}

func TestCodeEditorCaretGlobalXYHeadless(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText("hello\nworld")
	e.cursorLine = 1
	e.cursorCol = 2

	// Unparented widget with no live window: MapToGlobal degenerates to the
	// identity, so the global point must equal the (finite) local one.
	gx, gy := e.CaretGlobalXY()
	if math.IsNaN(gx) || math.IsInf(gx, 0) || math.IsNaN(gy) || math.IsInf(gy, 0) {
		t.Fatalf("CaretGlobalXY = (%v, %v), want finite values", gx, gy)
	}
	lx, ly := e.caretLocalXY()
	if gx != lx || gy != ly {
		t.Errorf("CaretGlobalXY = (%v, %v), want local (%v, %v) without a parent chain", gx, gy, lx, ly)
	}
}
