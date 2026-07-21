package ged

import (
	"errors"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/prop"
	"strings"
)

func setDefaultContent(w gui.IWidget, factoryName string) {
	short := factoryName
	if idx := strings.LastIndex(factoryName, "."); idx >= 0 {
		short = factoryName[idx+1:]
	}
	switch strings.ToLower(short) {
	case "button":
		if b, ok := w.(*gui.Button); ok {
			b.SetText("Button")
		}
	case "label":
		if l, ok := w.(*gui.Label); ok {
			l.SetText("Label")
		}
	case "edit":
		if e, ok := w.(*gui.Edit); ok {
			e.SetText("Text Input")
		}
	case "checkbox":
		if c, ok := w.(*gui.CheckBox); ok {
			c.SetText("CheckBox")
		}
	case "radiobutton":
		if r, ok := w.(*gui.RadioButton); ok {
			r.SetText("Radio")
		}
	case "progressbar":
		if p, ok := w.(*gui.ProgressBar); ok {
			p.SetValue(0.5)
			p.SetShowText(true)
		}
	case "slider":
		if s, ok := w.(*gui.Slider); ok {
			s.SetValue(50)
		}
	case "groupbox":
		if g, ok := w.(*gui.GroupBox); ok {
			g.SetTitle("Group")
		}
	case "combobox":
		if c, ok := w.(*gui.ComboBox); ok {
			c.Append(gui.ListItem{Text: "Item 1"})
			c.Append(gui.ListItem{Text: "Item 2"})
		}
	case "linechart":
		if c, ok := w.(*gui.LineChart); ok {
			c.SetTitle("Line Chart")
			c.AddSeries("Series A", paint.Color{R: 65, G: 131, B: 215, A: 255},
				[]float64{10, 25, 18, 35, 28, 42, 38})
		}
	case "barchart":
		if c, ok := w.(*gui.BarChart); ok {
			c.SetTitle("Bar Chart")
			c.AddBar("A", 30, paint.Color{R: 65, G: 131, B: 215, A: 255})
			c.AddBar("B", 50, paint.Color{R: 228, G: 77, B: 66, A: 255})
			c.AddBar("C", 40, paint.Color{R: 90, G: 185, B: 102, A: 255})
		}
	case "piechart":
		if c, ok := w.(*gui.PieChart); ok {
			c.SetTitle("Pie Chart")
			c.AddSlice("A", 40, paint.Color{R: 65, G: 131, B: 215, A: 255})
			c.AddSlice("B", 30, paint.Color{R: 228, G: 77, B: 66, A: 255})
			c.AddSlice("C", 30, paint.Color{R: 90, G: 185, B: 102, A: 255})
		}
	case "gauge":
		if g, ok := w.(*gui.Gauge); ok {
			g.SetTitle("Gauge")
			g.SetValue(65)
			g.SetUnit("%")
			g.AddZone(0, 40, paint.Color{R: 90, G: 185, B: 102, A: 255})
			g.AddZone(40, 75, paint.Color{R: 249, G: 168, B: 37, A: 255})
			g.AddZone(75, 100, paint.Color{R: 228, G: 77, B: 66, A: 255})
		}
	case "scatterplot":
		if c, ok := w.(*gui.ScatterPlot); ok {
			c.SetTitle("Scatter")
			c.AddSeries("Data", paint.Color{R: 65, G: 131, B: 215, A: 255},
				[]gui.ScatterPoint{{X: 1, Y: 2}, {X: 3, Y: 5}, {X: 5, Y: 4}, {X: 7, Y: 8}})
		}
	// ── New widgets ──
	case "toggleswitch":
		if t, ok := w.(*gui.ToggleSwitch); ok {
			t.SetText("Switch")
			t.SetChecked(true)
		}
	case "searchbox":
		if s, ok := w.(*gui.SearchBox); ok {
			s.SetPlaceholder("Search...")
		}
	case "numberinput":
		if n, ok := w.(*gui.NumberInput); ok {
			n.SetValue(42)
			n.SetRange(0, 100)
			n.SetStep(1)
		}
	case "datepicker":
		if d, ok := w.(*gui.DatePicker); ok {
			d.SetDate(2026, 4, 12)
		}
	case "colorpicker":
		if c, ok := w.(*gui.ColorPicker); ok {
			c.SetColor(paint.Color{R: 66, G: 133, B: 244, A: 255})
		}
	case "rating":
		if r, ok := w.(*gui.Rating); ok {
			r.SetValue(3)
		}
	case "dropdownbutton":
		if d, ok := w.(*gui.DropdownButton); ok {
			d.SetText("Select")
			d.AddItem("Option 1", nil, nil)
			d.AddItem("Option 2", nil, nil)
			d.AddItem("Option 3", nil, nil)
		}
	case "switchgroup":
		if s, ok := w.(*gui.SwitchGroup); ok {
			s.SetItems([]string{"Tab 1", "Tab 2", "Tab 3"})
		}
	case "imageview":
		// Empty by default — shows "No Image" placeholder
	case "tag":
		if t, ok := w.(*gui.Tag); ok {
			t.SetText("Tag")
			t.SetCloseable(true)
		}
	case "badge":
		if b, ok := w.(*gui.Badge); ok {
			b.SetCount(5)
		}
	case "avatar":
		if a, ok := w.(*gui.Avatar); ok {
			a.SetText("User")
		}
	case "breadcrumb":
		if b, ok := w.(*gui.Breadcrumb); ok {
			b.AddItem("Home", nil)
			b.AddItem("Products", nil)
			b.AddItem("Detail", nil)
		}
	case "link":
		if l, ok := w.(*gui.Link); ok {
			l.SetText("Click here")
			l.SetURL("https://example.com")
		}
	case "labelseparator":
		if l, ok := w.(*gui.LabelSeparator); ok {
			l.SetText("OR")
		}
	case "placeholder":
		if p, ok := w.(*gui.Placeholder); ok {
			p.SetTitle("No Data")
			p.SetSubtitle("Nothing to show yet")
		}
	case "timeline":
		if t, ok := w.(*gui.Timeline); ok {
			t.AddItem("Step 1", "Done", 2)
			t.AddItem("Step 2", "Active", 1)
			t.AddItem("Step 3", "Pending", 0)
		}
	case "notificationpanel":
		if n, ok := w.(*gui.NotificationPanel); ok {
			n.AddNotification(gui.NotificationItem{Title: "Info", Message: "Welcome!", Level: gui.NotifyInfo})
			n.AddNotification(gui.NotificationItem{Title: "Success", Message: "Done", Level: gui.NotifySuccess})
		}
	case "card":
		if c, ok := w.(*gui.Card); ok {
			c.SetTitle("Card Title")
		}
	case "accordion":
		// Empty by default — user adds sections
	case "scrollarea":
		// Empty by default
	case "table":
		// Empty by default — user configures columns
	case "listwidget":
		if l, ok := w.(*gui.ListWidget); ok {
			l.Append(gui.ListItem{Text: "Item 1"})
			l.Append(gui.ListItem{Text: "Item 2"})
			l.Append(gui.ListItem{Text: "Item 3"})
		}
	case "treeview":
		// Empty by default — user configures tree nodes
	case "tabwidget":
		// Empty by default — user adds tab pages
	case "form":
		// Empty by default — container for child widgets
	case "dialog":
		// Empty by default — container/window widget
	}
}

type FakeWidget struct {
	graph.Item
	widget        gui.IWidget
	factoryName   string
	name          string
	eventHandlers map[string]string
	code          string // Go source code for event handlers

	// Pixmap caching fields – avoids recreating the offscreen surface every frame
	cachedPixmap paint.Pixmap
	pixmapDirty  bool
	cachedW      int
	cachedH      int
}

func NewFakeWidget() *FakeWidget {
	p := new(FakeWidget)
	p.Init(p)
	return p
}

func NewFakeWidgetFromFactory(name string) (*FakeWidget, error) {
	factory := core.FindFactory(name)
	if factory == nil {
		return nil, errors.New("factory not found")
	}

	// dnd.SetAction(gui.DndCopy)

	widget, ok := factory.New().(gui.IWidget)
	if !ok {
		return nil, errors.New("object is not a widget")
	}

	// Set default content for the widget
	setDefaultContent(widget, name)

	p := NewFakeWidget()
	p.SetWidget(widget)
	return p, nil
}

func (this *FakeWidget) Init(self graph.IItem) {
	this.Item.Init(self)
}

func (this *FakeWidget) DrawSelf(g paint.Painter) {

	if this.widget == nil {
		this.Item.DrawSelf(g)
		return
	}

	g.Save()
	defer g.Restore()

	// Draw subtle blue border in scene (mm) coordinates
	g.SetPen1(paint.Color{100, 149, 237, 200}, 0.2)
	g.Rectangle(this.Bounds())
	g.Stroke()

	// Render the widget into an offscreen pixmap to avoid breaking the
	// main painter's clip/transform state. The previous approach used
	// ResetClip()+ResetMatrix() which removed the clip set by DrawAll,
	// causing ghost artifacts when items were moved.
	ww, wh := this.widget.Width(), this.widget.Height()
	if ww <= 0 || wh <= 0 {
		return
	}

	iww, iwh := int(paint.Round(ww)), int(paint.Round(wh))
	if iww <= 0 || iwh <= 0 {
		return
	}

	// Reuse the cached pixmap when size is unchanged and content is clean.
	// This avoids allocating a new cairo surface every single frame.
	if this.cachedPixmap == nil || this.pixmapDirty || iww != this.cachedW || iwh != this.cachedH {
		offscreen := paint.NewPixmap(iww, iwh)
		og := offscreen.NewPainter()
		og.SetBrush1(gui.Theme().FormColor)
		og.Rectangle(0, 0, ww, wh)
		og.Fill()
		func() {
			defer func() { recover() }()
			gui.DrawWidgetAll(this.widget, og, 0, 0, 0, 0, ww, wh)
		}()

		this.cachedPixmap = offscreen
		this.cachedW = iww
		this.cachedH = iwh
		this.pixmapDirty = false
	}

	// Draw the offscreen pixmap at the item position in scene coordinates.
	// The current transform converts mm -> pixels, so we translate to the
	// item origin, then scale from pixel-space back to mm-space so the
	// pixmap appears at the correct size.
	g.Translate(this.Pos())
	scaleX := this.Width() / ww
	scaleY := this.Height() / wh
	g.Scale(scaleX, scaleY)
	g.DrawPixmap(this.cachedPixmap)

	// Draw layout container visual indicators (dashed border + type label)
	if label := layoutTypeLabel(this.factoryName); label != "" {
		g.Scale(1.0/scaleX, 1.0/scaleY) // back to mm space
		w, h := this.Width(), this.Height()

		// Draw dashed blue border using short line segments
		dashLen := 1.5 // mm per dash
		gapLen := 1.0  // mm per gap
		g.SetPen1(paint.Color{66, 133, 244, 140}, 0.15)

		// Top edge
		for cx := 0.0; cx < w; cx += dashLen + gapLen {
			end := cx + dashLen
			if end > w {
				end = w
			}
			g.MoveTo(cx, 0)
			g.LineTo(end, 0)
			g.Stroke()
		}
		// Bottom edge
		for cx := 0.0; cx < w; cx += dashLen + gapLen {
			end := cx + dashLen
			if end > w {
				end = w
			}
			g.MoveTo(cx, h)
			g.LineTo(end, h)
			g.Stroke()
		}
		// Left edge
		for cy := 0.0; cy < h; cy += dashLen + gapLen {
			end := cy + dashLen
			if end > h {
				end = h
			}
			g.MoveTo(0, cy)
			g.LineTo(0, end)
			g.Stroke()
		}
		// Right edge
		for cy := 0.0; cy < h; cy += dashLen + gapLen {
			end := cy + dashLen
			if end > h {
				end = h
			}
			g.MoveTo(w, cy)
			g.LineTo(w, end)
			g.Stroke()
		}

		// Draw type label at top-left
		g.SetFont(paint.NewFont("", 6, true, false))
		g.SetBrush1(paint.Color{66, 133, 244, 200})
		g.DrawText1(0.5, 2.0, label)

		g.Scale(scaleX, scaleY) // restore pixel space for subsequent drawing
	}

	// Draw lock icon overlay if widget is locked
	if this.IsLocked() {
		g.Scale(1.0/scaleX, 1.0/scaleY) // back to mm space
		g.SetBrush(paint.Color{255, 165, 0, 180})
		lockSize := 2.5 // mm
		lx := this.Width() - lockSize - 0.5
		ly := 0.5
		// Small lock indicator rectangle
		g.Rectangle(lx, ly, lockSize, lockSize)
		g.Fill()
		// Lock symbol: outline
		g.SetPen1(paint.Color{255, 255, 255, 220}, 0.15)
		cx := lx + lockSize*0.5
		cy := ly + lockSize*0.5
		// Lock body
		bw := lockSize * 0.6
		bh := lockSize * 0.4
		g.Rectangle(cx-bw*0.5, cy, bw, bh)
		g.Stroke()
		// Lock shackle (arc-like shape using lines)
		sw := lockSize * 0.35
		sh := lockSize * 0.3
		g.MoveTo(cx-sw*0.5, cy)
		g.LineTo(cx-sw*0.5, cy-sh)
		g.LineTo(cx+sw*0.5, cy-sh)
		g.LineTo(cx+sw*0.5, cy)
		g.Stroke()
	}
}

// MarkDirty forces the cached offscreen pixmap to be recreated on the next draw.
// Call this after any change to the embedded widget's visual state (text, value,
// size, etc.) so the designer preview stays in sync.
func (this *FakeWidget) MarkDirty() {
	this.pixmapDirty = true
}

func (this *FakeWidget) SetWidget(iw gui.IWidget) {
	this.widget = iw
	this.factoryName = core.FactoryNameOf(iw)
	this.MarkDirty()
}

func (this *FakeWidget) SizePx() (w, h float64) {
	w, h = this.Size()
	w = gui.MmToPixel(w)
	h = gui.MmToPixel(h)
	return
}

func (this *FakeWidget) Widget() gui.IWidget {
	return this.widget
}

func (this *FakeWidget) Layout() {
	this.widget.SetSize(this.SizePx())
	// Also call the widget's own Layout so it arranges its internals correctly
	if layouter, ok := this.widget.(interface{ Layout() }); ok {
		layouter.Layout()
	}
	this.MarkDirty()
}

// SetEventHandler stores an event binding (event name -> handler function name).
func (this *FakeWidget) SetEventHandler(event, handler string) {
	if this.eventHandlers == nil {
		this.eventHandlers = make(map[string]string)
	}
	this.eventHandlers[event] = handler
}

// EventHandlers returns all event bindings.
func (this *FakeWidget) EventHandlers() map[string]string {
	return this.eventHandlers
}

// RemoveEventHandler removes an event binding by name.
func (this *FakeWidget) RemoveEventHandler(event string) {
	if this.eventHandlers != nil {
		delete(this.eventHandlers, event)
	}
}

func (this *FakeWidget) GetCode() string { return this.code }

func (this *FakeWidget) SetCode(s string) { this.code = s }

// IsLocked returns true if the widget is locked (both position and size).
func (this *FakeWidget) IsLocked() bool {
	return this.IsLockPos() && this.IsLockSize()
}

// SetLocked locks or unlocks both position and size.
func (this *FakeWidget) SetLocked(b bool) {
	this.SetLockPos(b)
	this.SetLockSize(b)
}

func (this *FakeWidget) WidgetFactoryName() string {
	return this.factoryName
}

func (this *FakeWidget) WidgetName() string {
	return this.name
}

func (this *FakeWidget) SetWidgetName(name string) {
	this.name = name
	this.MarkDirty()
}

func (this *FakeWidget) SaveDesign() *core.TDoc {
	doc := core.NewTDoc()
	doc.SetValue(this.factoryName)
	doc.WriteAttr("bounds", this.Bounds1())
	doc.WriteAttr("name", this.WidgetName())
	// Persist lock state
	if this.IsLocked() {
		doc.WriteAttr("locked", true)
	}
	// Persist event handler code
	if this.code != "" {
		doc.WriteAttr("code", this.code)
	}
	// Persist event handlers as child nodes under "events"
	if len(this.eventHandlers) > 0 {
		evtDoc := core.NewTDoc()
		evtDoc.SetKey("events")
		for evtName, handler := range this.eventHandlers {
			child := core.NewTDoc()
			child.SetKey(evtName)
			child.SetValue(handler)
			evtDoc.AddChild(child)
		}
		doc.AddChild(evtDoc)
	}
	// Persist the embedded widget's editable scalar properties (text, values,
	// tag bindings, device settings, ...) under "props" so a configured widget
	// round-trips. Geometry/name are already stored above; this covers the
	// widget-specific fields enumerated via core.IEnumProperties. Each child is
	// keyed by the property id with the serialized value, mirroring "events".
	if this.widget != nil {
		if ep, ok := this.widget.(core.IEnumProperties); ok {
			if props := captureWidgetProperties(ep); len(props) > 0 {
				propsDoc := core.NewTDoc()
				propsDoc.SetKey("props")
				for id, val := range props {
					child := core.NewTDoc()
					child.SetKey(id)
					child.SetValue(val)
					propsDoc.AddChild(child)
				}
				doc.AddChild(propsDoc)
			}
		}
	}
	// Persist nested child widgets under "children" — mirrors
	// GedScene.SaveDesign so a FakeWidget acting as a layout container
	// (VBox/HBox/...) round-trips the widgets laid out inside it. Flat
	// designs (no nested children) emit no "children" node and are
	// byte-identical to before.
	if this.HasChildren() {
		childrenDoc := core.NewTDoc()
		childrenDoc.SetKey("children")
		for _, c := range this.Children() {
			if ia, ok := c.(interface {
				SaveDesign() *core.TDoc
			}); ok {
				if p := ia.SaveDesign(); p != nil {
					childrenDoc.AddChild(p)
				}
			}
		}
		doc.AddChild(childrenDoc)
	}
	return doc
}

// loadChildWidgets reconstructs child FakeWidgets from a "children" TDoc block
// and reparents each under parent, recursing into nested containers. It is the
// single load path shared by GedScene.LoadDesign and FakeWidget.LoadDesign, so
// both behave identically.
//
// Skip-and-continue: when a node's factory name is not registered — a widget
// renamed or removed across versions, or a plugin widget that simply isn't
// loaded — the node is skipped together with its ENTIRE subtree (its children
// cannot attach to a parent that was never created) and tallied into *skipped.
// This never aborts the load, so one unknown widget can no longer take the
// whole .silkui file down with it: the remaining valid siblings still load.
// Each skip logs a warning naming the missing factory; the entry point emits a
// single summary from the running count.
func loadChildWidgets(childrenDoc *core.TDoc, parent graph.IItem, skipped *int) {
	if childrenDoc == nil {
		return
	}
	for _, v := range childrenDoc.Childdren() {
		var factoryName string
		v.Value(&factoryName)
		child, err := NewFakeWidgetFromFactory(factoryName)
		if err != nil {
			shown := factoryName
			if shown == "" {
				shown = "(empty)"
			}
			core.Warn("silkui load: unknown widget factory ", shown, "; skipping node and its subtree")
			*skipped++
			continue
		}
		child.SetParent(parent)
		child.loadDesign(v, skipped)
	}
}

func (this *FakeWidget) LoadDesign(doc *core.TDoc) error {
	var skipped int
	this.loadDesign(doc, &skipped)
	if skipped > 0 {
		core.Warn("silkui load: skipped ", skipped, " widget(s) with unknown factories")
	}
	return nil
}

// loadDesign performs the actual load, threading the skipped-node accumulator
// through the recursion so a single summary can be emitted by the entry point.
// It never fails on an unknown child factory (see loadChildWidgets), so a
// partially valid design still loads its valid widgets.
func (this *FakeWidget) loadDesign(doc *core.TDoc, skipped *int) {
	var rect geom.Rect
	doc.ReadAttr("bounds", &rect)
	this.SetBounds1(rect)
	doc.ReadAttr("name", &this.name)
	doc.ReadAttr("code", &this.code)
	// Restore lock state
	var locked bool
	doc.ReadAttr("locked", &locked)
	if locked {
		this.SetLocked(true)
	}
	// Load event handlers from child nodes under "events"
	evtDoc := doc.ChildByKey("events", false)
	if evtDoc != nil {
		this.eventHandlers = make(map[string]string)
		for _, child := range evtDoc.Childdren() {
			key := child.Key()
			var handler string
			child.Value(&handler)
			if key != "" && handler != "" {
				this.eventHandlers[key] = handler
			}
		}
	}
	// Restore the embedded widget's editable scalar properties from the "props"
	// block written by SaveDesign. The widget was already constructed from the
	// factory (NewFakeWidgetFromFactory) before loadDesign runs, so its setters
	// are ready to receive the persisted values.
	if propsDoc := doc.ChildByKey("props", false); propsDoc != nil && this.widget != nil {
		if ep, ok := this.widget.(core.IEnumProperties); ok {
			vals := make(map[string]string)
			for _, child := range propsDoc.Childdren() {
				key := child.Key()
				if key == "" {
					continue
				}
				var val string
				child.Value(&val)
				vals[key] = val
			}
			applyWidgetProperties(ep, vals)
			this.MarkDirty()
		}
	}
	// Reconstruct nested child widgets from the "children" block via the
	// shared loader, which skips unknown-factory subtrees instead of
	// aborting and recurses for arbitrarily deep container nesting.
	loadChildWidgets(doc.ChildByKey("children", false), this, skipped)
}

func (this *FakeWidget) EnumProperties(list prop.IPropertyList) {
	list.AddProperty("控件名称", this.WidgetName, this.SetWidgetName)
	list.AddProperty("控件类型", this.WidgetFactoryName, nil)
	list.AddProperty("X 位置 (mm)", this.X, this.SetX)
	list.AddProperty("Y 位置 (mm)", this.Y, this.SetY)
	list.AddProperty("宽度 (mm)", this.Width, this.SetWidth)
	list.AddProperty("高度 (mm)", this.Height, this.SetHeight)
	list.AddProperty("锁定", this.IsLocked, this.SetLocked)

	// Also enumerate the embedded widget's properties so the property sheet
	// shows widget-specific fields (text, checked, value, etc.). The
	// mark-dirty adapter wraps each embedded setter so editing one of these
	// values in the sheet repaints the designer preview.
	if this.widget != nil {
		if ep, ok := this.widget.(core.IEnumProperties); ok {
			ep.EnumProperties(newMarkDirtyPropertyList(list, this.MarkDirty))
		}
	}
}

// OnResize is called by graph.Item.SetSize after bounds change (e.g. resize handles).
// It re-layouts the embedded widget so its pixel dimensions stay in sync.
func (this *FakeWidget) OnResize() {
	this.MarkDirty()
	if this.widget != nil {
		this.Layout()
	}
}

func (this *FakeWidget) Generate() gui.IWidget {
	fake, err := NewFakeWidgetFromFactory(this.factoryName)
	if err != nil {
		return nil
	}
	// Copy the designed widget's editable scalar properties (text, values, tag
	// bindings, device host/points, ...) onto the fresh runtime widget so the
	// generated widget keeps its configuration instead of reverting to factory
	// defaults. Reuses the same capture/apply round-trip as save/load.
	if this.widget != nil && fake.widget != nil {
		if src, ok := this.widget.(core.IEnumProperties); ok {
			if dst, ok := fake.widget.(core.IEnumProperties); ok {
				applyWidgetProperties(dst, captureWidgetProperties(src))
			}
		}
	}
	x := gui.MmToPixelZ(this.X())
	y := gui.MmToPixelZ(this.Y())
	w := gui.MmToPixelZ(this.Width())
	h := gui.MmToPixelZ(this.Height())
	fake.widget.SetBounds(x, y, w, h)

	// Recurse into nested children so the live preview matches the
	// generated code (GenerateCode). Children of a simple-AddWidget
	// container (VBox/HBox/Card/GroupBox) are added via AddWidget so the
	// container arranges them; children of any other parent are reparented
	// and keep their absolute bounds. Without this the preview showed an
	// empty container while the emitted source had its children.
	adder, hasAdd := fake.widget.(interface{ AddWidget(gui.IWidget) })
	useAdd := hasAdd && isSimpleAddContainer(this.factoryName)
	for _, c := range this.Children() {
		cf, ok := c.(*FakeWidget)
		if !ok {
			continue
		}
		cw := cf.Generate()
		if cw == nil {
			continue
		}
		if useAdd {
			adder.AddWidget(cw)
		} else {
			cw.SetParent(fake.widget)
		}
	}
	return fake.widget
}

// layoutTypeLabel returns a short label string for layout/container widgets,
// or "" if the widget is not a layout type. Used to draw visual indicators
// in the designer canvas.
func layoutTypeLabel(factoryName string) string {
	switch factoryName {
	case "gui.HBox":
		return "H"
	case "gui.VBox":
		return "V"
	case "gui.GridLayout":
		return "Grid"
	case "gui.FormLayout":
		return "Form"
	case "gui.Card":
		return "Card"
	case "gui.Accordion":
		return "Acc"
	case "gui.StackedWidget":
		return "Stack"
	case "gui.TabWidget":
		return "Tab"
	case "gui.Splitter":
		return "Split"
	case "gui.ScrollArea":
		return "Scroll"
	default:
		return ""
	}
}
