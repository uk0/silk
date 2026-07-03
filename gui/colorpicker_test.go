package gui

import (
	"github.com/uk0/silk/paint"
	"testing"
)

// ---------------------------------------------------------------------------
// Pure-helper tests (palette model — no widget/window required)
// ---------------------------------------------------------------------------

func TestColorPickerDefaultPaletteShape(t *testing.T) {
	p := defaultColorPalette()
	if len(p) < 12 || len(p) > 16 {
		t.Fatalf("defaultColorPalette() = %d colors, want 12-16", len(p))
	}
	black := paint.Color{0, 0, 0, 255}
	white := paint.Color{255, 255, 255, 255}
	if paletteIndexOf(p, black) < 0 {
		t.Error("default palette missing black")
	}
	if paletteIndexOf(p, white) < 0 {
		t.Error("default palette missing white")
	}
	// Palette callers must not see two functions returning the same backing
	// array — each call should be independently mutable.
	q := defaultColorPalette()
	q[0] = paint.Color{1, 2, 3, 4}
	if p[0] == q[0] {
		t.Error("defaultColorPalette returns shared backing array")
	}
}

func TestColorPickerPaletteIndexOf(t *testing.T) {
	p := []paint.Color{
		{1, 0, 0, 255},
		{0, 1, 0, 255},
		{0, 0, 1, 255},
	}
	if got := paletteIndexOf(p, paint.Color{0, 1, 0, 255}); got != 1 {
		t.Errorf("paletteIndexOf(green) = %d, want 1", got)
	}
	if got := paletteIndexOf(p, paint.Color{9, 9, 9, 9}); got != -1 {
		t.Errorf("paletteIndexOf(missing) = %d, want -1", got)
	}
	if got := paletteIndexOf(nil, paint.Color{0, 0, 0, 255}); got != -1 {
		t.Errorf("paletteIndexOf(nil palette) = %d, want -1", got)
	}
}

func TestColorPickerPaletteAtIndex(t *testing.T) {
	p := []paint.Color{
		{1, 2, 3, 255},
		{4, 5, 6, 255},
	}
	if got := paletteAtIndex(p, 1); got != (paint.Color{4, 5, 6, 255}) {
		t.Errorf("paletteAtIndex(1) = %v", got)
	}
	if got := paletteAtIndex(p, -1); got != (paint.Color{}) {
		t.Errorf("paletteAtIndex(-1) = %v, want zero color", got)
	}
	if got := paletteAtIndex(p, 9); got != (paint.Color{}) {
		t.Errorf("paletteAtIndex(out-of-range) = %v, want zero color", got)
	}
}

func TestColorPickerPaletteHitTestIndex(t *testing.T) {
	// strip at x=30, y=4, cell=20, n=4 -> covers x∈[30,110], y∈[4,24]
	x0, y0, cell := 30.0, 4.0, 20.0
	n := 4

	cases := []struct {
		name    string
		x, y    float64
		wantIdx int
	}{
		{"first cell center", 35, 10, 0},
		{"second cell center", 55, 10, 1},
		{"last cell center", 95, 10, 3},
		{"left of strip", 20, 10, -1},
		{"right past last cell", 115, 10, -1},
		{"above strip", 50, 0, -1},
		{"below strip", 50, 30, -1},
		{"exact right edge of last cell", 109.999, 10, 3},
		{"start of first cell", 30, 4, 0},
	}
	for _, c := range cases {
		if got := paletteHitTestIndex(c.x, c.y, x0, y0, cell, n); got != c.wantIdx {
			t.Errorf("%s: hit(%v,%v) = %d, want %d", c.name, c.x, c.y, got, c.wantIdx)
		}
	}

	// Degenerate inputs are safe.
	if got := paletteHitTestIndex(10, 10, 0, 0, 0, 4); got != -1 {
		t.Errorf("zero cell: got %d, want -1", got)
	}
	if got := paletteHitTestIndex(10, 10, 0, 0, 20, 0); got != -1 {
		t.Errorf("zero n: got %d, want -1", got)
	}
}

// ---------------------------------------------------------------------------
// Public API tests
// ---------------------------------------------------------------------------

func TestColorPickerDefaultColor(t *testing.T) {
	cp := NewColorPicker()
	c := cp.Color()
	if c.A == 0 {
		t.Errorf("default color is fully transparent: %v", c)
	}
	if c == (paint.Color{}) {
		t.Errorf("default color is zero value")
	}
}

func TestColorPickerSetColorFiresOnChangeOnly(t *testing.T) {
	cp := NewColorPicker()
	calls := 0
	var last paint.Color
	cp.SigColorChanged(func(c paint.Color) {
		calls++
		last = c
	})
	red := paint.Color{255, 0, 0, 255}
	cp.SetColor(red)
	if cp.Color() != red {
		t.Errorf("Color() = %v, want %v", cp.Color(), red)
	}
	if calls != 1 || last != red {
		t.Errorf("callback: calls=%d last=%v want 1 red", calls, last)
	}
	// No-op set must NOT fire the callback again.
	cp.SetColor(red)
	if calls != 1 {
		t.Errorf("no-op SetColor fired callback: calls=%d", calls)
	}
}

func TestColorPickerSetPaletteOverride(t *testing.T) {
	cp := NewColorPicker()
	custom := []paint.Color{
		{10, 20, 30, 255},
		{40, 50, 60, 255},
	}
	cp.SetPalette(custom)
	if got := cp.Palette(); len(got) != 2 {
		t.Fatalf("Palette() len = %d, want 2", len(got))
	}
	// SetPalette must deep-copy so caller mutation does not leak in.
	custom[0] = paint.Color{99, 99, 99, 255}
	if cp.Palette()[0] != (paint.Color{10, 20, 30, 255}) {
		t.Errorf("SetPalette did not copy; mutated caller slice leaked into widget")
	}
	// nil restores defaults.
	cp.SetPalette(nil)
	if len(cp.Palette()) < 12 {
		t.Errorf("SetPalette(nil) did not restore defaults, len=%d", len(cp.Palette()))
	}
}

func TestColorPickerKeyboardNav(t *testing.T) {
	cp := NewColorPicker()
	cp.SetPalette([]paint.Color{
		{10, 0, 0, 255},
		{20, 0, 0, 255},
		{30, 0, 0, 255},
		{40, 0, 0, 255},
	})
	cp.activeIdx = 0
	calls := 0
	cp.SigColorChanged(func(c paint.Color) { calls++ })

	cp.OnKeyDown(KeyRight, false)
	if cp.activeIdx != 1 {
		t.Errorf("Right: activeIdx=%d, want 1", cp.activeIdx)
	}
	if calls != 0 {
		t.Errorf("Right fired color callback before commit: calls=%d", calls)
	}
	cp.OnKeyDown(KeyRight, false)
	cp.OnKeyDown(KeyDown, false) // Down acts like Right in single-row layout
	if cp.activeIdx != 3 {
		t.Errorf("after Right,Right,Down: activeIdx=%d, want 3", cp.activeIdx)
	}
	// Clamps at the end.
	cp.OnKeyDown(KeyRight, false)
	if cp.activeIdx != 3 {
		t.Errorf("Right past end: activeIdx=%d, want 3 (clamped)", cp.activeIdx)
	}
	// Home/End jumps.
	cp.OnKeyDown(KeyHome, false)
	if cp.activeIdx != 0 {
		t.Errorf("Home: activeIdx=%d, want 0", cp.activeIdx)
	}
	cp.OnKeyDown(KeyLeft, false)
	if cp.activeIdx != 0 {
		t.Errorf("Left past start: activeIdx=%d, want 0 (clamped)", cp.activeIdx)
	}
	cp.OnKeyDown(KeyEnd, false)
	if cp.activeIdx != 3 {
		t.Errorf("End: activeIdx=%d, want 3", cp.activeIdx)
	}
	// Enter commits the focused swatch and fires the change callback.
	cp.OnKeyDown(KeyEnter, false)
	if cp.Color() != (paint.Color{40, 0, 0, 255}) {
		t.Errorf("Enter commit: Color()=%v want {40,0,0,255}", cp.Color())
	}
	if calls != 1 {
		t.Errorf("Enter: callback calls=%d, want 1", calls)
	}
	// Space on the same swatch is a no-op for the callback.
	cp.OnKeyDown(KeySpace, false)
	if calls != 1 {
		t.Errorf("Space on already-selected: callback calls=%d, want 1", calls)
	}
	// Space on a different swatch fires once.
	cp.OnKeyDown(KeyHome, false)
	cp.OnKeyDown(KeySpace, false)
	if calls != 2 {
		t.Errorf("Space on new swatch: callback calls=%d, want 2", calls)
	}
	if cp.Color() != (paint.Color{10, 0, 0, 255}) {
		t.Errorf("Space commit: Color()=%v want {10,0,0,255}", cp.Color())
	}
}

func TestColorPickerLeftDownPicksSwatch(t *testing.T) {
	cp := NewColorPicker()
	cp.SetPalette([]paint.Color{
		{11, 0, 0, 255},
		{22, 0, 0, 255},
		{33, 0, 0, 255},
	})
	cp.SetSize(400, 30) // give it room so the inline strip is laid out

	calls := 0
	var got paint.Color
	cp.SigColorChanged(func(c paint.Color) {
		calls++
		got = c
	})

	x0, y0, cell := cp.paletteOrigin()

	// Click on the second swatch.
	cp.OnLeftDown(x0+1.5*cell, y0+0.5*cell)
	if calls != 1 {
		t.Fatalf("click on swatch[1]: callback calls=%d, want 1", calls)
	}
	if got != (paint.Color{22, 0, 0, 255}) {
		t.Errorf("click on swatch[1]: color=%v want {22,0,0,255}", got)
	}
	if cp.activeIdx != 1 {
		t.Errorf("click on swatch[1]: activeIdx=%d, want 1", cp.activeIdx)
	}

	// Click on the main swatch (left of palette) — should NOT change color.
	cp.OnLeftDown(2, 10)
	if calls != 1 {
		t.Errorf("click on main swatch fired color callback: calls=%d want 1", calls)
	}
}
