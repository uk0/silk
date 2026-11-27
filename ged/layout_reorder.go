package ged

import (
	"math"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type layoutReorderGesture struct {
	item, parent *FakeWidget
	moved        bool
	grabX, grabY float64
}

func (view *GedView) managedReorderPress(x, y float64) layoutReorderGesture {
	if view.Selection().Count() > 1 || designerActionKeyDown() || keyDown(gui.KeyShift) || keyDown(gui.KeyMenu) {
		return layoutReorderGesture{}
	}
	sx, sy := view.MapToScene(x, y)
	if decor, _ := view.FindHandleAt(sx, sy); decor != nil {
		return layoutReorderGesture{}
	}
	item, ok := view.Scene().FindItemAt(sx, sy, nil).(*FakeWidget)
	if !ok || item.IsLockPos() {
		return layoutReorderGesture{}
	}
	parent, ok := item.Parent().(*FakeWidget)
	if !ok || !parent.layoutManaged || !unlockedLayoutAncestors(parent) ||
		(!isTabularLayout(parent) && !canReorderLinearLayout(parent)) {
		return layoutReorderGesture{}
	}
	r := item.MapRectToScene1(item.Bounds1())
	if r.IsEmpty() {
		return layoutReorderGesture{}
	}
	return layoutReorderGesture{item: item, parent: parent,
		grabX: min(1, max(0, (sx-r.X)/r.Width)),
		grabY: min(1, max(0, (sy-r.Y)/r.Height))}
}

func canReorderLinearLayout(parent *FakeWidget) bool {
	if !parent.layoutManaged || (parent.factoryName != string(LayoutHorizontal) && parent.factoryName != string(LayoutVertical)) {
		return false
	}
	return unlockedLayoutAncestors(parent)
}

func (view *GedView) layoutReorderSlot(x, y float64) (linearPaletteInsertion, int) {
	arm := view.layoutReorder
	invalid := linearPaletteInsertion{}
	if !arm.moved || arm.item == nil || arm.item.Parent() != arm.parent || arm.parent.Scene() != view.Scene() ||
		arm.item.IsLockPos() || !canReorderLinearLayout(arm.parent) {
		return invalid, -1
	}
	sx, sy := view.MapToScene(x, y)
	if !finiteDropCoordinate(sx) || !finiteDropCoordinate(sy) || (view.showGuides && (x < rulerThicknessPx || y < rulerThicknessPx)) {
		return invalid, -1
	}
	page := view.Scene().MapRectToScene1(view.Scene().Bounds1())
	clip := designDropArea(page, arm.parent)
	if !pointInDropRect(clip, sx, sy) {
		return invalid, -1
	}
	siblings := arm.parent.Children()
	from := arm.item.IndexInParent()
	if len(siblings) < 2 || from < 0 || from >= len(siblings) || siblings[from] != arm.item {
		return invalid, -1
	}
	slot := linearLayoutSlot(arm.parent, clip, sx, sy, siblings, arm.item)
	if !slot.valid || slot.index == from || slot.index >= len(siblings) {
		return invalid, from
	}
	// A locked sibling must not change index as an indirect side effect.
	for k := min(from, slot.index); k <= max(from, slot.index); k++ {
		if siblings[k].IsLockPos() {
			return invalid, -1
		}
	}
	return slot, from
}

func (view *GedView) updateManagedPress(x, y float64) {
	dx, dy := x-view.managedPressX, y-view.managedPressY
	if view.layoutReorder.item == nil {
		if math.Abs(dx) > 1 || math.Abs(dy) > 1 {
			view.showManagedGeometryHint(x, y)
		}
		return
	}
	if dx*dx+dy*dy >= 16 {
		view.layoutReorder.moved = true
	}
	if !view.layoutReorder.moved {
		return
	}
	if transfer := view.managedTransferAt(x, y); transfer.destination() != nil {
		view.showLayoutTransfer(transfer, x, y)
		return
	}
	if isTabularLayout(view.layoutReorder.parent) {
		view.updateTabularDrag(x, y)
		return
	}
	slot, from := view.layoutReorderSlot(x, y)
	if !slot.valid {
		if from < 0 {
			view.showManagedGeometryHint(x, y)
		} else {
			view.clearManagedGeometryHint()
		}
		return
	}
	view.managedHint = paletteDropFeedback{active: true, valid: true, autoLayout: true, x: x, y: y, insertion: slot,
		hint: i18n.Tf(dropReorderSlot, slot.ordinal)}
	view.Self().Update()
}

func (view *GedView) finishManagedPress() {
	// Keep normal click/selection semantics, never send a geometry drag.
	x, y := view.managedPressX, view.managedPressY
	view.managedPress = false
	view.layoutReorder = layoutReorderGesture{}
	view.managedHint = paletteDropFeedback{}
	view.isDragging = false
	view.alignGuides = nil
	view.dropContainer = nil
	view.GraphView.OnLeftUp(x, y)
	view.Self().Update()
}

// OnPointerCancel retires our managed drag if native capture is lost.
func (view *GedView) OnPointerCancel() {
	if view.managedPress {
		view.finishManagedPress()
	}
}

type linearLayoutOrderCommand struct {
	item, parent *FakeWidget
	from, to     int
}

func (c *linearLayoutOrderCommand) Text() string { return i18n.T(reorderLayoutCommand) }
func (c *linearLayoutOrderCommand) Redo()        { c.move(c.to) }
func (c *linearLayoutOrderCommand) Undo()        { c.move(c.from) }
func (c *linearLayoutOrderCommand) move(index int) {
	if c.item.Parent() == c.parent && graph.MoveSiblingToIndex(c.item, index) {
		c.parent.Layout()
	}
}
