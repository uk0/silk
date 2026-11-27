package ged

import (
	"errors"
	"testing"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func TestManagedChildLeavesLayoutAtActualReleasePoint(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	box := transferBox(t, s, LayoutVertical, "Source", 12, 15, 55, 50)
	moved, sibling := transferButton(t, box, "Moved"), transferButton(t, box, "Sibling")
	enableFixtureLayout(box)
	a, b := moved.Bounds1(), sibling.Bounds1()
	changes := 0
	s.SigDesignChanged(func() { changes++ })

	transferDrag(v, moved, 122, 90)
	hover := v.managedHint
	if !hover.active || !hover.valid || hover.autoLayout || hover.bounds.IsEmpty() ||
		moved.Parent() != box || moved.Bounds1() != a || changes != 0 || s.UndoStack().CanUndo() {
		t.Fatalf("expected a non-mutating free placement preview, got %+v", hover)
	}
	v.OnLeftUp(v.MapFromScene(187, 124))
	if moved.Parent() != s || moved.layoutCell.Valid || moved.Bounds1() == hover.bounds || !v.Selection().Contains(moved) ||
		changes != 1 || s.UndoStack().Count() != 1 || sibling.Parent() != box {
		t.Fatalf("expected one free placement edit at release: %v; changes=%d", moved.Parent(), changes)
	}
	after, afterSibling := moved.Bounds1(), sibling.Bounds1()
	if after.Width != a.Width || after.Height != a.Height || after.X < 145 || after.Y < 105 {
		t.Fatalf("drop did not keep grabbed geometry near release: %v (was %v)", after, a)
	}
	v.Undo()
	if moved.Parent() != box || moved.IndexInParent() != 0 || moved.Bounds1() != a || sibling.Bounds1() != b ||
		s.UndoStack().CanUndo() || !v.Selection().Contains(moved) {
		t.Fatal("one Undo failed to restore source order, geometry and selection")
	}
	v.Redo()
	if moved.Parent() != s || moved.Bounds1() != after || sibling.Bounds1() != afterSibling {
		t.Fatal("Redo did not reproduce the free drop")
	}

	doc := s.SaveDesign()
	reload := NewGedScene()
	if err := reload.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	for _, n := range []*decl.Node{buildSceneDeclNode(reload), mustDeclFromDesign(t, doc)} {
		runtime, index, err := n.BuildWithIndex()
		if err != nil {
			t.Fatal(err)
		}
		w := index["Moved"].(gui.IWidget)
		x, y := w.Pos()
		if w.Parent() != runtime || x != roundPx(after.X) || y != roundPx(after.Y) {
			t.Fatalf("SDK changed ownership or landing: %T %.2f %.2f, want %v", w.Parent(), x, y, after)
		}
	}
}

func TestManagedGridChildDropsIntoFreeCardContent(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	src := transferBox(t, s, LayoutGrid, "Source", 8, 18, 65, 68)
	moved, sibling := transferButton(t, src, "Moved"), transferButton(t, src, "Sibling")
	moved.layoutCell = cell(1, 0, 1, 2)
	enableFixtureLayout(src)
	priorSlot := placementForChild(sibling)
	priorBounds := moved.Bounds1()
	card := transferWidget(t, s, "gui.Card", "Target", 100, 43, 108, 68)
	card.Widget().(*gui.Card).SetTitle("Free target")
	card.Layout()
	// Release beside the bottom-right edge: the unchanged control should fit
	// inside the real Card content rectangle after clamp.
	x, y := 202.0, 104.0
	transferDrag(v, moved, x, y)
	if hint := v.managedHint; !hint.valid || hint.bounds.IsEmpty() {
		t.Fatalf("missing free Card preview: %+v", hint)
	}
	expect := v.managedHint.bounds
	v.OnLeftUp(v.MapFromScene(x, y))
	if moved.Parent() != card || moved.Bounds1() != expect || moved.layoutCell.Valid ||
		card.contentPlacement != contentFree || placementForChild(sibling) != priorSlot {
		t.Fatalf("free Card attachment or layout cell metadata incorrect: %v %v", moved.Parent(), moved.Bounds1())
	}
	client := card.Widget().(*gui.Card).ContentBounds()
	clientScene := geom.Rect{X: card.X() + gui.PixelToMm(client.X), Y: card.Y() + gui.PixelToMm(client.Y),
		Width: gui.PixelToMm(client.Width), Height: gui.PixelToMm(client.Height)}
	if landed := expect.IntersectCopy(clientScene); !nearTransferPoint(landed.Width, landed.Height, expect.Width, expect.Height) {
		t.Fatalf("preview clipped within Card: %v vs %v", expect, clientScene)
	}
	runtime, err := gui.LoadFormFromDoc(s.SaveDesign())
	if err != nil {
		t.Fatal(err)
	}
	hasTarget := false
	for _, node := range runtime.Children() {
		host, ok := node.(*gui.Card)
		if !ok || host.Title() != "Free target" {
			continue
		}
		hasTarget = true
		if kids := host.Children(); len(kids) != 1 {
			t.Fatalf("SDK did not preserve Card children: %v", kids)
		} else if x, y := kids[0].Pos(); x != roundPx(expect.X)-roundPx(card.X()) || y != roundPx(expect.Y)-roundPx(card.Y()) {
			t.Fatalf("SDK changed Card-local drop position: %v %v", x, y)
		}
	}
	if !hasTarget {
		t.Fatal("SDK did not load the free Card")
	}
	v.Undo()
	if moved.Parent() != src || moved.Bounds1() != priorBounds || moved.layoutCell != cell(1, 0, 1, 2) ||
		sibling.layoutCell.Valid || card.contentPlacement != contentLegacy || card.HasChildren() {
		t.Fatal("Undo did not restore explicit span, implicit sibling slot, and Card policy")
	}
}

type rejectedDetachDraft struct{ attempts int }

func (d *rejectedDetachDraft) HasPendingChanges() bool { return true }
func (d *rejectedDetachDraft) CommitPendingChanges() error {
	d.attempts++
	return errors.New("uncommitted property draft")
}

func TestManagedFreeDropWaitsForPendingEdits(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	src := transferBox(t, s, LayoutVertical, "Source", 9, 17, 60, 65)
	moved := transferButton(t, src, "Moved")
	enableFixtureLayout(src)
	draft := &rejectedDetachDraft{}
	s.AddPendingEditor(draft)
	before := moved.Bounds1()
	transferDrag(v, moved, 160, 95)
	if !v.managedHint.valid || draft.attempts != 0 {
		t.Fatal("preview must not commit the pending editor")
	}
	v.OnLeftUp(v.MapFromScene(160, 95))
	if moved.Parent() != src || moved.Bounds1() != before || s.UndoStack().CanUndo() || draft.attempts != 1 || v.managedHint.active {
		t.Fatal("failed property commit did not cancel the drag")
	}
}

func TestManagedFreeDropRespectsLocalDestinationCoordinates(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	src := transferBox(t, s, LayoutVertical, "Source", 9, 17, 50, 65)
	moved := transferButton(t, src, "Moved")
	enableFixtureLayout(src)
	host := transferWidget(t, s, "gui.GroupBox", "Local target", 125, 36, 100, 75)
	host.SetLocalCoord(true)
	host.Widget().(*gui.GroupBox).SetTitle("Group target")
	host.Layout()
	transferDrag(v, moved, 177, 79)
	hint := v.managedHint
	if !hint.valid {
		t.Fatalf("expected free GroupBox preview: %+v", hint)
	}
	v.OnLeftUp(v.MapFromScene(177, 79))
	got := moved.MapRectToScene1(moved.Bounds1())
	if moved.Parent() != host || !nearTransferPoint(got.X, got.Y, hint.bounds.X, hint.bounds.Y) ||
		moved.X() >= hint.bounds.X || moved.Y() >= hint.bounds.Y || host.contentPlacement != contentFree {
		t.Fatalf("preview differs from mapped GroupBox landing: %+v vs %+v", hint.bounds, got)
	}
}

func TestManagedFreeDropRejectsUnsafeTargets(t *testing.T) {
	for _, name := range []string{"outside", "title", "small", "locked-sibling", "unsupported", "escape", "nested-target-moves"} {
		t.Run(name, func(t *testing.T) {
			v := transferFixture()
			s := v.GedScene()
			src := transferBox(t, s, LayoutVertical, "Source", 9, 17, 70, 65)
			moved := transferButton(t, src, "Moved")
			sibling := transferButton(t, src, "Sibling")
			target := transferWidget(t, s, "gui.Card", "Target", 118, 39, 80, 77)
			target.Widget().(*gui.Card).SetTitle("Free title")
			x, y := 148.0, 80.0
			if name == "nested-target-moves" {
				target.SetParent(src)
			}
			enableFixtureLayout(src)
			target.Layout()
			switch name {
			case "outside":
				x = 250
			case "title":
				y = target.Y() + 0.3
			case "small":
				target.SetSize(18, 16)
				x, y = centerForTransfer(target)
			case "locked-sibling":
				sibling.SetLockPos(true)
			case "unsupported":
				target = transferWidget(t, s, "gui.Splitter", "Splitter", 111, 25, 110, 96)
				x, y = centerForTransfer(target)
			case "nested-target-moves":
				x, y = centerForTransfer(target)
			}
			old := moved.Bounds1()
			transferDrag(v, moved, x, y)
			if name == "escape" {
				v.OnKeyDown(gui.KeyEsc, false)
			}
			v.OnLeftUp(v.MapFromScene(x, y))
			if moved.Parent() != src || moved.Bounds1() != old || s.UndoStack().CanUndo() {
				t.Fatalf("rejected detach mutated scene: %v %v", moved.Parent(), moved.Bounds1())
			}
		})
	}
}

func TestManagedFreeDropCarriesWholeNestedSubtree(t *testing.T) {
	v := transferFixture()
	s := v.GedScene()
	box := transferBox(t, s, LayoutVertical, "Source", 10, 12, 84, 66)
	moved := transferWidget(t, box, "gui.Card", "Moved", 12, 14, 80, 32)
	moved.Widget().(*gui.Card).SetTitle("Grab here")
	grid := transferBox(t, moved, LayoutGrid, "Nested", 15, 20, 45, 17)
	leaf := transferButton(t, grid, "Leaf")
	leaf.layoutCell = cell(0, 0, 1, 1)
	enableFixtureLayout(grid)
	enableFixtureLayout(box)
	rects := func() []geom.Rect {
		res := make([]geom.Rect, 0, 3)
		for _, i := range []graph.IItem{moved, grid, leaf} {
			res = append(res, i.MapRectToScene1(i.Bounds1()))
		}
		return res
	}
	before := rects()
	clearClickHistory()
	v.OnLeftDown(v.MapFromScene(before[0].X+2, before[0].Y+1))
	v.OnMouseMove(v.MapFromScene(118, 98))
	v.OnLeftUp(v.MapFromScene(118, 98))
	after := rects()
	if moved.Parent() != s || grid.Parent() != moved || leaf.Parent() != grid {
		t.Fatal("the nested widget tree did not follow the drop")
	}
	for i := 1; i < len(after); i++ {
		if !nearTransferPoint(after[i].X-after[0].X, after[i].Y-after[0].Y, before[i].X-before[0].X, before[i].Y-before[0].Y) {
			t.Fatalf("nested child moved relative to its new owner: %v before %v", after, before)
		}
	}
	v.Undo()
	for i, b := range rects() {
		if b != before[i] {
			t.Fatalf("undo offset mismatch: %v vs %v", b, before[i])
		}
	}
}
