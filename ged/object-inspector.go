package ged

import (
	"silk/core"
	"silk/graph"
	"silk/gui"
	"silk/paint"
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
	items       []inspectorItem
	selectedIdx int
	hoverIdx    int
	scrollY     float64
	rowHeight   float64
	cbSelect    func(graph.IItem)

	// --- Drag Reorder ---
	dragging   bool
	dragIdx    int
	dropIdx    int
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
}

// SetScene binds the inspector to a GedScene and rebuilds the item list.
func (this *ObjectInspector) SetScene(scene *GedScene) {
	this.scene = scene
	this.Rebuild()
}

// SigSelect registers a callback that fires when the user selects an item.
func (this *ObjectInspector) SigSelect(fn func(graph.IItem)) {
	this.cbSelect = fn
}

// Rebuild walks the scene tree and populates the flat items list.
func (this *ObjectInspector) Rebuild() {
	this.items = nil
	if this.scene == nil {
		return
	}

	// Root row: the form itself
	this.items = append(this.items, inspectorItem{
		item:     this.scene,
		name:     this.scene.FormTitle(),
		typeName: "GedScene",
		depth:    0,
	})

	// Children are flat (all direct children of the scene)
	for _, child := range this.scene.Children() {
		name := ""
		typeName := ""
		if fake, ok := child.(*FakeWidget); ok {
			name = fake.WidgetName()
			factoryName := fake.WidgetFactoryName()
			if idx := strings.LastIndex(factoryName, "."); idx >= 0 {
				typeName = factoryName[idx+1:]
			} else {
				typeName = factoryName
			}
			if name == "" {
				name = strings.ToLower(typeName)
			}
		}
		this.items = append(this.items, inspectorItem{
			item:     child,
			name:     name,
			typeName: typeName,
			depth:    1,
		})
	}

	this.Self().Update()
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
	headerH := 22.0
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
	startY := headerH - this.scrollY

	for i, item := range this.items {
		rowY := startY + float64(i)*rh
		if rowY+rh < headerH || rowY > h {
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

		// Bottom separator
		if i != this.selectedIdx {
			g.SetPen1(paint.Color{230, 230, 235, 100}, 0.5)
			g.MoveTo(0, rowY+rh)
			g.LineTo(w, rowY+rh)
			g.Stroke()
		}
	}

	// Draw blue insertion line at drop position during drag
	if this.dragging && this.dropIdx > 0 {
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

// OnLeftDown handles click selection and begins drag tracking.
func (this *ObjectInspector) OnLeftDown(x, y float64) {
	headerH := 22.0
	if y < headerH {
		return
	}
	idx := int(math.Floor((y - headerH + this.scrollY) / this.rowHeight))
	if idx >= 0 && idx < len(this.items) {
		this.selectedIdx = idx
		if this.cbSelect != nil {
			this.cbSelect(this.items[idx].item)
		}
		// Start potential drag (only for depth-1 items, not the root)
		if idx > 0 && this.items[idx].depth == 1 {
			this.dragIdx = idx
			this.dropIdx = idx
			this.dragStartY = y
		}
	}
	this.Self().Update()
}

// OnMouseMove handles hover highlighting and drag indicator.
func (this *ObjectInspector) OnMouseMove(x, y float64) {
	headerH := 22.0

	// Check if we should start dragging (threshold = 5px)
	if this.dragIdx > 0 && !this.dragging {
		dy := y - this.dragStartY
		if dy > 5 || dy < -5 {
			this.dragging = true
		}
	}

	// Update drop target while dragging
	if this.dragging {
		idx := int(math.Floor((y - headerH + this.scrollY) / this.rowHeight))
		if idx < 1 {
			idx = 1 // cannot drop above root (index 0)
		}
		if idx >= len(this.items) {
			idx = len(this.items)
		}
		if idx != this.dropIdx {
			this.dropIdx = idx
			this.Self().Update()
		}
		return
	}

	if y < headerH {
		if this.hoverIdx != -1 {
			this.hoverIdx = -1
			this.Self().Update()
		}
		return
	}
	idx := int(math.Floor((y - headerH + this.scrollY) / this.rowHeight))
	if idx < 0 || idx >= len(this.items) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnLeftUp completes drag-reorder if active.
func (this *ObjectInspector) OnLeftUp(x, y float64) {
	if this.dragging && this.scene != nil && this.dragIdx != this.dropIdx {
		this.reorderItems(this.dragIdx, this.dropIdx)
	}
	this.dragging = false
	this.dragIdx = 0
	this.dropIdx = 0
	this.dragStartY = 0
	this.Self().Update()
}

// reorderItems moves the item at fromIdx to the position at toIdx
// by detaching all scene children and re-attaching in the new order.
func (this *ObjectInspector) reorderItems(fromIdx, toIdx int) {
	if this.scene == nil {
		return
	}
	// Only reorder depth-1 items (scene children)
	// items[0] is the root (GedScene), items[1..] are children
	childCount := len(this.items) - 1
	if childCount < 2 {
		return
	}
	// Convert inspector indices to child indices (0-based within children)
	fromChild := fromIdx - 1
	toChild := toIdx - 1
	if fromChild < 0 || fromChild >= childCount {
		return
	}
	if toChild < 0 {
		toChild = 0
	}
	if toChild > childCount {
		toChild = childCount
	}
	if fromChild == toChild {
		return
	}

	// Get current children
	children := this.scene.Children()
	if len(children) < 2 {
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
		changed = true
	}
	if changed {
		this.Self().Update()
	}
}

// OnMouseWheel handles scrolling.
func (this *ObjectInspector) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3
	maxScroll := float64(len(this.items))*this.rowHeight - (this.Height() - 22)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// SelectItem programmatically selects the given scene item in the inspector.
func (this *ObjectInspector) SelectItem(item graph.IItem) {
	for i, it := range this.items {
		if it.item == item {
			this.selectedIdx = i
			this.Self().Update()
			return
		}
	}
}
