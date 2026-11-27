package ged

import (
	"math"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type layoutTransferPlan struct {
	target     *FakeWidget
	freeParent graph.IItem
	freeBounds geom.Rect
	slot       linearPaletteInsertion
	cell       tabularDropPlan
	reject     string
}

func (p layoutTransferPlan) destination() graph.IItem {
	if p.target != nil {
		return p.target
	}
	return p.freeParent
}

func (p layoutTransferPlan) valid() bool {
	return p.reject == "" && (p.target != nil && (p.slot.valid || p.cell.valid) ||
		p.freeParent != nil && !p.freeBounds.IsEmpty())
}

func managedLayoutTarget(item graph.IItem) *FakeWidget {
	box, ok := item.(*FakeWidget)
	if ok && box.layoutManaged && isDesignerLayoutFactory(box.factoryName) {
		return box
	}
	return nil
}

func (view *GedView) layoutTransferAt(item *FakeWidget, target *FakeWidget, x, y float64) layoutTransferPlan {
	p := layoutTransferPlan{target: target}
	if target == nil || (item != nil && item.Parent() == target) {
		return layoutTransferPlan{}
	}
	if item == nil {
		p.reject = dropTransferSingle
		return p
	}
	scene := view.Scene()
	if scene == nil || target.Scene() != scene || item.Scene() != scene || item.Parent() == nil ||
		!unlockedLayoutAncestors(item) || !unlockedLayoutAncestors(target) || item.IsLockSize() {
		p.reject = dropLayoutLocked
		return p
	}
	for node := graph.IItem(target); node != nil; node = node.Parent() {
		if node == item {
			p.reject = dropCellGap
			return p
		}
	}
	page := scene.MapRectToScene1(scene.Bounds1())
	clip := designDropArea(page, target)
	if !pointInDropRect(clip, x, y) {
		p.reject = dropCellGap
		return p
	}
	shape := layoutPlacement{RowSpan: 1, ColumnSpan: 1}
	if source := managedLayoutTarget(item.Parent()); source != nil && isTabularLayout(source) {
		shape = placementForChild(item)
	}
	if (shape.RowSpan != 1 || shape.ColumnSpan != 1) && target.factoryName != string(LayoutGrid) {
		p.reject = dropTransferSpan
		return p
	}
	if isTabularLayout(target) {
		p.cell = view.planTabularFootprint(target, clip, x, y, nil, shape)
		p.reject = p.cell.failure
	} else {
		p.slot = linearPaletteSlot(target, clip, x, y)
	}
	return p
}

func (view *GedView) managedTransferAt(x, y float64) layoutTransferPlan {
	arm := view.layoutReorder
	if !arm.moved || arm.item == nil || arm.item.Parent() != arm.parent || view.Scene() == nil ||
		(view.showGuides && (x < rulerThicknessPx || y < rulerThicknessPx)) {
		return layoutTransferPlan{}
	}
	sx, sy := view.MapToScene(x, y)
	if !finiteDropCoordinate(sx) || !finiteDropCoordinate(sy) {
		return layoutTransferPlan{}
	}
	target := nearestDropContainer(view.Scene().FindItemAt(sx, sy, nil), func(it graph.IItem) bool { return it == arm.item })
	if box := managedLayoutTarget(target); box != nil {
		return view.layoutTransferAt(arm.item, box, sx, sy)
	}
	return view.layoutDetachAt(arm, target, sx, sy)
}

func (view *GedView) showLayoutTransfer(p layoutTransferPlan, x, y float64) {
	hint := i18n.T(p.reject)
	if p.freeParent != nil {
		if p.valid() {
			hint = i18n.Tf(dropTransferFreeHint, freeDropTargetName(p.freeParent))
		}
		view.managedHint = paletteDropFeedback{active: true, valid: p.valid(), freeMove: true,
			x: x, y: y, bounds: p.freeBounds, hint: hint}
		view.Self().Update()
		return
	}
	if p.valid() {
		target := p.target.WidgetName()
		if target == "" {
			target = widgetFriendlyName(p.target.WidgetFactoryName())
		}
		if p.cell.valid {
			hint = i18n.Tf(dropTransferCell, target, p.cell.cell.Row+1, p.cell.cell.Column+1)
		} else {
			hint = i18n.Tf(dropTransferOrder, target, p.slot.ordinal)
		}
	}
	view.managedHint = paletteDropFeedback{active: true, valid: p.valid(), autoLayout: true, x: x, y: y,
		hint: hint, cell: p.cell, insertion: p.slot}
	view.Self().Update()
}

func singleTransferItem(sel *graph.Selection) *FakeWidget {
	if sel.Count() != 1 {
		return nil
	}
	f, _ := sel.ItemList()[0].(*FakeWidget)
	return f
}

type layoutTransferCommand struct {
	view             *GedView
	item, target     *FakeWidget
	source           graph.IItem
	oldSlot, newSlot int
	cell             layoutPlacement
	before           []layoutSnapshot
	roots            []*FakeWidget
	implicitSource   map[*FakeWidget]layoutPlacement
	freeParent       graph.IItem
	freeBounds       geom.Rect
	freeContentHost  *FakeWidget
	oldContentMode   contentPlacementMode
}

func transferRoots(source, target *FakeWidget) []*FakeWidget {
	if source == nil {
		return []*FakeWidget{target}
	}
	for p := target.Parent(); p != nil; p = p.Parent() {
		if p == source {
			return []*FakeWidget{source}
		}
	}
	for p := source.Parent(); p != nil; p = p.Parent() {
		if p == target {
			return []*FakeWidget{target}
		}
	}
	return []*FakeWidget{source, target}
}

func (view *GedView) buildLayoutTransfer(item *FakeWidget, plan layoutTransferPlan) gui.ICommand {
	if !plan.valid() || item == nil || item.Parent() == nil || item.Parent() == plan.destination() {
		return nil
	}
	cmd := &layoutTransferCommand{view: view, item: item, source: item.Parent(), target: plan.target,
		oldSlot: item.IndexInParent(), newSlot: -1}
	if plan.slot.valid {
		cmd.newSlot = plan.slot.index
	} else {
		cmd.cell = plan.cell.cell
	}
	srcLayout := managedLayoutTarget(cmd.source)
	if plan.freeParent != nil {
		if srcLayout == nil || !unlockedDetachSubtree(item) {
			view.showStatus(i18n.T(dropLayoutLocked))
			return nil
		}
		cmd.freeParent, cmd.freeBounds = plan.freeParent, plan.freeBounds
		cmd.roots = []*FakeWidget{srcLayout}
		if host, ok := cmd.freeParent.(*FakeWidget); ok && host.supportsContentLayout() {
			cmd.freeContentHost, cmd.oldContentMode = host, host.contentPlacement
		}
	} else {
		cmd.roots = transferRoots(srcLayout, cmd.target)
	}
	if srcLayout != nil && isTabularLayout(srcLayout) {
		cmd.implicitSource = make(map[*FakeWidget]layoutPlacement)
		for i, node := range srcLayout.Children() {
			if w, ok := node.(*FakeWidget); ok && w != item && !w.layoutCell.Valid {
				// Legacy implicit slots describe the original sibling index.
				// Do not shift remaining cells simply by removing an earlier item.
				cmd.implicitSource[w] = srcLayout.childPlacement(w, i)
			}
		}
	}

	seen := make(map[*FakeWidget]bool)
	var capture func(*FakeWidget)
	capture = func(f *FakeWidget) {
		if seen[f] {
			return
		}
		seen[f] = true
		cmd.before = append(cmd.before, layoutSnapshot{item: f, bounds: f.Bounds1(), placement: f.layoutCell})
		for _, node := range f.Children() {
			if child, ok := node.(*FakeWidget); ok {
				capture(child)
			}
		}
	}
	for _, root := range cmd.roots {
		capture(root)
	}
	capture(item)
	if !cmd.trialFits(srcLayout) {
		view.showStatus(i18n.T(dropTransferUnsafe))
		return nil
	}
	return cmd
}

func (c *layoutTransferCommand) trialFits(source *FakeWidget) bool {
	trial := &layoutTrial{
		cells: make(map[*FakeWidget]layoutPlacement), children: make(map[*FakeWidget]gui.IWidget),
		orders: make(map[*FakeWidget][]graph.IItem),
	}
	for f, p := range c.implicitSource {
		trial.cells[f] = p
	}
	trial.cells[c.item] = c.cell
	if source != nil {
		src := source.Children()
		order := make([]graph.IItem, 0, len(src)-1)
		for _, node := range src {
			if node != c.item {
				order = append(order, node)
			}
		}
		trial.orders[source] = order
	}
	if c.target != nil {
		dst := c.target.Children()
		insert := c.newSlot
		if insert < 0 {
			insert = len(dst)
		}
		order := make([]graph.IItem, 0, len(dst)+1)
		order = append(order, dst[:insert]...)
		order = append(order, c.item)
		trial.orders[c.target] = append(order, dst[insert:]...)
	}

	page := c.view.Scene().MapRectToScene1(c.view.Scene().Bounds1())
	found := false
	for _, root := range c.roots {
		// Each detached root gets its own result map. Nested root pairs share
		// one solver tree so changes to a common manager aren't approximated.
		clear(trial.children)
		runtime := root.generate(trial)
		if runtime == nil {
			return false
		}
		rootRect := root.MapRectToScene1(root.Bounds1())
		rootClip := designDropArea(page, root)
		for _, old := range c.before {
			f := old.item
			w := trial.children[f]
			if w == nil || f == root {
				continue
			}
			box, visible := transferTrialBounds(w, runtime, rootRect, rootClip)
			oldRect := f.MapRectToScene1(f.Bounds1())
			wasVisible := f.Widget().IsVisible() && !oldRect.IntersectCopy(designDropArea(page, f.Parent())).IsEmpty()
			// A free destination under the source can itself be moved or
			// resized by reflow. Never drop into a rectangle that disappears
			// from the point we just previewed.
			if f == c.freeParent && (!visible || !nearTransferPoint(box.X, box.Y, oldRect.X, oldRect.Y) ||
				!nearTransferPoint(box.Width, box.Height, oldRect.Width, oldRect.Height)) {
				return false
			}
			if f == c.item {
				found = visible && w.Parent() == trial.children[c.target]
				if !found {
					return false
				}
			} else if wasVisible && !visible {
				return false
			}
			if (f.IsLockPos() && !nearTransferPoint(box.X, box.Y, oldRect.X, oldRect.Y)) ||
				(f.IsLockSize() && !nearTransferPoint(box.Width, box.Height, oldRect.Width, oldRect.Height)) {
				return false
			}
		}
	}
	return found || c.freeParent != nil
}

func nearTransferPoint(x, y, x0, y0 float64) bool {
	return math.Abs(x-x0) < 1e-5 && math.Abs(y-y0) < 1e-5
}

func transferTrialBounds(w, root gui.IWidget, rootRect, clip geom.Rect) (geom.Rect, bool) {
	var path []gui.IWidget
	visible := true
	for node := w; node != root; node = node.Parent() {
		if node == nil {
			return geom.Rect{}, false
		}
		visible = visible && node.IsVisible()
		path = append(path, node)
	}
	box := rootRect
	for i := len(path) - 1; i >= 0; i-- {
		x, y, width, height := path[i].Bounds()
		box = geom.Rect{X: box.X + gui.PixelToMm(x), Y: box.Y + gui.PixelToMm(y),
			Width: gui.PixelToMm(width), Height: gui.PixelToMm(height)}
		clip = clip.IntersectCopy(box)
	}
	return box, visible && !clip.IsEmpty()
}

func (c *layoutTransferCommand) Text() string {
	if c.freeParent != nil {
		return i18n.T(dropTransferFreeCommand)
	}
	return i18n.T(dropTransferCommand)
}

func (c *layoutTransferCommand) Redo() {
	c.keepSelection(func() {
		for item, p := range c.implicitSource {
			item.layoutCell = p
		}
		c.item.layoutCell = c.cell
		if c.freeParent != nil {
			if c.freeContentHost != nil {
				c.freeContentHost.contentPlacement = contentFree
			}
			moveSubtreeToFreeBounds(c.item, c.freeParent, c.freeBounds)
			c.roots[0].Layout()
			return
		}
		x, y := c.item.Pos()
		tx, ty := graph.TranslateBetweenParents(c.source, c.target, x, y)
		c.item.SetParentAt(c.target, c.newSlot)
		c.item.SetPos(tx, ty)
		if !c.item.ManagesChildGeometry() {
			translateFreeChildren(c.item, tx-x, ty-y)
		}
		for _, root := range c.roots {
			root.Layout()
		}
		// A nested target can keep its size and position throughout an outer
		// reflow. It still needs to pick up the freshly attached control.
		if len(c.roots) == 1 && c.roots[0] != c.target {
			c.target.Layout()
		}
		if source := managedLayoutTarget(c.source); source != nil && len(c.roots) == 1 && c.roots[0] != source {
			source.Layout()
		}
	})
}

func (c *layoutTransferCommand) Undo() {
	c.keepSelection(func() {
		c.item.SetParentAt(c.source, c.oldSlot)
		if c.freeContentHost != nil {
			c.freeContentHost.contentPlacement = c.oldContentMode
		}
		for _, point := range c.before {
			point.item.layoutCell = point.placement
		}
		for _, point := range c.before {
			point.item.SetBounds1(point.bounds)
			point.item.hasCellTracks = false
			point.item.MarkDirty()
		}
	})
}

func (c *layoutTransferCommand) keepSelection(apply func()) {
	sel := c.view.Selection()
	previous := sel.ItemList()
	apply()
	sel.Clear()
	sel.AddMulti(previous)
}
