package ged

import (
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
)

// itemsOf lifts FakeWidgets into the graph.IItem slice alignFrame takes.
func itemsOf(widgets ...*FakeWidget) []graph.IItem {
	items := make([]graph.IItem, len(widgets))
	for i, w := range widgets {
		items[i] = w
	}
	return items
}

// overlapsOn reports whether a and b overlap along one axis (horizontal when
// horizontal is true). Touching edges are not an overlap — a clamped
// distribute is allowed to pack rects edge to edge.
func overlapsOn(a, b geom.Rect, horizontal bool) bool {
	if horizontal {
		return a.Left() < b.Right() && b.Left() < a.Right()
	}
	return a.Top() < b.Bottom() && b.Top() < a.Bottom()
}

// TestDistributeHOverfullNeverOverlaps is the minimum-gap guard. The three
// rects are wider (150 total) than the span between the outer two (110), so an
// even gap would be negative:
//
//	A: X 0   W 50   B: X 10  W 50   C: X 60  W 50
//	span = 110, occupied = 150, gap = (110-150)/2 = -20
//
// A negative gap slides B backwards onto C. Clamped at zero, the rects pack
// edge to edge from A and nothing overlaps.
func TestDistributeHOverfullNeverOverlaps(t *testing.T) {
	in := []geom.Rect{
		{X: 0, Y: 0, Width: 50, Height: 5},
		{X: 10, Y: 0, Width: 50, Height: 5},
		{X: 60, Y: 0, Width: 50, Height: 5},
	}
	got, _ := alignRects(in, DistributeH)
	for i := range got {
		for j := i + 1; j < len(got); j++ {
			if overlapsOn(got[i], got[j], true) {
				t.Errorf("DistributeH on an over-full span overlapped rects %d %v and %d %v (all: %v)",
					i, got[i], j, got[j], got)
			}
		}
	}
}

// TestDistributeVOverfullNeverOverlaps is the vertical twin of the above.
func TestDistributeVOverfullNeverOverlaps(t *testing.T) {
	in := []geom.Rect{
		{X: 0, Y: 0, Width: 5, Height: 50},
		{X: 0, Y: 10, Width: 5, Height: 50},
		{X: 0, Y: 60, Width: 5, Height: 50},
	}
	got, _ := alignRects(in, DistributeV)
	for i := range got {
		for j := i + 1; j < len(got); j++ {
			if overlapsOn(got[i], got[j], false) {
				t.Errorf("DistributeV on an over-full span overlapped rects %d %v and %d %v (all: %v)",
					i, got[i], j, got[j], got)
			}
		}
	}
}

// TestDistributeAxisReportsClamp checks the flag the status-bar message rides
// on: true only when the even gap would have been negative.
func TestDistributeAxisReportsClamp(t *testing.T) {
	roomy := []geom.Rect{
		{X: 0, Y: 0, Width: 10, Height: 5},
		{X: 30, Y: 0, Width: 10, Height: 5},
		{X: 100, Y: 0, Width: 10, Height: 5},
	}
	if clamped := distributeAxis(roomy, true); clamped {
		t.Errorf("distributeAxis reported a clamp on a span with room to spare: %v", roomy)
	}

	tight := []geom.Rect{
		{X: 0, Y: 0, Width: 50, Height: 5},
		{X: 10, Y: 0, Width: 50, Height: 5},
		{X: 60, Y: 0, Width: 50, Height: 5},
	}
	if clamped := distributeAxis(tight, true); !clamped {
		t.Errorf("distributeAxis did not report the clamp on an over-full span: %v", tight)
	}
	// Packed edge to edge from the first rect: 0..50, 50..100, 100..150.
	want := []geom.Rect{
		{X: 0, Y: 0, Width: 50, Height: 5},
		{X: 50, Y: 0, Width: 50, Height: 5},
		{X: 100, Y: 0, Width: 50, Height: 5},
	}
	if !rectsEqual(tight, want) {
		t.Errorf("clamped distribute:\n got  %v\n want %v", tight, want)
	}
}

// TestFrameAlignDelta covers the frame-relative translation for every mode.
// box is a 10x4 rect at (5,5); frame is a 100x50 rect at the origin (a form),
// so the hand-computed deltas are:
//
//	left   -5      right  100-15 = 85
//	top    -5      bottom 50-9   = 41
//	hcentre 50-10  = 40          vcentre 25-7 = 18
func TestFrameAlignDelta(t *testing.T) {
	box := geom.Rect{X: 5, Y: 5, Width: 10, Height: 4}
	frame := geom.Rect{X: 0, Y: 0, Width: 100, Height: 50}

	cases := []struct {
		name   string
		mode   AlignMode
		dx, dy float64
	}{
		{"left", AlignLeft, -5, 0},
		{"right", AlignRight, 85, 0},
		{"hcenter", AlignHCenter, 40, 0},
		{"top", AlignTop, 0, -5},
		{"bottom", AlignBottom, 0, 41},
		{"vcenter", AlignVCenter, 0, 18},
		{"center", AlignCenter, 40, 18},
		{"distributeH", DistributeH, 0, 0},
		{"distributeV", DistributeV, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dx, dy := frameAlignDelta(box, frame, tc.mode)
			if dx != tc.dx || dy != tc.dy {
				t.Errorf("frameAlignDelta(%s) = (%g,%g), want (%g,%g)", tc.name, dx, dy, tc.dx, tc.dy)
			}
		})
	}
}

// TestFrameAlignDeltaOffsetFrame proves the frame's own origin is respected:
// a container's rect does not start at (0,0) the way the form's does.
func TestFrameAlignDeltaOffsetFrame(t *testing.T) {
	box := geom.Rect{X: 60, Y: 30, Width: 10, Height: 5}
	frame := geom.Rect{X: 50, Y: 20, Width: 80, Height: 60}

	if dx, _ := frameAlignDelta(box, frame, AlignLeft); dx != -10 {
		t.Errorf("AlignLeft dx = %g, want -10 (box.Left 60 → frame.Left 50)", dx)
	}
	if dx, _ := frameAlignDelta(box, frame, AlignRight); dx != 60 {
		t.Errorf("AlignRight dx = %g, want 60 (box.Right 70 → frame.Right 130)", dx)
	}
	if _, dy := frameAlignDelta(box, frame, AlignBottom); dy != 45 {
		t.Errorf("AlignBottom dy = %g, want 45 (box.Bottom 35 → frame.Bottom 80)", dy)
	}
}

// TestAlignFrameFormRoot: a widget parented to the scene measures against the
// form's content rect, which starts at the origin of the children's coordinate
// space no matter where the form itself sits.
func TestAlignFrameFormRoot(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)
	a := addFakeAt(t, scene, "a", 10, 10, 20, 5)

	frame, ok := alignFrame(itemsOf(a))
	if !ok {
		t.Fatalf("alignFrame on a form-parented widget reported no frame")
	}
	want := geom.Rect{X: 0, Y: 0, Width: 200, Height: 150}
	if frame != want {
		t.Errorf("alignFrame = %v, want %v", frame, want)
	}
}

// TestAlignFrameContainer: widgets sharing a container parent measure against
// the container's own rect, not the form's.
func TestAlignFrameContainer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(50, 20, 80, 60)
	vbox.SetParent(scene)

	a := addFakeAt(t, scene, "a", 60, 30, 10, 5)
	a.SetParent(vbox)
	b := addFakeAt(t, scene, "b", 90, 40, 10, 5)
	b.SetParent(vbox)

	frame, ok := alignFrame(itemsOf(a, b))
	if !ok {
		t.Fatalf("alignFrame on two widgets in one container reported no frame")
	}
	want := geom.Rect{X: 50, Y: 20, Width: 80, Height: 60}
	if frame != want {
		t.Errorf("alignFrame = %v, want %v (the container's rect)", frame, want)
	}
}

// TestAlignFrameMixedParents: a selection spanning parents has no single
// frame. Reporting one would align half the selection against the wrong box.
func TestAlignFrameMixedParents(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(50, 20, 80, 60)
	vbox.SetParent(scene)

	inside := addFakeAt(t, scene, "inside", 60, 30, 10, 5)
	inside.SetParent(vbox)
	outside := addFakeAt(t, scene, "outside", 10, 10, 10, 5)

	if frame, ok := alignFrame(itemsOf(inside, outside)); ok {
		t.Errorf("alignFrame across two parents returned %v, want no frame", frame)
	}
}

// TestAlignSelectionSingleAlignsToForm is decision 1: one selected widget has
// nothing to align against, so it aligns against the form itself.
func TestAlignSelectionSingleAlignsToForm(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	a := addFakeAt(t, scene, "a", 25, 35, 10, 5)
	view.Selection().Clear()
	view.Selection().Add(a)

	view.AlignSelection(AlignRight)

	if x, y := a.Pos(); x != 190 || y != 35 {
		t.Errorf("single-widget AlignRight put it at (%g,%g), want (190,35) — the form's right edge", x, y)
	}

	// One undoable step, and it restores the original position.
	view.Scene().UndoStack().Undo()
	if x, y := a.Pos(); x != 25 || y != 35 {
		t.Errorf("position after undo = (%g,%g), want (25,35)", x, y)
	}
}

// TestAlignSelectionSingleInContainerAlignsToContainer is decision 2: the same
// lone widget nested in a container measures against the container's rect.
func TestAlignSelectionSingleInContainerAlignsToContainer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(50, 20, 80, 60)
	vbox.SetParent(scene)

	a := addFakeAt(t, scene, "a", 60, 30, 10, 5)
	a.SetParent(vbox)

	view.Selection().Clear()
	view.Selection().Add(a)

	view.AlignSelection(AlignRight)

	// 50 + 80 - 10 = 120, not the form's 200 - 10 = 190.
	if x, y := a.Pos(); x != 120 || y != 30 {
		t.Errorf("nested AlignRight put the widget at (%g,%g), want (120,30) — the container's right edge", x, y)
	}
}

// TestAlignSelectionCenterKeepsGroupLayout: 居中 moves the whole selection by
// one shared delta so the group lands centred in the form with its internal
// spacing intact.
func TestAlignSelectionCenterKeepsGroupLayout(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 100)

	a := addFakeAt(t, scene, "a", 0, 0, 20, 10)
	b := addFakeAt(t, scene, "b", 40, 20, 20, 10)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(b)

	view.AlignSelection(AlignCenter)

	// Bounding box 0,0..60,30 → centre (30,15); form centre (100,50);
	// shared delta (70,35).
	if x, y := a.Pos(); x != 70 || y != 35 {
		t.Errorf("a after 居中 = (%g,%g), want (70,35)", x, y)
	}
	if x, y := b.Pos(); x != 110 || y != 55 {
		t.Errorf("b after 居中 = (%g,%g), want (110,55)", x, y)
	}
}

// TestAlignSelectionCenterMixedParentsNoOp: with the selection spanning
// parents there is no single frame, so 居中 refuses rather than silently
// centring half of it in the wrong box.
func TestAlignSelectionCenterMixedParentsNoOp(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(50, 20, 80, 60)
	vbox.SetParent(scene)

	inside := addFakeAt(t, scene, "inside", 60, 30, 10, 5)
	inside.SetParent(vbox)
	outside := addFakeAt(t, scene, "outside", 10, 10, 10, 5)

	view.Selection().Clear()
	view.Selection().Add(inside)
	view.Selection().Add(outside)

	view.AlignSelection(AlignCenter)

	if x, y := inside.Pos(); x != 60 || y != 30 {
		t.Errorf("nested widget moved to (%g,%g) on a mixed-parent 居中, want (60,30)", x, y)
	}
	if x, y := outside.Pos(); x != 10 || y != 10 {
		t.Errorf("root widget moved to (%g,%g) on a mixed-parent 居中, want (10,10)", x, y)
	}
}

// TestAlignSelectionDistributeClampViaView runs the clamp through the canvas
// glue: three widgets whose combined width (150) exceeds the span between the
// outer two (110) must come out packed, not stacked.
func TestAlignSelectionDistributeClampViaView(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	a := addFakeAt(t, scene, "a", 0, 0, 50, 5)
	b := addFakeAt(t, scene, "b", 10, 0, 50, 5)
	c := addFakeAt(t, scene, "c", 60, 0, 50, 5)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(b)
	view.Selection().Add(c)

	view.AlignSelection(DistributeH)

	want := []float64{0, 50, 100}
	for i, w := range []*FakeWidget{a, b, c} {
		if x, _ := w.Pos(); x != want[i] {
			t.Errorf("%s.X after a clamped DistributeH = %g, want %g", w.WidgetName(), x, want[i])
		}
	}
}

// TestAlignSelectionMultiKeepsSelectionReference: with two or more widgets the
// reference stays the selection's own extremes — the form's edge must not
// hijack a plain 左对齐.
func TestAlignSelectionMultiKeepsSelectionReference(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	a := addFakeAt(t, scene, "a", 40, 10, 10, 5)
	b := addFakeAt(t, scene, "b", 70, 30, 10, 5)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(b)

	view.AlignSelection(AlignLeft)

	if x, _ := a.Pos(); x != 40 {
		t.Errorf("a.X after a two-widget AlignLeft = %g, want 40 (the leftmost selected widget)", x)
	}
	if x, _ := b.Pos(); x != 40 {
		t.Errorf("b.X after a two-widget AlignLeft = %g, want 40", x)
	}
}
