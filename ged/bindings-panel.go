package ged

import (
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.BindingsPanel", gui.TypeOf(BindingsPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.BindingsPanel",
		Name: "信号/槽",
		Icon: "link",
		Desc: "显示本设计的事件绑定",
	})
}

// bindingsWarnColor marks a binding codegen cannot wire. The theme carries no
// warning colour, so this is the amber the rest of the tree warns in
// (ProblemsPanel's warning marker, gui.calcWarnColor).
var bindingsWarnColor = paint.Color{R: 230, G: 180, B: 60, A: 255}

// BindingRow is one line of the panel: one event of one widget.
//
// Bound is false for an event the design leaves unbound — the row exists so an
// event nobody wired is visible next to the ones that are. Known is false when
// widgetEvents has no entry for the (factory, event) pair, which is a binding
// the generator drops: recorded in the design, absent from the generated file.
type BindingRow struct {
	Widget  string
	Factory string
	Event   string
	Handler string
	Bound   bool
	Known   bool
}

// BindingsPanel lists every event binding in one design, the 信号/槽 overview.
//
// The property sheet's 事件 rows and the canvas's 绑定事件 submenu both show
// one widget at a time, so an event nobody bound — or two widgets sharing a
// handler name — is invisible until the design is generated. This panel reads
// the generator's own answers (walkFakeWidgets, handlerBindingsFor,
// AvailableEvents), so what it lists is what the generated file wires, not a
// second opinion maintained beside it.
//
// It is a tool view, never a placeable widget: no factoryMap entry, so
// isPlaceable keeps it out of the palette.
type BindingsPanel struct {
	gui.Widget

	scene *GedScene
	// Subscription id on scene's design-changed list, 0 when bound to nothing.
	sub  int
	rows []BindingRow
	// items[i] is the widget row i describes, so a press can hand the caller
	// the item rather than a name it would have to look up again.
	items       []graph.IItem
	selectedIdx int
	hoverIdx    int
	scrollY     float64
	rowHeight   float64
	cbSelect    func(graph.IItem)
}

// NewBindingsPanel creates an empty bindings panel.
func NewBindingsPanel() *BindingsPanel {
	p := new(BindingsPanel)
	p.Init(p)
	return p
}

func (this *BindingsPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 24
	this.selectedIdx = -1
	this.hoverIdx = -1
}

// contentTop is where the rows start: the title strip, and nothing else. Every
// row calculation goes through it so the offset is declared once — a second
// copy of it is how a panel selects the row above the one you clicked.
func (this *BindingsPanel) contentTop() float64 {
	return inspectorHeaderH
}

// SetScene binds the panel to a design and lists its bindings. It subscribes
// through AddDesignChangedCallback rather than SigDesignChanged: that slot
// holds one callback and may already be owned by the host.
//
// The previous subscription is dropped first. One panel is shared by every open
// document and design.go's bindBindingsPanelTo re-aims it on every tab change,
// so subscribing per call would leave the panel listening to every design the
// session has visited — once more for each time the user came back to it.
func (this *BindingsPanel) SetScene(scene *GedScene) {
	if this.scene != nil {
		this.scene.RemoveDesignChangedCallback(this.sub)
	}
	this.sub = 0
	this.scene = scene
	if scene != nil {
		this.sub = scene.AddDesignChangedCallback(this.Rebuild)
	}
	this.Rebuild()
}

// Rows returns the listed bindings, top to bottom. It is the panel's content
// in the only form a caller without a painter can read.
func (this *BindingsPanel) Rows() []BindingRow {
	out := make([]BindingRow, len(this.rows))
	copy(out, this.rows)
	return out
}

// SelectedRow returns the highlighted row index, -1 when none is.
func (this *BindingsPanel) SelectedRow() int {
	return this.selectedIdx
}

// SigSelect registers the callback fired when a row is pressed, carrying the
// widget that row describes.
func (this *BindingsPanel) SigSelect(fn func(graph.IItem)) {
	this.cbSelect = fn
}

// Rebuild walks the design and rebuilds the row list. It walks with
// walkFakeWidgets — the walk GenerateCode collects fields with — so the list
// is in document order and a widget nested in a layout container is listed
// like any other.
func (this *BindingsPanel) Rebuild() {
	this.rows = nil
	this.items = nil
	if this.scene != nil {
		walkFakeWidgets(this.scene.Children(), func(fake *FakeWidget) {
			this.appendWidget(fake)
		})
	}
	if this.selectedIdx >= len(this.rows) {
		this.selectedIdx = -1
	}
	this.clampScroll()
	this.Self().Update()
}

// appendWidget lists one widget: the bindings it contributes to the generated
// file first, in the order the generator emits them, then the events of its
// type that nothing is bound to.
func (this *BindingsPanel) appendWidget(fake *FakeWidget) {
	factory := fake.WidgetFactoryName()
	name := fake.WidgetName()
	if name == "" {
		name = strings.ToLower(shortFactoryName(factory))
	}
	bound := map[string]bool{}
	for _, b := range handlerBindingsFor(factory, fake.GetCode(), fake.EventHandlers()) {
		bound[b.event] = true
		this.appendRow(fake, BindingRow{
			Widget:  name,
			Factory: factory,
			Event:   b.event,
			Handler: b.handler,
			Bound:   true,
			Known:   b.known,
		})
	}
	for _, evt := range AvailableEvents(factory) {
		if bound[evt] {
			continue
		}
		this.appendRow(fake, BindingRow{
			Widget:  name,
			Factory: factory,
			Event:   evt,
			Bound:   false,
			Known:   true,
		})
	}
}

func (this *BindingsPanel) appendRow(item graph.IItem, row BindingRow) {
	this.rows = append(this.rows, row)
	this.items = append(this.items, item)
}

// shortFactoryName is the type without its package — what the columns have
// room for.
func shortFactoryName(factoryName string) string {
	if idx := strings.LastIndex(factoryName, "."); idx >= 0 {
		return factoryName[idx+1:]
	}
	return factoryName
}

// Draw renders the four columns: widget, type, event, handler.
func (this *BindingsPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

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
	g.DrawText1(8, headerH-5, "信号/槽")

	font := paint.NewFont(t.Font.Family(), 12, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 12, true, false)
	rh := this.rowHeight
	top := this.contentTop()
	startY := top - this.scrollY
	// Four columns: widget, type, event, handler.
	cols := [4]float64{8, w * 0.28, w * 0.48, w * 0.72}

	for i, row := range this.rows {
		rowY := startY + float64(i)*rh
		if rowY+rh < top || rowY > h {
			continue
		}

		if i == this.hoverIdx && i != this.selectedIdx {
			g.SetBrush1(paint.Color{230, 235, 245, 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}
		if i == this.selectedIdx {
			g.SetBrush1(paint.Color{51, 120, 215, 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		// An unbound event is a row about something that is not there, so it
		// is dimmed; a binding the generator cannot wire is the one row the
		// user must act on, so it is the one drawn in the warning colour.
		textColor := t.TextColor
		if !row.Bound {
			textColor = paint.Color{150, 150, 165, 255}
		}
		if !row.Known {
			textColor = bindingsWarnColor
		}
		if i == this.selectedIdx {
			textColor = paint.Color{255, 255, 255, 255}
		}

		textY := rowY + rh - 7
		g.SetBrush1(textColor)
		if i == this.selectedIdx {
			g.SetFont(boldFont)
		} else {
			g.SetFont(font)
		}
		g.DrawText1(cols[0], textY, row.Widget)
		g.DrawText1(cols[1], textY, shortFactoryName(row.Factory))
		g.DrawText1(cols[2], textY, row.Event)
		handler := row.Handler
		if !row.Bound {
			handler = "(未绑定)"
		}
		g.DrawText1(cols[3], textY, handler)

		if i != this.selectedIdx {
			g.SetPen1(paint.Color{230, 230, 235, 100}, 0.5)
			g.MoveTo(0, rowY+rh)
			g.LineTo(w, rowY+rh)
			g.Stroke()
		}
	}
}

// OnLeftDown selects the row under the cursor and reports its widget.
func (this *BindingsPanel) OnLeftDown(x, y float64) {
	top := this.contentTop()
	if y < top {
		return
	}
	idx, _ := rowAt(y, top, this.scrollY, this.rowHeight)
	if idx >= 0 && idx < len(this.rows) {
		this.selectedIdx = idx
		if this.cbSelect != nil {
			this.cbSelect(this.items[idx])
		}
	}
	this.Self().Update()
}

// OnMouseMove tracks the hover row.
func (this *BindingsPanel) OnMouseMove(x, y float64) {
	idx := -1
	if y >= this.contentTop() {
		if r, _ := rowAt(y, this.contentTop(), this.scrollY, this.rowHeight); r >= 0 && r < len(this.rows) {
			idx = r
		}
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover row.
func (this *BindingsPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the list.
func (this *BindingsPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3
	this.clampScroll()
	this.Self().Update()
}

// clampScroll keeps scrollY inside the range the current row count allows —
// a rebuild that leaves fewer rows than the offset skips would read as an
// empty panel with no scrollbar to drag back.
func (this *BindingsPanel) clampScroll() {
	maxScroll := float64(len(this.rows))*this.rowHeight - (this.Height() - this.contentTop())
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
