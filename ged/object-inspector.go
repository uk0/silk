package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"math"
	"strings"
)

func init() {
	core.RegisterFactory("ged.ObjectInspector", gui.TypeOf(ObjectInspector{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.ObjectInspector",
		Name: "对象",
		Icon: "tree-view",
		Desc: "对象层级检查器",
	})
}

// inspectorItem represents one row in the object inspector tree.
//
// It deliberately caches no lock state. Locks are toggled from the canvas
// context menu without rebuilding this panel, so a cached copy shows the badge
// the widget carried at the last rebuild rather than the one it carries now.
// Draw reads the flags live for the same reason the canvas does.
type inspectorItem struct {
	item     graph.IItem
	name     string
	typeName string
	depth    int
}

// ObjectInspector is a tool panel that shows the widget hierarchy as a tree,
// similar to Qt Creator's Object Inspector. Each row displays the widget name
// and class type. Clicking a row selects the corresponding scene item.
type ObjectInspector struct {
	gui.Widget
	scene       *GedScene
	view        *GedView
	items       []inspectorItem
	selectedIdx int
	hoverIdx    int
	scrollY     float64
	rowHeight   float64
	cbSelect    func(graph.IItem)

	filterBox  *gui.SearchBox
	filterText string

	// --- Drag Reorder / Re-parent ---
	dragging   bool
	dragIdx    int
	dropIdx    int
	dropInto   int // row to nest the drag into, -1 when the drop reorders instead
	dragStartY float64
}

func NewObjectInspector() *ObjectInspector {
	p := new(ObjectInspector)
	p.Init(p)
	return p
}

func (this *ObjectInspector) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 24
	this.selectedIdx = -1
	this.hoverIdx = -1
	this.dropInto = -1

	this.filterBox = gui.NewSearchBox()
	this.filterBox.SetParent(self)
	this.filterBox.SetPlaceholder("过滤控件…")
	this.filterBox.SigTextChanged(func(s string) {
		this.SetFilter(s)
	})
}

// inspectorHeaderH is the title strip, inspectorFilterH the filter box under
// it. Every row calculation goes through contentTop so the two strips are
// declared once — a second copy of the offset is how a panel ends up selecting
// the row above the one you clicked.
const (
	inspectorHeaderH = 22.0
	inspectorFilterH = 26.0
)

func (this *ObjectInspector) contentTop() float64 {
	return inspectorHeaderH + inspectorFilterH
}

func (this *ObjectInspector) Layout() {
	w, _ := this.Size()
	this.filterBox.SetBounds(4, inspectorHeaderH+2, w-8, inspectorFilterH-5)
}

// SetScene binds the inspector to a GedScene and rebuilds the item list.
//
// The canvas view comes with it: GraphView.SetScene records itself on the
// scene, and the tree needs it for the two operations that reach back out — a
// row press sets the canvas selection, and a drag re-parent reuses the command
// that selection generates. A scene nobody is showing yields a read-only tree.
func (this *ObjectInspector) SetScene(scene *GedScene) {
	this.scene = scene
	this.view = nil
	if scene != nil {
		this.view, _ = scene.View().(*GedView)
	}
	this.Rebuild()
}

// SetFilter narrows the tree to rows whose widget name or type contains text
// (case-insensitive), following the widget palette's convention: while a
// filter is active the matches are listed flat, without the hierarchy they
// normally hang in. Empty text restores the tree.
func (this *ObjectInspector) SetFilter(text string) {
	if text == this.filterText {
		return
	}
	this.filterText = text
	this.Rebuild()
}

// RowNames returns the name of every visible row, top to bottom. It is the
// panel's content in the only form a caller without a painter can read.
func (this *ObjectInspector) RowNames() []string {
	names := make([]string, 0, len(this.items))
	for _, it := range this.items {
		names = append(names, it.name)
	}
	return names
}

// SigSelect registers a callback that fires when the user selects an item.
func (this *ObjectInspector) SigSelect(fn func(graph.IItem)) {
	this.cbSelect = fn
}

// Rebuild walks the scene tree and populates the flat items list. The
// highlight follows the selected ITEM across the rebuild — a re-parent or a
// filter moves rows around, and a row index kept as-is would end up
// highlighting whichever widget inherited the slot.
func (this *ObjectInspector) Rebuild() {
	selected := this.SelectedItem()
	this.items = nil
	if this.scene == nil {
		this.selectedIdx = -1
		this.Self().Update()
		return
	}

	if filter := strings.ToLower(strings.TrimSpace(this.filterText)); filter != "" {
		// Filter mode, as the widget palette does it: the containers that
		// organise the list step aside and every match is listed flat.
		this.appendMatches(this.scene.Children(), filter)
	} else {
		// Root row: the form itself
		this.items = append(this.items, inspectorItem{
			item:     this.scene,
			name:     this.scene.FormTitle(),
			typeName: "GedScene",
			depth:    0,
		})

		// Walk the tree so widgets nested inside layout containers show
		// indented under their parent (depth grows per level), not as a flat
		// scene-level list.
		this.appendChildren(this.scene.Children(), 1)
	}

	this.selectedIdx = this.rowOfItem(selected)
	this.clampScroll()
	this.Self().Update()
}

// appendChildren adds an inspector row for each item at the given depth,
// then recurses into its children at depth+1 so the inspector mirrors the
// scene's container nesting.
func (this *ObjectInspector) appendChildren(items []graph.IItem, depth int) {
	for _, child := range items {
		this.items = append(this.items, newInspectorItem(child, depth))
		if child.HasChildren() {
			this.appendChildren(child.Children(), depth+1)
		}
	}
}

// appendMatches walks the same tree but emits only the rows that match, all at
// depth 0: with the non-matching ancestors gone the indentation would describe
// a hierarchy that is no longer on screen.
func (this *ObjectInspector) appendMatches(items []graph.IItem, filter string) {
	for _, child := range items {
		row := newInspectorItem(child, 0)
		if inspectorRowMatches(row.name, row.typeName, filter) {
			this.items = append(this.items, row)
		}
		if child.HasChildren() {
			this.appendMatches(child.Children(), filter)
		}
	}
}

// newInspectorItem reads one row's display state off the item: the widget's
// own name (falling back to the lower-cased type when it has none), the type
// as secondary text, and the two lock flags the canvas badges.
func newInspectorItem(child graph.IItem, depth int) inspectorItem {
	name := ""
	typeName := ""
	if fake, ok := child.(*FakeWidget); ok {
		factoryName := fake.WidgetFactoryName()
		if idx := strings.LastIndex(factoryName, "."); idx >= 0 {
			typeName = factoryName[idx+1:]
		} else {
			typeName = factoryName
		}
		name = fake.WidgetName()
		if name == "" {
			name = strings.ToLower(typeName)
		}
	}
	return inspectorItem{
		item:     child,
		name:     name,
		typeName: typeName,
		depth:    depth,
	}
}

// inspectorRowMatches reports whether a row survives the filter. filter is
// already lower-cased and trimmed; matching runs against the widget name and
// the type, the same two strings the palette matches on.
func inspectorRowMatches(name, typeName, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), filter) ||
		strings.Contains(strings.ToLower(typeName), filter)
}

// SelectedItem returns the scene item the highlighted row stands for, nil when
// no row is highlighted.
func (this *ObjectInspector) SelectedItem() graph.IItem {
	if this.selectedIdx < 0 || this.selectedIdx >= len(this.items) {
		return nil
	}
	return this.items[this.selectedIdx].item
}

// rowOfItem returns the row listing item, or -1 when it is not on screen.
func (this *ObjectInspector) rowOfItem(item graph.IItem) int {
	if item == nil {
		return -1
	}
	for i, it := range this.items {
		if it.item == item {
			return i
		}
	}
	return -1
}

// Draw renders the object inspector tree.
func (this *ObjectInspector) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header
	headerH := inspectorHeaderH
	g.SetBrush1(paint.Color{235, 238, 245, 255})
	g.Rectangle(0, 0, w, headerH)
	g.Fill()
	g.SetPen1(paint.Color{200, 200, 210, 255}, 1)
	g.MoveTo(0, headerH)
	g.LineTo(w, headerH)
	g.Stroke()

	g.SetFont(paint.NewFont(t.Font.Family(), 12, true, false))
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, headerH-5, "Object Inspector")

	// Rows
	font := paint.NewFont(t.Font.Family(), 12, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 12, true, false)
	rh := this.rowHeight
	top := this.contentTop()
	startY := top - this.scrollY

	for i, item := range this.items {
		rowY := startY + float64(i)*rh
		if rowY+rh < top || rowY > h {
			continue
		}

		// Dim the dragged item slightly during drag
		isDragged := this.dragging && i == this.dragIdx

		// Hover highlight
		if i == this.hoverIdx && i != this.selectedIdx && !isDragged {
			g.SetBrush1(paint.Color{230, 235, 245, 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		// Selection highlight
		if i == this.selectedIdx && !isDragged {
			g.SetBrush1(paint.Color{51, 120, 215, 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		// Dimmed background for dragged item
		if isDragged {
			g.SetBrush1(paint.Color{200, 210, 230, 80})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		indent := 12.0 + float64(item.depth)*20.0

		// Tree connector lines
		if item.depth > 0 {
			lineX := 12.0 + float64(item.depth-1)*20.0 + 10.0
			g.SetPen1(paint.Color{180, 180, 190, 255}, 1)
			midY := rowY + rh*0.5
			// Vertical line from parent
			g.MoveTo(lineX, rowY)
			g.LineTo(lineX, midY)
			g.Stroke()
			// Horizontal connector
			g.MoveTo(lineX, midY)
			g.LineTo(lineX+8, midY)
			g.Stroke()
		}

		// Name text
		textY := rowY + rh - 7
		if isDragged {
			// Dimmed text for dragged item
			g.SetBrush1(paint.Color{150, 150, 165, 160})
			g.SetFont(font)
		} else if i == this.selectedIdx {
			g.SetBrush1(paint.Color{255, 255, 255, 255})
			g.SetFont(boldFont)
		} else {
			g.SetBrush1(t.TextColor)
			g.SetFont(font)
		}
		g.DrawText1(indent, textY, item.name)

		// Type text (right-aligned or after name)
		if item.typeName != "" {
			typeText := "(" + item.typeName + ")"
			if isDragged {
				g.SetBrush1(paint.Color{130, 130, 145, 100})
			} else if i == this.selectedIdx {
				g.SetBrush1(paint.Color{200, 220, 255, 255})
			} else {
				g.SetBrush1(paint.Color{130, 130, 145, 255})
			}
			nameExt := font.TextExtents(item.name)
			typeX := indent + nameExt.Width + 8
			g.DrawText1(typeX, textY, typeText)
		}

		// Lock badge, in the canvas's own glyph: a widget that refuses to move
		// must say so wherever it is listed, not only where it is drawn.
		if item.item != nil && (item.item.IsLockPos() || item.item.IsLockSize()) {
			sz := rh - 10
			drawLockGlyph(g, w-sz-8, rowY+5, sz, 1)
		}

		// Bottom separator
		if i != this.selectedIdx {
			g.SetPen1(paint.Color{230, 230, 235, 100}, 0.5)
			g.MoveTo(0, rowY+rh)
			g.LineTo(w, rowY+rh)
			g.Stroke()
		}
	}

	// Drop feedback: a frame around the row the drag would nest into, or the
	// insertion line where it would land between siblings.
	if this.dragging && this.dropInto >= 0 {
		intoY := startY + float64(this.dropInto)*rh
		g.SetPen1(paint.Color{51, 120, 215, 255}, 2)
		g.Rectangle(1, intoY+1, w-2, rh-2)
		g.Stroke()
	} else if this.dragging && this.dropIdx > 0 {
		dropY := startY + float64(this.dropIdx)*rh
		g.SetPen1(paint.Color{51, 120, 215, 255}, 2)
		g.MoveTo(12, dropY)
		g.LineTo(w-4, dropY)
		g.Stroke()
		// Small triangle indicator on the left
		g.SetBrush1(paint.Color{51, 120, 215, 255})
		g.MoveTo(4, dropY-4)
		g.LineTo(12, dropY)
		g.LineTo(4, dropY+4)
		g.Fill()
	}
}

// rowAt maps a panel y coordinate to a row index and how far down that row the
// point sits (0 at its top edge, approaching 1 at its bottom). Pure, so the
// band arithmetic below can be tested without a live panel.
func rowAt(y, top, scrollY, rowHeight float64) (idx int, frac float64) {
	if rowHeight <= 0 {
		return -1, 0
	}
	pos := (y - top + scrollY) / rowHeight
	idx = int(math.Floor(pos))
	return idx, pos - float64(idx)
}

// dropIsInto reports whether a drop at this depth into a row nests into it
// rather than settling between siblings. The middle half of a row is the
// "into" target and the two quarters at its edges are the gaps around it —
// the same split every tree with drag re-parenting uses, and the reason a
// re-parent gesture and a reorder gesture can share one drag.
func dropIsInto(frac float64) bool {
	return frac >= 0.25 && frac < 0.75
}

// OnLeftDown handles click selection and begins drag tracking.
func (this *ObjectInspector) OnLeftDown(x, y float64) {
	top := this.contentTop()
	if y < top {
		return
	}
	idx, _ := rowAt(y, top, this.scrollY, this.rowHeight)
	if idx >= 0 && idx < len(this.items) {
		this.selectedIdx = idx
		this.selectOnCanvas(this.items[idx].item)
		if this.cbSelect != nil {
			this.cbSelect(this.items[idx].item)
		}
		// Start a potential drag on any row but the form root. Filtered rows
		// are listed flat, so neither the sibling arithmetic in reorderItems
		// nor a nesting target means anything while a filter is on.
		if idx > 0 && this.filterText == "" {
			this.dragIdx = idx
			this.dropIdx = idx
			this.dropInto = -1
			this.dragStartY = y
		}
	}
	this.Self().Update()
}

// OnMouseMove handles hover highlighting and drag indicator.
func (this *ObjectInspector) OnMouseMove(x, y float64) {
	top := this.contentTop()

	// Check if we should start dragging (threshold = 5px)
	if this.dragIdx > 0 && !this.dragging {
		dy := y - this.dragStartY
		if dy > 5 || dy < -5 {
			this.dragging = true
		}
	}

	// Update drop target while dragging
	if this.dragging {
		into, before := this.dropTargetAt(y)
		if into != this.dropInto || before != this.dropIdx {
			this.dropInto, this.dropIdx = into, before
			this.Self().Update()
		}
		return
	}

	if y < top {
		if this.hoverIdx != -1 {
			this.hoverIdx = -1
			this.Self().Update()
		}
		return
	}
	idx, _ := rowAt(y, top, this.scrollY, this.rowHeight)
	if idx < 0 || idx >= len(this.items) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// dropTargetAt resolves a drag position into either a row to nest into
// (into >= 0) or a sibling slot to insert before (into == -1). A row that
// cannot take the dragged item as a child falls back to the slot above it, so
// the gesture always has somewhere to land.
func (this *ObjectInspector) dropTargetAt(y float64) (into, before int) {
	idx, frac := rowAt(y, this.contentTop(), this.scrollY, this.rowHeight)
	if idx >= 0 && idx < len(this.items) && dropIsInto(frac) &&
		this.canNestInto(this.dragIdx, idx) {
		return idx, -1
	}
	// Reordering only ever reaches a direct child of the scene: reorderItems
	// rejects anything else. Offering the insertion line for a nested row would
	// promise a move that is silently dropped, so refuse the gesture here where
	// the promise is drawn rather than there where it is broken.
	if !this.canReorder(this.dragIdx) {
		return -1, -1
	}
	if frac >= 0.5 {
		idx++
	}
	if idx < 1 {
		idx = 1 // cannot drop above root (index 0)
	}
	if idx > len(this.items) {
		idx = len(this.items)
	}
	return -1, idx
}

// canReorder reports whether the row at idx may be moved among its siblings —
// true only for a direct child of the scene, which is what reorderItems can
// actually express.
func (this *ObjectInspector) canReorder(idx int) bool {
	if this.scene == nil || idx < 0 || idx >= len(this.items) {
		return false
	}
	it := this.items[idx].item
	return it != nil && it.Parent() == graph.IItem(this.scene)
}

// canNestInto reports whether the row at target can adopt the row at drag: the
// form root always can, a container widget can, and nothing may be dropped
// into itself, into its current parent (a no-op) or into its own subtree.
func (this *ObjectInspector) canNestInto(drag, target int) bool {
	if drag < 0 || drag >= len(this.items) || target < 0 || target >= len(this.items) {
		return false
	}
	item, parent := this.items[drag].item, this.items[target].item
	if item == nil || parent == nil || item == parent {
		return false
	}
	if item.Parent() == parent.Self() {
		return false
	}
	for p := parent; p != nil; p = p.Parent() {
		if p == item {
			return false // a cycle; SetParentAt has no guard against one
		}
	}
	return target == 0 || isContainerItem(parent)
}

// OnLeftUp completes the drag: a drop onto a row re-parents, a drop between
// rows reorders siblings.
func (this *ObjectInspector) OnLeftUp(x, y float64) {
	if this.dragging && this.scene != nil {
		if this.dropInto >= 0 {
			this.reparentInto(this.dropInto)
		} else if this.dragIdx != this.dropIdx {
			this.reorderItems(this.dragIdx, this.dropIdx)
		}
	}
	this.dragging = false
	this.dragIdx = 0
	this.dropIdx = 0
	this.dropInto = -1
	this.dragStartY = 0
	this.Self().Update()
}

// reparentInto nests the dragged widget into the row's item, routed through
// the canvas's own re-parent so one undo step covers a tree drag exactly as it
// covers a canvas drag: GedView.reparentSelection builds the ReparentCommand
// (parent + sibling slot + position) and pushes it on the scene's undo stack.
// The pointerless drop's rescue rides in that same step (parkStep).
//
// It works on the view's selection, which the row press has just set to the
// dragged widget. A host that bound only a scene has no selection to move, so
// the drag stays a no-op there rather than mutating the tree behind the undo
// stack's back.
func (this *ObjectInspector) reparentInto(row int) {
	if this.view == nil || row < 0 || row >= len(this.items) {
		return
	}
	target := this.items[row].item
	if target == nil {
		return
	}
	if target == graph.IItem(this.scene) {
		target = nil // the form root, spelled the way a canvas drop spells it
	}
	moved := this.view.Selection().ItemList()
	this.view.reparentSelection(target, &parkStep{items: moved, parent: target})
	this.Rebuild()
}

// parkStep is the rescue half of a tree drop, grouped with the re-parent into
// one undo step (GedView.reparentSelection). It is the one command here that
// cannot be built in the pre-applied state the others are: which widgets ended
// up outside their new parent — and how far out — is only settled once the
// re-parent itself has been applied. So it decides on the first Redo and keeps
// the move it made, which is what Undo takes back and what a later Redo
// replays.
//
// Splitting the two into separate undo steps would be worse than not recording
// the park at all: the state between them has the widget owned by a container
// it does not overlap, which is precisely the invisible widget the park exists
// to prevent.
type parkStep struct {
	items  []graph.IItem
	parent graph.IItem
	move   *graph.MoveCommand
}

func (this *parkStep) Redo() {
	if this.move == nil {
		this.move = parkInsideParent(this.items, this.parent)
	}
	if this.move != nil {
		this.move.Redo()
	}
}

func (this *parkStep) Undo() {
	if this.move != nil {
		this.move.Undo()
	}
}

// Text is not what the stack shows — the compound this rides in carries the
// re-parent's own text — but ICommand asks for it.
func (this *parkStep) Text() string {
	return "Park inside parent"
}

// parkInsideParent returns the move that brings re-parented widgets into their
// new parent's box, or nil when none of them fell outside it.
//
// A tree drop has no pointer — the widget keeps the scene rect it had somewhere
// else entirely. A canvas drop has one, but the container is chosen from the
// cursor mid-drag and the grid snap moves the widget afterwards, so a widget
// released near a container's edge can be snapped back out of it. Either way,
// because FakeWidget has no local coordinates, Item.DrawAll clips a child to
// the intersection with its parent and returns early when that is empty. The
// widget stays in the document, answers Parent() correctly, and is invisible
// on the canvas with no way to drag it back.
//
// Only items that have fallen outside are moved, and only far enough to sit
// at the parent's top-left inset — a tree drop states intent about ownership,
// not about position, so it must disturb the layout as little as it can.
//
// The park takes each widget's subtree with it (AddMoveSubtree, the walk every
// other move here uses): a container's children carry their own scene
// coordinates, so parking the container alone lands it inside its new owner
// and leaves its contents outside it — the same disappearance, one level down.
//
// It is a command rather than a bare SetPos because the re-parent's own record
// neither replays nor restores it. That record holds the position from before
// the park, so an unrecorded park came back on Ctrl+Y with the widget outside
// its new parent and invisible again; and once the park carries a subtree,
// Ctrl+Z put the container back and left its children where the park had
// dropped them, which is the same widget lost from the other side. parkStep
// carries the result into the re-parent's own undo step.
func parkInsideParent(items []graph.IItem, parent graph.IItem) *graph.MoveCommand {
	if parent == nil {
		return nil // the form root clips nothing
	}
	px, py := parent.X(), parent.Y()
	pw, ph := parent.Width(), parent.Height()
	cmd := graph.NewMoveCommand()
	for _, it := range items {
		if it == nil || it.Parent() != parent || it.IsLockPos() {
			continue
		}
		x, y, w, h := it.X(), it.Y(), it.Width(), it.Height()
		if x < px+pw && x+w > px && y < py+ph && y+h > py {
			continue // already overlaps; leave the layout alone
		}
		graph.AddMoveSubtree(cmd, it, px+parentInsetMm-x, py+parentInsetMm-y)
	}
	if cmd.Count() == 0 {
		return nil // nothing had fallen outside; the step has nothing to replay
	}
	return cmd
}

// parentInsetMm keeps a parked widget off its container's own border so the
// user can see it landed inside rather than on the edge.
const parentInsetMm = 2.0

// selectOnCanvas mirrors a row press onto the canvas selection, so the widget
// picks up its handles and every selection-driven panel (property sheet,
// code panel, status bar) follows the tree.
func (this *ObjectInspector) selectOnCanvas(item graph.IItem) {
	if this.view == nil || item == nil || item == graph.IItem(this.scene) {
		return
	}
	sel := this.view.Selection()
	sel.Clear()
	sel.Add(item)
	this.view.Update()
}

// SyncSelection points the highlight at the canvas selection. A multi-item
// selection has no single row to stand for it, so the highlight clears.
func (this *ObjectInspector) SyncSelection(items []graph.IItem) {
	if len(items) == 1 {
		this.SelectItem(items[0])
		return
	}
	this.selectedIdx = -1
	this.Self().Update()
}

// reorderItems moves the item at fromIdx to the position at toIdx
// by detaching all scene children and re-attaching in the new order.
func (this *ObjectInspector) reorderItems(fromIdx, toIdx int) {
	if this.scene == nil {
		return
	}
	// fromIdx/toIdx are inspector-row indices into the FLATTENED items list
	// (root + children + grandchildren), so bound-check before dereferencing.
	if fromIdx < 0 || fromIdx >= len(this.items) {
		return
	}
	// A flattened row index is NOT a sibling index. Derive the real sibling
	// position from the dragged item itself, and reject anything that is not a
	// direct child of the scene (only top-level rows are draggable, but the
	// item may sit after nested rows once containers exist).
	it := this.items[fromIdx].item
	if it == nil || it.Parent() != graph.IItem(this.scene) {
		return
	}
	children := this.scene.Children()
	childCount := len(children)
	if childCount < 2 {
		return
	}
	fromChild := it.IndexInParent()
	if fromChild < 0 || fromChild >= childCount {
		return
	}
	// Map the drop row to a sibling slot by counting the direct children whose
	// flattened row precedes toIdx. For a non-nested tree this reduces to the
	// old toIdx-1; with nesting it correctly skips grandchildren rows.
	toChild := 0
	for i := 0; i < len(this.items) && i < toIdx; i++ {
		if this.items[i].depth == 1 {
			toChild++
		}
	}
	if toChild > childCount {
		toChild = childCount
	}
	if fromChild == toChild {
		return
	}

	// Build new order
	ordered := make([]graph.IItem, len(children))
	copy(ordered, children)

	// Remove item from old position
	item := ordered[fromChild]
	ordered = append(ordered[:fromChild], ordered[fromChild+1:]...)

	// Insert at new position
	insertAt := toChild
	if insertAt > fromChild {
		insertAt-- // adjust since we removed one
	}
	if insertAt > len(ordered) {
		insertAt = len(ordered)
	}
	newOrdered := make([]graph.IItem, 0, len(ordered)+1)
	newOrdered = append(newOrdered, ordered[:insertAt]...)
	newOrdered = append(newOrdered, item)
	newOrdered = append(newOrdered, ordered[insertAt:]...)

	// Detach all children from scene, then re-attach in new order
	for _, ch := range children {
		ch.SetParent(nil)
	}
	for _, ch := range newOrdered {
		ch.SetParent(this.scene)
	}

	// Rebuild inspector list and update selection
	this.Rebuild()
}

// OnMouseLeave resets hover and drag state.
func (this *ObjectInspector) OnMouseLeave() {
	changed := false
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		changed = true
	}
	if this.dragging {
		this.dragging = false
		this.dragIdx = 0
		this.dropIdx = 0
		this.dropInto = -1
		changed = true
	}
	if changed {
		this.Self().Update()
	}
}

// OnMouseWheel handles scrolling.
func (this *ObjectInspector) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3
	this.clampScroll()
	this.Self().Update()
}

// clampScroll keeps scrollY inside the range the current row count allows.
// Rebuild needs it as much as the wheel does: scroll down, then type a filter
// that leaves three rows, and the offset that was valid a moment ago scrolls
// every remaining row off the top — the panel reads as empty and the user has
// no scrollbar to drag back.
func (this *ObjectInspector) clampScroll() {
	maxScroll := float64(len(this.items))*this.rowHeight - (this.Height() - this.contentTop())
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
}

// SelectItem highlights the row standing for item. An item with no row on
// screen — filtered out, or belonging to another document — clears the
// highlight rather than leaving it on an unrelated row.
func (this *ObjectInspector) SelectItem(item graph.IItem) {
	this.selectedIdx = this.rowOfItem(item)
	this.Self().Update()
}
