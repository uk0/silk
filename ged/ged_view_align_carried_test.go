package ged

import "testing"

// newAlignBox drops a named container at the given bounds — the align tests
// need a widget that can hold children, which addFakeAt's Button cannot.
func newAlignBox(t *testing.T, scene *GedScene, name string, x, y, w, h float64) *FakeWidget {
	t.Helper()
	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	box.SetWidgetName(name)
	box.SetBounds(x, y, w, h)
	box.SetParent(scene)
	return box
}

// TestDistributeSpacesOnlyWhatMoves is the headline case: a widget inside a
// selected container took a slot in the 分布 lattice and then never moved into
// it, because it rides with the container instead. Four selected items, three
// movers: the gaps came out measured for four and applied to three, so the
// result was not even and matched nothing the user selected.
//
// Asserted on the gaps rather than on coordinates because the even gap IS what
// 水平分布 promises; with the outer two pinned there is exactly one arrangement
// that satisfies it.
func TestDistributeSpacesOnlyWhatMoves(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(400, 200)

	a := addFakeAt(t, scene, "a", 0, 5, 10, 4)
	box := newAlignBox(t, scene, "box", 25, 5, 10, 4)
	inside := addFakeAt(t, scene, "inside", 27, 5, 4, 2)
	inside.SetParent(box)
	d := addFakeAt(t, scene, "d", 90, 5, 10, 4)

	view.Selection().Clear()
	for _, w := range []*FakeWidget{a, box, inside, d} {
		view.Selection().Add(w)
	}

	view.AlignSelection(DistributeH)

	// Span 100, occupied 30 over the three movers, so the even gap is 35 and
	// the container lands at 45.
	if x, _ := a.Pos(); x != 0 {
		t.Fatalf("a.X = %g, want 0 — the outer two are the anchors of a 水平分布", x)
	}
	if x, _ := d.Pos(); x != 90 {
		t.Fatalf("d.X = %g, want 90 — the outer two are the anchors of a 水平分布", x)
	}

	ax, _ := a.Pos()
	aw, _ := a.Size()
	bx, _ := box.Pos()
	bw, _ := box.Size()
	dx, _ := d.Pos()
	if left, right := bx-(ax+aw), dx-(bx+bw); left != right {
		t.Errorf("水平分布 left the gaps at %g and %g, want them equal — the widget inside "+
			"the container took a slot in the spread and then rode along instead of using it",
			left, right)
	}

	// It does ride along, at the offset it had: 27 - 25 = 2.
	if ix, _ := inside.Pos(); ix-bx != 2 {
		t.Errorf("widget inside the container sits at %g with the container at %g: offset %g, want 2",
			ix, bx, ix-bx)
	}
}

// TestAlignHCenterMeanExcludesCarriedChild is the same disagreement in an align
// mode, and it needs no unusual layout to show up: the mean is a balance point
// over the widgets that swing onto it, so a widget that is only along for the
// ride must not pull it. Its own centre ends up nowhere near the line it helped
// choose.
func TestAlignHCenterMeanExcludesCarriedChild(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(300, 200)

	box := newAlignBox(t, scene, "box", 10, 20, 20, 30)
	inside := addFakeAt(t, scene, "inside", 12, 25, 4, 2)
	inside.SetParent(box)
	other := addFakeAt(t, scene, "other", 100, 60, 20, 10)

	view.Selection().Clear()
	for _, w := range []*FakeWidget{box, inside, other} {
		view.Selection().Add(w)
	}

	view.AlignSelection(AlignHCenter)

	// Mean over the two movers = (20 + 110) / 2 = 65. Counting the child's
	// centre of 14 as a third vote drags the line down to 48.
	const wantCX = 65.0
	bx, _ := box.Pos()
	bw, _ := box.Size()
	if cx := bx + bw/2; cx != wantCX {
		t.Errorf("container centre = %g after 水平居中对齐, want %g — the widget inside it voted "+
			"on the mean and then rode away from it", cx, wantCX)
	}
	ox, _ := other.Pos()
	ow, _ := other.Size()
	if cx := ox + ow/2; cx != wantCX {
		t.Errorf("second widget centre = %g, want %g", cx, wantCX)
	}
	// Still carried, at the offset it had: 12 - 10 = 2.
	if ix, _ := inside.Pos(); ix-bx != 2 {
		t.Errorf("widget inside the container sits at %g with the container at %g: offset %g, want 2",
			ix, bx, ix-bx)
	}
}

// TestAlignLeftIgnoresOverhangingChildEdge is the one align case where the
// child's rect is not simply redundant. A child may hang past its container's
// edge — nothing insets or clips it — and counting that edge made 左对齐 pick a
// line no mover owned, then move the whole group onto it while the child that
// set it slid out to the left again. 左对齐 lines the selection up on an edge
// one of them already has; it does not invent one.
//
// With the child inside the container's bounds, which is the ordinary case, the
// container's own edge is the smaller one and the two answers agree.
func TestAlignLeftIgnoresOverhangingChildEdge(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(300, 200)

	box := newAlignBox(t, scene, "box", 60, 30, 40, 20)
	inside := addFakeAt(t, scene, "inside", 50, 35, 10, 5)
	inside.SetParent(box)
	a := addFakeAt(t, scene, "a", 100, 10, 10, 5)

	view.Selection().Clear()
	for _, w := range []*FakeWidget{box, inside, a} {
		view.Selection().Add(w)
	}

	view.AlignSelection(AlignLeft)

	if x, y := box.Pos(); x != 60 || y != 30 {
		t.Errorf("container moved to (%g,%g) on 左对齐, want (60,30) — it is the leftmost widget "+
			"that can move, so it is the line, not a follower of the child hanging out of it", x, y)
	}
	if x, _ := a.Pos(); x != 60 {
		t.Errorf("a.X = %g after 左对齐, want 60 — the container's own left edge", x)
	}
	if x, _ := inside.Pos(); x != 50 {
		t.Errorf("child of the container moved to %g, want 50 — its container never moved", x)
	}
}
