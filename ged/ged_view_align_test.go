package ged

import (
	"silk/geom"
	"testing"
)

// rectsEqual compares two rect slices with a small tolerance so float
// division in the centre/distribute paths doesn't trip exact ==.
func rectsEqual(a, b []geom.Rect) bool {
	if len(a) != len(b) {
		return false
	}
	const eps = 1e-9
	abs := func(f float64) float64 {
		if f < 0 {
			return -f
		}
		return f
	}
	for i := range a {
		if abs(a[i].X-b[i].X) > eps || abs(a[i].Y-b[i].Y) > eps ||
			abs(a[i].Width-b[i].Width) > eps || abs(a[i].Height-b[i].Height) > eps {
			return false
		}
	}
	return true
}

// alignBase is the three-rect fixture shared by the align cases. The hand-
// computed expectations in TestAlignRects are derived from these positions:
//
//	R0: left 0   right 10  cx 5    top 0   bottom 4   cy 2
//	R1: left 20  right 50  cx 35   top 10  bottom 18  cy 14
//	R2: left 100 right 120 cx 110  top 50  bottom 56  cy 53
func alignBase() []geom.Rect {
	return []geom.Rect{
		{X: 0, Y: 0, Width: 10, Height: 4},
		{X: 20, Y: 10, Width: 30, Height: 8},
		{X: 100, Y: 50, Width: 20, Height: 6},
	}
}

// TestAlignRects checks the six pure align modes against hand-computed
// coordinates. Only X/Y may change; widths/heights must pass through.
func TestAlignRects(t *testing.T) {
	cases := []struct {
		name string
		mode alignMode
		want []geom.Rect
	}{
		{
			// min left = 0 → every X = 0
			"left", AlignLeft,
			[]geom.Rect{
				{X: 0, Y: 0, Width: 10, Height: 4},
				{X: 0, Y: 10, Width: 30, Height: 8},
				{X: 0, Y: 50, Width: 20, Height: 6},
			},
		},
		{
			// max right = 120 → X = 120 - width
			"right", AlignRight,
			[]geom.Rect{
				{X: 110, Y: 0, Width: 10, Height: 4},
				{X: 90, Y: 10, Width: 30, Height: 8},
				{X: 100, Y: 50, Width: 20, Height: 6},
			},
		},
		{
			// mean centre X = (5+35+110)/3 = 50 → X = 50 - width/2
			"hcenter", AlignHCenter,
			[]geom.Rect{
				{X: 45, Y: 0, Width: 10, Height: 4},
				{X: 35, Y: 10, Width: 30, Height: 8},
				{X: 40, Y: 50, Width: 20, Height: 6},
			},
		},
		{
			// min top = 0 → every Y = 0
			"top", AlignTop,
			[]geom.Rect{
				{X: 0, Y: 0, Width: 10, Height: 4},
				{X: 20, Y: 0, Width: 30, Height: 8},
				{X: 100, Y: 0, Width: 20, Height: 6},
			},
		},
		{
			// max bottom = 56 → Y = 56 - height
			"bottom", AlignBottom,
			[]geom.Rect{
				{X: 0, Y: 52, Width: 10, Height: 4},
				{X: 20, Y: 48, Width: 30, Height: 8},
				{X: 100, Y: 50, Width: 20, Height: 6},
			},
		},
		{
			// mean centre Y = (2+14+53)/3 = 23 → Y = 23 - height/2
			"vcenter", AlignVCenter,
			[]geom.Rect{
				{X: 0, Y: 21, Width: 10, Height: 4},
				{X: 20, Y: 19, Width: 30, Height: 8},
				{X: 100, Y: 20, Width: 20, Height: 6},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := alignRects(alignBase(), tc.mode)
			if !rectsEqual(got, tc.want) {
				t.Errorf("alignRects(%s):\n got  %v\n want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestAlignRectsDoesNotMutateInput proves alignRects copies: the caller's
// slice must be untouched after a call.
func TestAlignRectsDoesNotMutateInput(t *testing.T) {
	in := alignBase()
	orig := alignBase()
	_ = alignRects(in, AlignRight)
	if !rectsEqual(in, orig) {
		t.Errorf("alignRects mutated its input:\n got  %v\n want %v", in, orig)
	}
}

// TestDistributeH evens out horizontal gaps. Fixture (already X-sorted):
//
//	A: X 0   W 10   B: X 30  W 10   C: X 100  W 10
//
// span = 110 - 0 = 110, occupied = 30, gap = (110-30)/2 = 40.
// Outer rects stay; B slides to 0+10+40 = 50.
func TestDistributeH(t *testing.T) {
	in := []geom.Rect{
		{X: 0, Y: 5, Width: 10, Height: 4},
		{X: 30, Y: 7, Width: 10, Height: 4},
		{X: 100, Y: 9, Width: 10, Height: 4},
	}
	want := []geom.Rect{
		{X: 0, Y: 5, Width: 10, Height: 4},
		{X: 50, Y: 7, Width: 10, Height: 4},
		{X: 100, Y: 9, Width: 10, Height: 4},
	}
	got := alignRects(in, DistributeH)
	if !rectsEqual(got, want) {
		t.Errorf("DistributeH:\n got  %v\n want %v", got, want)
	}
}

// TestDistributeHUnsortedInput passes the same three rects out of X-order and
// asserts each comes back in its ORIGINAL slot with the correct new X. This
// locks in the sort-then-map-back contract alignSelection relies on.
func TestDistributeHUnsortedInput(t *testing.T) {
	// Order: C, A, B (indices stay attached to their rects).
	in := []geom.Rect{
		{X: 100, Y: 9, Width: 10, Height: 4}, // C (rightmost)
		{X: 0, Y: 5, Width: 10, Height: 4},   // A (leftmost)
		{X: 30, Y: 7, Width: 10, Height: 4},  // B (interior, moves to 50)
	}
	want := []geom.Rect{
		{X: 100, Y: 9, Width: 10, Height: 4},
		{X: 0, Y: 5, Width: 10, Height: 4},
		{X: 50, Y: 7, Width: 10, Height: 4},
	}
	got := alignRects(in, DistributeH)
	if !rectsEqual(got, want) {
		t.Errorf("DistributeH (unsorted):\n got  %v\n want %v", got, want)
	}
}

// TestDistributeV evens out vertical gaps. Fixture (already Y-sorted):
//
//	A: Y 0   H 6   B: Y 30  H 6   C: Y 100  H 6
//
// span = 106 - 0 = 106, occupied = 18, gap = (106-18)/2 = 44.
// Outer rects stay; B slides to 0+6+44 = 50.
func TestDistributeV(t *testing.T) {
	in := []geom.Rect{
		{X: 1, Y: 0, Width: 4, Height: 6},
		{X: 2, Y: 30, Width: 4, Height: 6},
		{X: 3, Y: 100, Width: 4, Height: 6},
	}
	want := []geom.Rect{
		{X: 1, Y: 0, Width: 4, Height: 6},
		{X: 2, Y: 50, Width: 4, Height: 6},
		{X: 3, Y: 100, Width: 4, Height: 6},
	}
	got := alignRects(in, DistributeV)
	if !rectsEqual(got, want) {
		t.Errorf("DistributeV:\n got  %v\n want %v", got, want)
	}
}

// TestAlignRectsLessThanTwoNoOp: zero or one rect is returned unchanged for
// every mode (nothing to align against).
func TestAlignRectsLessThanTwoNoOp(t *testing.T) {
	for _, n := range []int{0, 1} {
		in := make([]geom.Rect, n)
		if n == 1 {
			in[0] = geom.Rect{X: 7, Y: 9, Width: 3, Height: 2}
		}
		for mode := AlignLeft; mode <= DistributeV; mode++ {
			orig := make([]geom.Rect, n)
			copy(orig, in)
			got := alignRects(in, mode)
			if !rectsEqual(got, orig) {
				t.Errorf("alignRects(len=%d, mode=%d) changed a no-op input: got %v, want %v",
					n, mode, got, orig)
			}
		}
	}
}

// TestDistributeLessThanThreeNoOp: with exactly two rects there is no interior
// rect to space, so both distribute modes leave the input untouched even
// though align modes (len>=2) would act.
func TestDistributeLessThanThreeNoOp(t *testing.T) {
	in := []geom.Rect{
		{X: 0, Y: 0, Width: 10, Height: 5},
		{X: 80, Y: 40, Width: 10, Height: 5},
	}
	for _, mode := range []alignMode{DistributeH, DistributeV} {
		orig := make([]geom.Rect, len(in))
		copy(orig, in)
		got := alignRects(in, mode)
		if !rectsEqual(got, orig) {
			t.Errorf("distribute (mode=%d) with 2 rects changed input: got %v, want %v",
				mode, got, orig)
		}
	}
}

// addFakeAt drops a named FakeWidget at a given position/size and returns it.
// Mirrors the addFake helper in ged_view_zorder_test.go but with explicit
// bounds so position assertions are meaningful.
func addFakeAt(t *testing.T, scene *GedScene, name string, x, y, w, h float64) *FakeWidget {
	t.Helper()
	fw, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	fw.SetWidgetName(name)
	fw.SetBounds(x, y, w, h)
	fw.SetParent(scene)
	return fw
}

// TestAlignSelectionLeftViaView exercises the context-menu glue: select three
// widgets at different X, run alignSelection(AlignLeft), and confirm every
// widget's X collapses to the min-left (0) while Y and size are preserved.
func TestAlignSelectionLeftViaView(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 0, 0, 10, 4)
	b := addFakeAt(t, scene, "b", 20, 10, 30, 8)
	c := addFakeAt(t, scene, "c", 100, 50, 20, 6)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(b)
	view.Selection().Add(c)

	view.alignSelection(AlignLeft)

	for _, w := range []*FakeWidget{a, b, c} {
		if x, _ := w.Pos(); x != 0 {
			t.Errorf("%s.X after AlignLeft = %g, want 0", w.WidgetName(), x)
		}
	}
	// Y must be untouched.
	if _, y := b.Pos(); y != 10 {
		t.Errorf("b.Y after AlignLeft = %g, want 10 (unchanged)", y)
	}
	// Size must be untouched.
	if bw, bh := b.Size(); bw != 30 || bh != 8 {
		t.Errorf("b size after AlignLeft = (%g,%g), want (30,8)", bw, bh)
	}
}

// TestAlignSelectionDistributeHViaView selects three widgets and distributes
// them horizontally; the interior widget must land at the equalised position
// while the outer two stay fixed.
func TestAlignSelectionDistributeHViaView(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 0, 5, 10, 4)
	b := addFakeAt(t, scene, "b", 30, 7, 10, 4)
	c := addFakeAt(t, scene, "c", 100, 9, 10, 4)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(b)
	view.Selection().Add(c)

	view.alignSelection(DistributeH)

	if x, _ := a.Pos(); x != 0 {
		t.Errorf("a.X after DistributeH = %g, want 0", x)
	}
	if x, _ := b.Pos(); x != 50 {
		t.Errorf("b.X after DistributeH = %g, want 50", x)
	}
	if x, _ := c.Pos(); x != 100 {
		t.Errorf("c.X after DistributeH = %g, want 100", x)
	}
}

// TestAlignSelectionSingleNoOp: a one-item selection must not move — align
// has nothing to align against, so alignSelection short-circuits.
func TestAlignSelectionSingleNoOp(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	a := addFakeAt(t, scene, "a", 25, 35, 10, 5)
	view.Selection().Clear()
	view.Selection().Add(a)

	view.alignSelection(AlignRight)

	if x, y := a.Pos(); x != 25 || y != 35 {
		t.Errorf("single-item AlignRight moved widget to (%g,%g), want (25,35)", x, y)
	}
}
