package ged

import (
	"math"
	"sort"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type tabularDropPlan struct {
	cell, swapCell layoutPlacement
	span           geom.Rect
	swap           *FakeWidget
	valid, noop    bool
	appendRow      bool
	failure        string
}

func isTabularLayout(f *FakeWidget) bool {
	return f.factoryName == string(LayoutGrid) || f.factoryName == string(LayoutForm)
}

func (f *FakeWidget) tabularTracks() gui.LayoutCellTracks {
	if !f.hasCellTracks {
		// An empty or freshly assembled document can receive a drag before
		// its first design reflow. Measure a detached runtime, never the scene.
		if source, ok := f.Generate().(gui.LayoutCellTrackSource); ok {
			f.cellTracks, f.hasCellTracks = source.CellTracks(), true
		}
	}
	return f.cellTracks
}

func trackAt(tracks []gui.LayoutTrack, at float64) (gui.LayoutTrack, bool) {
	i := sort.Search(len(tracks), func(i int) bool { return tracks[i].Start+tracks[i].Size > at })
	if i == len(tracks) || at < tracks[i].Start {
		return gui.LayoutTrack{}, false
	}
	return tracks[i], true
}

func trackByIndex(tracks []gui.LayoutTrack, index int) (gui.LayoutTrack, bool) {
	i := sort.Search(len(tracks), func(i int) bool { return tracks[i].Index >= index })
	if i == len(tracks) || tracks[i].Index != index {
		return gui.LayoutTrack{}, false
	}
	return tracks[i], true
}

func spansIntersect(a, b layoutPlacement) bool {
	return a.Row < b.Row+b.RowSpan && a.Row+a.RowSpan > b.Row &&
		a.Column < b.Column+b.ColumnSpan && a.Column+a.ColumnSpan > b.Column
}

func (f *FakeWidget) nextTabularRow() int {
	next := 0
	for i, it := range f.Children() {
		if child, ok := it.(*FakeWidget); ok {
			p := f.childPlacement(child, i)
			next = max(next, p.Row+p.RowSpan)
		}
	}
	return next
}

func (view *GedView) tabularLayoutDrop(parent *FakeWidget, clip geom.Rect, x, y float64, moving *FakeWidget) tabularDropPlan {
	return view.planTabularFootprint(parent, clip, x, y, moving, layoutPlacement{RowSpan: 1, ColumnSpan: 1})
}

func (view *GedView) planTabularFootprint(parent *FakeWidget, clip geom.Rect, x, y float64, moving *FakeWidget, from layoutPlacement) tabularDropPlan {
	if !pointInDropRect(clip, x, y) {
		return tabularDropPlan{failure: dropCellGap}
	}
	if !unlockedLayoutAncestors(parent) {
		return tabularDropPlan{failure: dropLayoutLocked}
	}
	tracks := parent.tabularTracks()
	origin := parent.MapRectToScene1(parent.Bounds1())
	u, v := gui.MmToPixel(x-origin.X), gui.MmToPixel(y-origin.Y)
	if !pointInDropRect(tracks.Content, u, v) {
		return tabularDropPlan{failure: dropCellGap}
	}
	col, colOK := trackAt(tracks.Columns, u)
	if len(tracks.Columns) == 0 {
		col, colOK = gui.LayoutTrack{Start: tracks.Content.X, Size: tracks.Content.Width}, true
	}
	if !colOK {
		return tabularDropPlan{failure: dropCellGap}
	}
	if moving != nil {
		if moving.IsLockPos() || moving.Parent() != parent {
			return tabularDropPlan{failure: dropLayoutLocked}
		}
		from = placementForChild(moving)
	}
	out := tabularDropPlan{cell: layoutPlacement{Column: col.Index, RowSpan: from.RowSpan, ColumnSpan: from.ColumnSpan, Valid: true}}
	row, rowOK := trackAt(tracks.Rows, v)
	// Form rows do not normally fill the container. Blank space below the
	// last real row appends. A grid fills its flex tracks, so its last
	// device-sized edge band is the explicit append target for palette only.
	appendY := tracks.Content.Bottom()
	if len(tracks.Rows) != 0 {
		last := tracks.Rows[len(tracks.Rows)-1]
		appendY = math.Min(appendY, last.Start+last.Size)
	}
	band := math.Min(tracks.Content.Height/4, 8/math.Max(view.ZoomFactor(), .05))
	appendAtEdge := moving == nil && parent.factoryName == string(LayoutGrid) && v >= appendY-band
	if len(tracks.Rows) == 0 || (!rowOK && v >= appendY) || appendAtEdge {
		if moving != nil {
			return tabularDropPlan{failure: dropCellGap}
		}
		out.appendRow = true
		out.cell.Row = parent.nextTabularRow()
		if out.cell.Row >= 4096 {
			return tabularDropPlan{failure: dropCellGap}
		}
		rowY := math.Max(tracks.Content.Y, math.Min(appendY, tracks.Content.Bottom()-band))
		if len(tracks.Rows) == 0 {
			rowY = tracks.Content.Y
		}
		row = gui.LayoutTrack{Index: out.cell.Row, Start: rowY, Size: math.Max(band, tracks.Content.Bottom()-rowY)}
	} else if rowOK {
		out.cell.Row = row.Index
	} else {
		return tabularDropPlan{failure: dropCellGap}
	}
	right, bottom := col.Start+col.Size, row.Start+row.Size
	if (from.RowSpan > 1 || from.ColumnSpan > 1) && len(tracks.Rows) != 0 {
		lastRow, validRow := trackByIndex(tracks.Rows, row.Index+from.RowSpan-1)
		lastCol, validCol := trackByIndex(tracks.Columns, col.Index+from.ColumnSpan-1)
		if !validRow || !validCol {
			return tabularDropPlan{failure: dropCellGap}
		}
		right, bottom = lastCol.Start+lastCol.Size, lastRow.Start+lastRow.Size
	}
	out.span = geom.Rect{X: origin.X + gui.PixelToMm(col.Start), Y: origin.Y + gui.PixelToMm(row.Start),
		Width: gui.PixelToMm(right - col.Start), Height: gui.PixelToMm(bottom - row.Start)}.IntersectCopy(clip)
	if out.span.IsEmpty() {
		return tabularDropPlan{failure: dropCellGap}
	}
	if moving != nil && out.cell == from {
		return tabularDropPlan{noop: true}
	}
	siblings := parent.Children()
	for i, candidate := range siblings {
		child, ok := candidate.(*FakeWidget)
		if !ok || child == moving {
			continue
		}
		occupied := parent.childPlacement(child, i)
		if !spansIntersect(occupied, out.cell) {
			continue
		}
		// Never replace a registered field/widget. Only a deliberate move
		// between equivalent complete footprints may swap two occupants.
		if moving == nil || out.swap != nil || child.IsLockPos() || occupied != out.cell {
			return tabularDropPlan{span: out.span, failure: dropCellTaken}
		}
		out.swap, out.swapCell = child, from
	}
	if out.swap != nil {
		for i, candidate := range siblings {
			child, ok := candidate.(*FakeWidget)
			if ok && child != moving && child != out.swap && spansIntersect(parent.childPlacement(child, i), from) {
				return tabularDropPlan{span: out.span, failure: dropCellTaken}
			}
		}
	}
	out.valid = true
	return out
}

func unlockedLayoutAncestors(parent *FakeWidget) bool {
	for p := graph.IItem(parent); p != nil; p = p.Parent() {
		if fw, ok := p.(*FakeWidget); ok && fw.IsLockPos() {
			return false
		}
	}
	return true
}

type tabularMoveCommand struct {
	parent, item, other   *FakeWidget
	before, after         layoutPlacement
	otherBefore, otherNew layoutPlacement
}

func (c *tabularMoveCommand) Text() string { return i18n.T(dropMoveCellCommand) }
func (c *tabularMoveCommand) Redo()        { c.apply(c.after, c.otherNew) }
func (c *tabularMoveCommand) Undo()        { c.apply(c.before, c.otherBefore) }
func (c *tabularMoveCommand) apply(a, b layoutPlacement) {
	c.item.layoutCell = a
	if c.other != nil {
		c.other.layoutCell = b
	}
	c.parent.Layout()
}

// Reflow can affect any control in the grid or form, even if its cell is not
// the drop target. Check the detached result on release, not on every motion.
func (view *GedView) tabularReleaseFailure(parent, item *FakeWidget, plan tabularDropPlan, add bool) string {
	scene := view.Scene()
	if scene == nil || parent.Scene() != scene || (!add && item.Parent() != parent) {
		return dropCellClipped
	}
	page := scene.MapRectToScene1(scene.Bounds1())
	clip := designDropArea(page, parent)
	if clip.IsEmpty() {
		return dropCellClipped
	}
	trial := &layoutTrial{
		children: make(map[*FakeWidget]gui.IWidget),
		orders:   make(map[*FakeWidget][]graph.IItem),
	}
	if !add {
		trial.cells = map[*FakeWidget]layoutPlacement{item: plan.cell}
		if plan.swap != nil {
			trial.cells[plan.swap] = plan.swapCell
		}
	}
	runtime := parent.generate(trial)
	layouter, hasLayout := runtime.(interface{ Layout() })
	if !hasLayout {
		return dropCellClipped
	}
	if add {
		adder, ok := runtime.(layoutWidgetAdder)
		if !ok {
			return dropCellClipped
		}
		proposed := item.Generate()
		if proposed == nil {
			return dropCellClipped
		}
		adder.AddLayoutWidget(proposed, plan.cell.Row, plan.cell.Column, plan.cell.RowSpan, plan.cell.ColumnSpan)
		trial.children[item] = proposed
	}
	layouter.Layout()
	return tabularTrialFailure(parent, runtime, trial, page, clip, item, plan.swap, add)
}

func tabularTrialFailure(parent *FakeWidget, runtime gui.IWidget, trial *layoutTrial, page, clip geom.Rect, item, swap *FakeWidget, add bool) string {
	origin := parent.MapRectToScene1(parent.Bounds1())
	var baseline *layoutTrial
	var baselineRoot gui.IWidget
	for actual, proposed := range trial.children {
		if actual == parent {
			continue
		}
		rect, visible := transferTrialBounds(proposed, runtime, origin, clip)
		if actual == item || actual == swap {
			if proposed.Parent() != runtime || !visible {
				return dropCellClipped
			}
		}
		if actual == item && add {
			continue
		}
		oldRect := actual.MapRectToScene1(actual.Bounds1())
		wasVisible := actual.Widget().IsVisible() && !oldRect.IntersectCopy(designDropArea(page, actual.Parent())).IsEmpty()
		if wasVisible && !visible {
			return dropCellClipped
		}
		if !tabularLockAllows(actual, oldRect, rect) {
			// Free descendants can be authored at subpixel coordinates.
			// Compare with the same solver before the edit to distinguish
			// a real reflow from rounding on projection into runtime pixels.
			if baseline == nil {
				baseline = &layoutTrial{
					children: make(map[*FakeWidget]gui.IWidget),
					orders:   make(map[*FakeWidget][]graph.IItem),
				}
				baselineRoot = parent.generate(baseline)
			}
			old := baseline.children[actual]
			if old == nil || baselineRoot == nil {
				return dropCellLockedGeometry
			}
			before, _ := transferTrialBounds(old, baselineRoot, origin, clip)
			if !tabularLockAllows(actual, before, rect) {
				return dropCellLockedGeometry
			}
		}
	}
	if trial.children[item] == nil || (swap != nil && trial.children[swap] == nil) {
		return dropCellClipped
	}
	return ""
}

func tabularLockAllows(item *FakeWidget, before, after geom.Rect) bool {
	return (!item.IsLockPos() || nearTransferPoint(before.X, before.Y, after.X, after.Y)) &&
		(!item.IsLockSize() || nearTransferPoint(before.Width, before.Height, after.Width, after.Height))
}

func (view *GedView) tabularGestureAt(x, y float64) tabularDropPlan {
	arm := view.layoutReorder
	if !arm.moved || arm.parent == nil || arm.item == nil || arm.item.Parent() != arm.parent ||
		arm.parent.Scene() != view.Scene() || !arm.parent.layoutManaged || !isTabularLayout(arm.parent) {
		return tabularDropPlan{failure: dropCellGap}
	}
	sx, sy := view.MapToScene(x, y)
	if !finiteDropCoordinate(sx) || !finiteDropCoordinate(sy) || (view.showGuides && (x < rulerThicknessPx || y < rulerThicknessPx)) {
		return tabularDropPlan{failure: dropCellGap}
	}
	page := view.Scene().MapRectToScene1(view.Scene().Bounds1())
	return view.tabularLayoutDrop(arm.parent, designDropArea(page, arm.parent), sx, sy, arm.item)
}

func (view *GedView) updateTabularDrag(x, y float64) {
	cell := view.tabularGestureAt(x, y)
	if cell.noop {
		view.clearManagedGeometryHint()
		return
	}
	message := i18n.T(cell.failure)
	if cell.valid {
		format := dropMoveCell
		if cell.swap != nil {
			format = dropSwapCell
		}
		message = i18n.Tf(format, cell.cell.Row+1, cell.cell.Column+1)
	}
	view.managedHint = paletteDropFeedback{active: true, valid: cell.valid, autoLayout: true, x: x, y: y, cell: cell, hint: message}
	view.Self().Update()
}

func (view *GedView) canCommitTabularDrop() bool {
	if scene := view.GedScene(); scene != nil {
		if err := scene.CommitPendingEdits(); err != nil {
			core.Warn(err)
			return false
		}
	}
	return true
}

func (view *GedView) commitManagedDrag(x, y float64) {
	arm := view.layoutReorder
	if transfer := view.managedTransferAt(x, y); transfer.destination() != nil {
		if transfer.valid() && view.canCommitTabularDrop() {
			next := view.managedTransferAt(x, y)
			if next.destination() == transfer.destination() && (next.freeParent != nil) == (transfer.freeParent != nil) {
				transfer = next
			} else {
				transfer = layoutTransferPlan{}
			}
		} else {
			transfer = layoutTransferPlan{}
		}
		view.finishManagedPress()
		if cmd := view.buildLayoutTransfer(arm.item, transfer); cmd != nil {
			view.Scene().PushCommand(cmd)
		}
		return
	}
	if arm.parent != nil && isTabularLayout(arm.parent) {
		cell := view.tabularGestureAt(x, y)
		if cell.valid {
			if !view.canCommitTabularDrop() {
				view.finishManagedPress()
				return
			}
			cell = view.tabularGestureAt(x, y)
		}
		view.finishManagedPress()
		if !cell.valid {
			return
		}
		if reason := view.tabularReleaseFailure(arm.parent, arm.item, cell, false); reason != "" {
			view.showStatus(i18n.T(reason))
			return
		}
		cmd := &tabularMoveCommand{parent: arm.parent, item: arm.item, other: cell.swap,
			before: arm.item.layoutCell, after: cell.cell, otherNew: cell.swapCell}
		if cell.swap != nil {
			cmd.otherBefore = cell.swap.layoutCell
		}
		view.Scene().PushCommand(cmd)
		return
	}
	slot, from := view.layoutReorderSlot(x, y)
	view.finishManagedPress()
	if slot.valid && arm.item.Parent() == arm.parent && arm.parent.Scene() == view.Scene() && arm.item.IndexInParent() == from {
		view.Scene().PushCommand(&linearLayoutOrderCommand{item: arm.item, parent: arm.parent, from: from, to: slot.index})
	}
}
