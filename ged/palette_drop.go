package ged

import (
	"fmt"
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// paletteDropFeedback is both the hover preview and the release plan. Keep
// the geometry in scene millimetres so zoom/scroll cannot change the result.
type paletteDropFeedback struct {
	active, valid bool
	autoLayout    bool
	factoryName   string
	container     graph.IItem
	parent        graph.IItem
	bounds        geom.Rect
	x, y          float64 // pointer in view pixels, for the legible hint badge
	hint          string
	insertion     linearPaletteInsertion
}

func (view *GedView) resolvePaletteDrop(x, y float64, dnd gui.IDndContext) paletteDropFeedback {
	p := paletteDropFeedback{active: true, x: x, y: y, hint: i18n.T("Drag a widget from the palette")}
	if !finiteDropCoordinate(x) || !finiteDropCoordinate(y) {
		p.x, p.y = 0, 0
		return p
	}
	if dnd == nil || !dnd.HasFormat("text/plain") || dnd.PosibleActions()&gui.DndCopy == 0 {
		return p
	}
	name, ok := dnd.Data("text/plain").(string)
	if !ok || !isPlaceable(name) || core.FindFactory(name) == nil {
		return p
	}
	p.factoryName = name
	sx, sy := view.MapToScene(x, y)
	if !finiteDropCoordinate(sx) || !finiteDropCoordinate(sy) || view.Scene() == nil {
		return p
	}
	page := view.Scene().MapRectToScene1(view.Scene().Bounds1())
	if !pointInDropRect(page, sx, sy) || (view.showGuides && (x < rulerThicknessPx || y < rulerThicknessPx)) {
		p.hint = i18n.T("Drop inside the form")
		return p
	}
	p.parent = view.Scene()
	p.container = view.containerUnder(sx, sy)
	if p.container != nil {
		p.parent = p.container
	}

	// All ancestors clip their children. A partly off-page nested container
	// may have less usable space than its own rectangle promises.
	available := designDropArea(page, p.parent)
	w, h := defaultSizeForWidget(name)
	if fw, ok := p.container.(*FakeWidget); ok {
		p.autoLayout = fw.layoutManaged
	}
	if available.IsEmpty() || (!p.autoLayout && (w > available.Width || h > available.Height)) {
		p.container = nil
		p.hint = i18n.T(dropContainerTooSmall)
		return p
	}
	x1, y1 := view.snapToGrid(sx, sy)
	// Preserve the default size, preferring full visibility over a final
	// half-pitch snap outside the form/container. The same fitted rectangle is
	// shown while hovering, rather than jumping somewhere else on release.
	x1 = math.Max(available.X, math.Min(x1, available.Right()-w))
	y1 = math.Max(available.Y, math.Min(y1, available.Bottom()-h))
	p.bounds = geom.Rect{X: x1, Y: y1, Width: w, Height: h}
	p.valid = true
	target := i18n.T("Form")
	if fw, ok := p.container.(*FakeWidget); ok {
		target = widgetFriendlyName(fw.WidgetFactoryName())
		if fw.WidgetName() != "" {
			target = fw.WidgetName()
		}
	}
	p.hint = fmt.Sprintf("%s · %.0f × %.0f mm → %s", widgetFriendlyName(name), w, h, target)
	if p.autoLayout {
		if fw, ok := p.parent.(*FakeWidget); ok {
			p.insertion = linearPaletteSlot(fw, available, sx, sy)
		}
		if p.insertion.valid {
			p.hint = i18n.Tf(dropInsertSlot, widgetFriendlyName(name), target, p.insertion.ordinal)
		} else {
			p.hint = i18n.Tf(dropAppendLayout, widgetFriendlyName(name), target)
		}
	}
	return p
}

func designDropArea(page geom.Rect, parent graph.IItem) geom.Rect {
	visible := page
	for p := parent; p != nil; p = p.Parent() {
		visible = visible.IntersectCopy(p.MapRectToScene1(p.Bounds1()))
	}
	return visible
}

func finiteDropCoordinate(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func pointInDropRect(r geom.Rect, x, y float64) bool {
	return x >= r.X && y >= r.Y && x < r.Right() && y < r.Bottom()
}

func (view *GedView) clearPaletteDrop() {
	view.paletteDrop = paletteDropFeedback{}
	view.dropContainer = nil
	view.Self().Update()
}

func (view *GedView) drawPaletteDropBounds(g paint.Painter) {
	p := view.paletteDrop
	if !p.active && view.managedHint.insertion.valid {
		p = view.managedHint
	}
	if !p.active || !p.valid || (p.autoLayout && !p.insertion.valid) {
		return
	}
	g.Save()
	defer g.Restore()
	color := paint.CurrentIconPalette().Accent
	// Device-independent thickness so the slot is legible at every zoom.
	scale := gui.ScreenDpmm() * view.ZoomFactor()
	if !finiteDropCoordinate(scale) || scale <= 0 {
		scale = 1
	}
	if p.insertion.valid {
		line := p.insertion.marker
		g.SetPen1(color, 3/scale)
		g.MoveTo(line.X, line.Y)
		g.LineTo(line.Right(), line.Bottom())
		g.Stroke()
		return
	}
	fill := color
	fill.A = 28
	g.SetBrush1(fill)
	g.Rectangle1(p.bounds)
	g.Fill()
	g.SetPen1(color, 2/scale)
	g.Rectangle1(p.bounds)
	g.Stroke()
}

func (view *GedView) drawPaletteDropHint(g paint.Painter) {
	p := view.paletteDrop
	reorder := !p.active && view.managedHint.insertion.valid
	if !p.active {
		p = view.managedHint
	}
	if !p.active || view.Width() <= 16 || view.Height() <= 28 {
		return
	}
	g.Save()
	defer g.Restore()
	font := gui.Theme().Font
	g.SetFont(font)
	width := math.Min(view.Width()-8, font.TextExtents(p.hint).XAdvance+40)
	const height = 26.0
	x := math.Max(4, math.Min(p.x+14, view.Width()-width-4))
	y := math.Max(4, math.Min(p.y+18, view.Height()-height-4))
	g.Rectangle(x, y, width, height)
	g.Clip()
	g.SetBrush1(gui.Theme().FormColor)
	g.Rectangle(x, y, width, height)
	g.Fill()
	color := paint.CurrentIconPalette().Accent
	if !p.valid {
		color = paint.CurrentIconPalette().Danger
	}
	g.SetPen1(color, 1)
	g.Rectangle(x+0.5, y+0.5, width-1, height-1)
	g.Stroke()
	// Paint in the owner canvas so the badge never needs a separate surface.
	g.SetPen1(color, 1.75)
	switch {
	case reorder:
		drawReorderArrows(g, x+15, y+13, p.insertion.marker.Height == 0)
	case p.valid:
		g.MoveTo(x+9, y+13)
		g.LineTo(x+21, y+13)
		g.MoveTo(x+15, y+7)
		g.LineTo(x+15, y+19)
	default:
		g.MoveTo(x+10, y+8)
		g.LineTo(x+20, y+18)
		g.MoveTo(x+10, y+18)
		g.LineTo(x+20, y+8)
	}
	g.Stroke()
	g.SetBrush1(gui.Theme().TextColor)
	fe := font.FontExtents()
	g.DrawText1(x+29, y+(height-fe.Height)/2+fe.Ascent, p.hint)
}

// drawReorderArrows points along the move axis, perpendicular to the slot line.
func drawReorderArrows(g paint.Painter, x, y float64, vertical bool) {
	point := func(along, across float64) (float64, float64) {
		if vertical {
			return x + across, y + along
		}
		return x + along, y + across
	}
	g.MoveTo(point(-6, 0))
	g.LineTo(point(6, 0))
	for _, end := range []float64{-1, 1} {
		g.MoveTo(point(end*2, -4))
		g.LineTo(point(end*6, 0))
		g.LineTo(point(end*2, 4))
	}
}

// The palette gesture is one undoable addition, including its visible
// selection. Undo restores the previous selection; redo selects the new widget
// so the property sheet and object inspector never point at a detached item.
type paletteAddCommand struct {
	add       *graph.AddCommand
	selection *graph.Selection
	item      graph.IItem
	parent    graph.IItem
	previous  []graph.IItem
}

func (cmd *paletteAddCommand) Text() string { return cmd.add.Text() }
func (cmd *paletteAddCommand) Redo() {
	cmd.add.Redo()
	cmd.relayoutParent()
	cmd.selection.Clear()
	cmd.selection.Add(cmd.item)
}
func (cmd *paletteAddCommand) Undo() {
	cmd.add.Undo()
	cmd.relayoutParent()
	cmd.selection.Clear()
	cmd.selection.AddMulti(cmd.previous)
}

func (cmd *paletteAddCommand) relayoutParent() {
	if parent, ok := cmd.parent.(*FakeWidget); ok && parent.layoutManaged {
		parent.Layout()
	}
}

// Explicit grid cells need an explicit append location: deriving it from the
// child count can collide with a sparse grid's existing row/column metadata.
func appendPaletteLayoutCell(parent *FakeWidget) layoutPlacement {
	p := layoutPlacement{Valid: true, RowSpan: 1, ColumnSpan: 1}
	count, nextRow, lastColumn := 0, 0, -1
	for _, it := range parent.Children() {
		child, ok := it.(*FakeWidget)
		if !ok {
			continue
		}
		cell := parent.childPlacement(child, count)
		count++
		endRow := cell.Row + cell.RowSpan
		if endRow > nextRow {
			nextRow, lastColumn = endRow, cell.Column+cell.ColumnSpan-1
		} else if endRow == nextRow && cell.Column+cell.ColumnSpan-1 > lastColumn {
			lastColumn = cell.Column + cell.ColumnSpan - 1
		}
	}
	switch LayoutKind(parent.WidgetFactoryName()) {
	case LayoutHorizontal:
		p.Column = count
	case LayoutVertical:
		p.Row = count
	case LayoutGrid:
		p.Row = nextRow
	case LayoutForm:
		p.Row = nextRow
		if nextRow > 0 && lastColumn == 0 {
			p.Row, p.Column = nextRow-1, 1
		}
	}
	return p
}
