package ged

import "testing"

// TestAlignSelectionSkipsLockedWidget is the whole point of 位置锁定: the
// widget the user pinned must sit still whichever move reaches it. Drag and
// arrow keys already left it alone (Selection.GenerateMoveCommand skips it),
// but the multi-item align path built its own MoveCommand and moved it anyway,
// so 锁定位置 meant "the mouse will not move this" and nothing more.
func TestAlignSelectionSkipsLockedWidget(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(300, 200)

	a := addFakeAt(t, scene, "a", 0, 10, 10, 5)
	locked := addFakeAt(t, scene, "locked", 20, 30, 10, 5)
	c := addFakeAt(t, scene, "c", 100, 50, 10, 5)
	locked.SetLockPos(true)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(locked)
	view.Selection().Add(c)

	view.AlignSelection(AlignLeft)

	if x, y := locked.Pos(); x != 20 || y != 30 {
		t.Errorf("位置锁定 widget moved to (%g,%g) on 左对齐, want (20,30)", x, y)
	}
	// The unlocked ones still align — the skip drops one widget, not the command.
	if x, _ := a.Pos(); x != 0 {
		t.Errorf("a.X = %g, want 0", x)
	}
	if x, _ := c.Pos(); x != 0 {
		t.Errorf("c.X = %g after 左对齐, want 0", x)
	}
}

// TestAlignSelectionLockedWidgetStillSetsTheLine is the other half of the
// decision: the locked widget is skipped from the MOVE but stays in the
// GEOMETRY. Its edge is a real edge of the selection, so pinning the leftmost
// widget makes it the line the rest align to. Dropping it from alignRects'
// input instead would compute min-left over {20, 100} and sail the whole group
// past the widget the user pinned.
func TestAlignSelectionLockedWidgetStillSetsTheLine(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(300, 200)

	locked := addFakeAt(t, scene, "locked", 0, 10, 10, 5)
	b := addFakeAt(t, scene, "b", 20, 30, 10, 5)
	c := addFakeAt(t, scene, "c", 100, 50, 10, 5)
	locked.SetLockPos(true)

	view.Selection().Clear()
	view.Selection().Add(locked)
	view.Selection().Add(b)
	view.Selection().Add(c)

	view.AlignSelection(AlignLeft)

	if x, y := locked.Pos(); x != 0 || y != 10 {
		t.Errorf("locked anchor moved to (%g,%g), want (0,10)", x, y)
	}
	for _, w := range []*FakeWidget{b, c} {
		if x, _ := w.Pos(); x != 0 {
			t.Errorf("%s.X = %g, want 0 — the locked widget's own left edge", w.WidgetName(), x)
		}
	}
}

// TestAlignSelectionSkipsLockedContainerSubtree is the case the shared
// moveSubtree walk made loud: a locked CONTAINER used to hand its whole
// subtree to AddMoveSubtree, so 对齐 dragged children the user could not have
// dragged himself. Both the container and what is inside it must sit still.
func TestAlignSelectionSkipsLockedContainerSubtree(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(300, 200)

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	box.SetWidgetName("box")
	box.SetBounds(60, 30, 40, 20)
	box.SetParent(scene)
	box.SetLockPos(true)

	inside := addFakeAt(t, scene, "inside", 65, 35, 10, 5)
	inside.SetParent(box)

	a := addFakeAt(t, scene, "a", 10, 10, 10, 5)
	c := addFakeAt(t, scene, "c", 140, 80, 10, 5)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(box)
	view.Selection().Add(c)

	view.AlignSelection(AlignLeft)

	if x, y := box.Pos(); x != 60 || y != 30 {
		t.Errorf("locked container moved to (%g,%g), want (60,30)", x, y)
	}
	if x, y := inside.Pos(); x != 65 || y != 35 {
		t.Errorf("child of a locked container moved to (%g,%g), want (65,35)", x, y)
	}
	if x, _ := c.Pos(); x != 10 {
		t.Errorf("c.X = %g, want 10 — the rest of the selection still aligns", x)
	}
}

// TestAlignSelectionCarriesLockedChildOfMovingContainer guards the other
// direction, and the order of the two skips: a lock pins a widget inside its
// container, it does not pin the container. When the container itself is
// selected and moves, its children ride along whatever their own lock says —
// the contract graph.AddMoveSubtree documents — or a locked child would be
// left behind at the wrong offset inside the box that moved out from under it.
func TestAlignSelectionCarriesLockedChildOfMovingContainer(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(300, 200)

	box, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	box.SetWidgetName("box")
	box.SetBounds(60, 30, 40, 20)
	box.SetParent(scene)

	inside := addFakeAt(t, scene, "inside", 65, 35, 10, 5)
	inside.SetParent(box)
	inside.SetLockPos(true)

	a := addFakeAt(t, scene, "a", 10, 10, 10, 5)

	view.Selection().Clear()
	view.Selection().Add(a)
	view.Selection().Add(box)
	view.Selection().Add(inside)

	view.AlignSelection(AlignLeft)

	// min-left over the movers {10, 60} = 10, so the box travels -50.
	if x, y := box.Pos(); x != 10 || y != 30 {
		t.Fatalf("container moved to (%g,%g), want (10,30)", x, y)
	}
	if x, y := inside.Pos(); x != 15 || y != 35 {
		t.Errorf("locked child at (%g,%g) after its container moved -50, want (15,35) — "+
			"it must keep its offset inside the box", x, y)
	}
}

// TestDistributeRefusesWithLockedInterior: 分布 is a joint constraint, not N
// independent targets. Four widgets, the second pinned: skipping just the
// pinned one and letting the third take its computed slot leaves the gaps at
// 15 / 35 / 20 — a 均分 that evened out nothing, with no hint of why. The whole
// command is refused instead, so nothing moves and nothing lands on the undo
// stack.
//
// Four and not three on purpose: with three widgets the interior one is the
// ONLY mover, so a pinned interior and a refusal look identical and the test
// would prove nothing.
func TestDistributeRefusesWithLockedInterior(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(300, 200)

	// Even gap = (span 100 − occupied 40) / 3 = 20, so the targets are
	// 0 / 30 / 60 / 90: both interior widgets would have to move.
	a := addFakeAt(t, scene, "a", 0, 5, 10, 4)
	locked := addFakeAt(t, scene, "locked", 25, 5, 10, 4)
	c := addFakeAt(t, scene, "c", 70, 5, 10, 4)
	d := addFakeAt(t, scene, "d", 90, 5, 10, 4)
	locked.SetLockPos(true)

	view.Selection().Clear()
	for _, w := range []*FakeWidget{a, locked, c, d} {
		view.Selection().Add(w)
	}

	before := view.Scene().UndoStack().Count()
	view.AlignSelection(DistributeH)

	want := []float64{0, 25, 70, 90}
	for i, w := range []*FakeWidget{a, locked, c, d} {
		if x, _ := w.Pos(); x != want[i] {
			t.Errorf("%s.X = %g after a 水平分布 that cannot even out, want %g (nobody moves)",
				w.WidgetName(), x, want[i])
		}
	}
	if got := view.Scene().UndoStack().Count() - before; got != 0 {
		t.Errorf("refused 水平分布 pushed %d command(s); Ctrl+Z would burn a step on nothing", got)
	}
}

// TestDistributeVRefusesWithLockedInterior is the vertical twin, and it is a
// separate test rather than a table row because the refusal is written as one
// mode test over both 分布 modes: every veto test above drives DistributeH, so
// narrowing that test to DistributeH alone left 垂直分布 spreading the rest of
// the column around a widget that never moved, and the whole suite green.
func TestDistributeVRefusesWithLockedInterior(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 300)

	// Same arithmetic on the other axis: even gap = (span 100 − occupied 40)
	// / 3 = 20, so the targets are 0 / 30 / 60 / 90 and both interior widgets
	// would have to move.
	a := addFakeAt(t, scene, "a", 5, 0, 4, 10)
	locked := addFakeAt(t, scene, "locked", 5, 25, 4, 10)
	c := addFakeAt(t, scene, "c", 5, 70, 4, 10)
	d := addFakeAt(t, scene, "d", 5, 90, 4, 10)
	locked.SetLockPos(true)

	view.Selection().Clear()
	for _, w := range []*FakeWidget{a, locked, c, d} {
		view.Selection().Add(w)
	}

	before := view.Scene().UndoStack().Count()
	view.AlignSelection(DistributeV)

	want := []float64{0, 25, 70, 90}
	for i, w := range []*FakeWidget{a, locked, c, d} {
		if y := yOf(w); y != want[i] {
			t.Errorf("%s.Y = %g after a 垂直分布 that cannot even out, want %g (nobody moves)",
				w.WidgetName(), y, want[i])
		}
	}
	if got := view.Scene().UndoStack().Count() - before; got != 0 {
		t.Errorf("refused 垂直分布 pushed %d command(s); Ctrl+Z would burn a step on nothing", got)
	}
}

// yOf reads the vertical position, so the assertion above reads as one line
// per widget like its horizontal twin does.
func yOf(w *FakeWidget) float64 {
	_, y := w.Pos()
	return y
}

// TestDistributeIgnoresLockOnACarriedChild: the refusal vetoes 分布 over a lock
// that actually blocks something. A widget inside a selected container is not
// that: it rides with the container whatever its own lock says
// (graph.AddMoveSubtree), so the even gaps are still reachable and the command
// must go through.
//
// Asserted against a control run with the child unlocked rather than against
// hand-computed coordinates: the property is "this lock changes nothing", and
// pinning the numbers would also pin how a nested selected child feeds
// alignRects' geometry, which is not this skip's business.
func TestDistributeIgnoresLockOnACarriedChild(t *testing.T) {
	// build returns the four positions 水平分布 leaves behind, with the child
	// inside the container locked or not.
	build := func(lock bool) [4][2]float64 {
		view := NewGedView()
		scene := view.GedScene()
		scene.SetSize(400, 200)

		box, err := NewFakeWidgetFromFactory("gui.VBox")
		if err != nil {
			t.Fatalf("create VBox: %v", err)
		}
		box.SetWidgetName("box")
		box.SetBounds(25, 5, 10, 4)
		box.SetParent(scene)

		inside := addFakeAt(t, scene, "inside", 27, 5, 4, 2)
		inside.SetParent(box)
		inside.SetLockPos(lock)

		a := addFakeAt(t, scene, "a", 0, 5, 10, 4)
		d := addFakeAt(t, scene, "d", 90, 5, 10, 4)

		view.Selection().Clear()
		for _, w := range []*FakeWidget{a, box, inside, d} {
			view.Selection().Add(w)
		}
		view.AlignSelection(DistributeH)

		var out [4][2]float64
		for i, w := range []*FakeWidget{a, box, inside, d} {
			x, y := w.Pos()
			out[i] = [2]float64{x, y}
		}
		return out
	}

	free, locked := build(false), build(true)
	if free != locked {
		t.Errorf("水平分布 with a locked child inside a selected container gave %v, "+
			"want %v — the same as with it unlocked, since it rides with the container either way",
			locked, free)
	}
	// Guard the control itself: if 分布 moved nobody even unlocked, the
	// comparison above would pass on two refusals.
	if free[1][0] == 25 {
		t.Fatalf("control run did not move the container off 25: %v", free)
	}
}

// TestDistributeProceedsWithLockedAnchor: distributeAxis pins the outer two
// rects by construction, so a lock on one of them asks for nothing that is not
// happening anyway. Refusing on a bare IsLockPos would turn a perfectly
// achievable 分布 into a status message.
func TestDistributeProceedsWithLockedAnchor(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(300, 200)

	locked := addFakeAt(t, scene, "locked", 0, 5, 10, 4)
	b := addFakeAt(t, scene, "b", 30, 7, 10, 4)
	c := addFakeAt(t, scene, "c", 100, 9, 10, 4)
	locked.SetLockPos(true)

	view.Selection().Clear()
	view.Selection().Add(locked)
	view.Selection().Add(b)
	view.Selection().Add(c)

	view.AlignSelection(DistributeH)

	if x, _ := locked.Pos(); x != 0 {
		t.Errorf("locked anchor moved to %g, want 0", x)
	}
	if x, _ := b.Pos(); x != 50 {
		t.Errorf("b.X = %g, want 50 — the interior widget still gets evened out", x)
	}
	if x, _ := c.Pos(); x != 100 {
		t.Errorf("c.X = %g, want 100", x)
	}
}
