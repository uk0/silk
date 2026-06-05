package ged

import (
	"silk/core"
	"silk/gui"
	"silk/paint"
	"math"
	"strings"
)

func init() {
	core.RegisterFactory("ged.WidgetList", gui.TypeOf(WidgetList{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.WidgetList",
		Name: "控件",
		Icon: "window",
		Desc: "显示控件列表",
	})
}

// widgetCategory represents a collapsible group of widgets in the palette.
type widgetCategory struct {
	name      string
	collapsed bool
	items     []gui.ListItem
}

type WidgetList struct {
	gui.ListWidget
	categories []widgetCategory
	filterText string
}

func NewWidgetList() *WidgetList {
	p := new(WidgetList)
	p.Init(p)
	return p
}

// categoryDefs defines which factory names belong in each category.
// The order here determines display order.
var categoryDefs = []struct {
	name  string
	names []string
}{
	{
		name:  "输入控件 (Input)",
		names: []string{"gui.Button", "gui.Edit", "gui.CheckBox", "gui.RadioButton", "gui.ComboBox", "gui.SpinBox", "gui.Slider", "gui.ToggleSwitch", "gui.SearchBox", "gui.NumberInput", "gui.DatePicker", "gui.Calendar", "gui.ColorPicker", "gui.Rating", "gui.DropdownButton", "gui.SwitchGroup"},
	},
	{
		name:  "显示控件 (Display)",
		names: []string{"gui.Label", "gui.ProgressBar", "gui.Spinner", "gui.GroupBox", "gui.ImageView", "gui.Tag", "gui.Badge", "gui.Avatar", "gui.Breadcrumb", "gui.Link", "gui.LabelSeparator", "gui.Placeholder", "gui.Timeline", "gui.Alert"},
	},
	{
		name:  "容器/布局 (Layout)",
		names: []string{"gui.VBox", "gui.HBox", "gui.GridLayout", "gui.FormLayout", "gui.Splitter", "gui.StackedWidget", "gui.TabWidget", "gui.Card", "gui.Accordion"},
	},
	{
		name:  "数据视图 (Data)",
		names: []string{"gui.ListWidget", "gui.TreeView", "gui.Table", "gui.NotificationPanel", "gui.Pagination"},
	},
	{
		name:  "图表 (Charts)",
		names: []string{"gui.LineChart", "gui.BarChart", "gui.PieChart", "gui.Gauge", "gui.ScatterPlot"},
	},
	{
		name:  "对话框/窗口 (Window)",
		names: []string{"gui.Form", "gui.Dialog"},
	},
}

func (this *WidgetList) Init(self gui.IWidget) {
	this.ListWidget.Init(self)

	// Internal/system widgets that should NOT appear in the widget list
	excluded := map[string]bool{
		"ged.WidgetList": true, "ged.CodePanel": true,
		"ged.FileExplorer": true, "ged.EditorTabs": true,
		"gui.Dock": true, "gui.Frame": true, "gui.Menu": true,
		"gui.TabBar": true, "gui.ScrollBar": true, "gui.Space": true,
		"gui.Action": true, "gui.Separator": true, "gui.ButtonBox": true,
		"gui.tooltipWindow": true, "gui.HeaderView": true,
		"gui.ScrollArea": true,
		"prop.PropertySheet": true, "prop.control.CheckBox": true,
		"prop.control.TextEdit": true,
		"graph.DbgTreeView": true, "graph.GraphView": true,
	}

	// Build a set of available widget factory names
	available := map[string]bool{}
	for _, v := range core.AllFactories() {
		if excluded[v.Name()] {
			continue
		}
		p := v.New()
		_, ok := p.(gui.IWidget)
		if ok {
			available[v.Name()] = true
		}
		if ia, ok := p.(core.IClose); ok {
			ia.Close()
		}
	}

	// Build categories from the definition list
	categorized := map[string]bool{}
	for _, cd := range categoryDefs {
		cat := widgetCategory{name: cd.name}
		for _, name := range cd.names {
			if available[name] {
				cat.items = append(cat.items, gui.ListItem{
					Text: widgetFriendlyName(name),
					Data: name,
				})
				categorized[name] = true
			}
		}
		if len(cat.items) > 0 {
			this.categories = append(this.categories, cat)
		}
	}

	// Collect any remaining uncategorized widgets into an "其他 (Other)" category
	var other widgetCategory
	other.name = "其他 (Other)"
	for name := range available {
		if !categorized[name] {
			other.items = append(other.items, gui.ListItem{
				Text: widgetFriendlyName(name),
				Data: name,
			})
		}
	}
	if len(other.items) > 0 {
		this.categories = append(this.categories, other)
	}

	// Populate the underlying ListWidget items from the expanded categories
	this.rebuildFlatList()

	this.SetIconVisible(false)
	this.SetRowHeight(28)

	this.SetDragStartCallback(func(idx []int) (data []interface{}, acts gui.DndAction) {
		for _, i := range idx {
			item := this.Item(i)
			if item.Data != nil {
				// Skip category headers (they have Data == nil or a special marker)
				if _, ok := item.Data.(string); ok {
					data = append(data, item.Data)
				}
			}
		}
		acts = gui.DndCopy
		return
	})
}

// SetFilter updates the search filter text and rebuilds the flat list.
// When filter is non-empty, all matching items are shown regardless of
// category collapse state. Matching is case-insensitive against both
// the friendly display name and the factory name.
func (this *WidgetList) SetFilter(text string) {
	this.filterText = text
	this.rebuildFlatList()
	this.Self().Update()
}

// rebuildFlatList repopulates the underlying ListWidget items from the
// current category/collapsed state. When a filter is active, category
// headers are hidden and all matching items across all categories are
// shown regardless of collapse state.
func (this *WidgetList) rebuildFlatList() {
	this.Clear()
	filter := strings.ToLower(strings.TrimSpace(this.filterText))

	if filter != "" {
		// Filter mode: show all matching items, skip category headers
		for ci := range this.categories {
			cat := &this.categories[ci]
			for _, item := range cat.items {
				factoryName, _ := item.Data.(string)
				friendlyLower := strings.ToLower(item.Text)
				factoryLower := strings.ToLower(factoryName)
				if strings.Contains(friendlyLower, filter) || strings.Contains(factoryLower, filter) {
					this.Append(item)
				}
			}
		}
		return
	}

	// Normal mode: show categories with expand/collapse
	for ci := range this.categories {
		cat := &this.categories[ci]
		// Add category header row
		indicator := "▼ "
		if cat.collapsed {
			indicator = "▶ "
		}
		this.Append(gui.ListItem{
			Text: indicator + cat.name,
			Data: catHeaderData{index: ci},
		})
		// Add widget items if expanded
		if !cat.collapsed {
			for _, item := range cat.items {
				this.Append(item)
			}
		}
	}
}

// catHeaderData is a marker type stored in ListItem.Data to identify category headers.
type catHeaderData struct {
	index int
}

// isCategoryHeader returns true if the item at the given index is a category header.
func (this *WidgetList) isCategoryHeader(idx int) (bool, int) {
	if idx < 0 || idx >= this.Count() {
		return false, -1
	}
	item := this.Item(idx)
	if hd, ok := item.Data.(catHeaderData); ok {
		return true, hd.index
	}
	return false, -1
}

// toggleCategory toggles the collapsed state of a category and rebuilds the list.
func (this *WidgetList) toggleCategory(catIdx int) {
	if catIdx < 0 || catIdx >= len(this.categories) {
		return
	}
	this.categories[catIdx].collapsed = !this.categories[catIdx].collapsed
	this.rebuildFlatList()
	this.Self().Update()
}

// OnLeftDown intercepts clicks on category headers to toggle expand/collapse.
func (this *WidgetList) OnLeftDown(x, y float64) {
	row, _ := this.HitTest(x, y)
	if isHeader, catIdx := this.isCategoryHeader(row); isHeader {
		this.toggleCategory(catIdx)
		return
	}
	this.ListWidget.OnLeftDown(x, y)
}

// Draw overrides ListWidget.Draw to render a clean, Qt Creator-inspired
// widget palette with compact icon+text rows and styled category headers.
func (this *WidgetList) Draw(g paint.Painter) {
	g.Save()
	defer func() {
		gui.Theme().DrawViewFrame(g, 0, 0, this.Width(), this.Height())
		g.Restore()
	}()

	// Background
	g.SetBrush1(paint.Color{R: 245, G: 246, B: 248, A: 255})
	g.Rectangle(0, 0, this.Width(), this.Height())
	g.Fill()

	vw, vh := this.ViewportSizePx()
	_, sy := this.ScrollPos()
	rh := this.RowHeight()
	if rh <= 0 {
		rh = 28
	}

	// Calculate visible row range (sy is in row units)
	firstRow := int(sy)
	lastRow := firstRow + int(vh/rh) + 2
	if lastRow > this.Count() {
		lastRow = this.Count()
	}
	if firstRow < 0 {
		firstRow = 0
	}

	font := gui.Theme().Font
	boldFont := paint.NewFont(font.Family(), 12, true, false)
	normalFont := paint.NewFont(font.Family(), 12, false, false)

	for i := firstRow; i < lastRow; i++ {
		y := float64(i)*rh - sy*rh
		item := this.Item(i)

		isHeader, _ := this.isCategoryHeader(i)

		if isHeader {
			// --- Category header: dark blue-gray background ---
			g.SetBrush1(paint.Color{R: 60, G: 75, B: 95, A: 255})
			g.Rectangle(0, y, vw, rh)
			g.Fill()

			// Subtle bottom highlight line
			g.SetPen1(paint.Color{R: 75, G: 92, B: 115, A: 255}, 0)
			g.MoveTo(0, y+rh-1)
			g.LineTo(vw, y+rh-1)
			g.Stroke()

			// Triangle indicator
			hd, _ := item.Data.(catHeaderData)
			cat := &this.categories[hd.index]
			g.SetBrush1(paint.Color{R: 200, G: 210, B: 225, A: 255})
			if cat.collapsed {
				drawTriangleRight(g, 8, y+9, 10)
			} else {
				drawTriangleDown(g, 6, y+11, 10)
			}

			// Category name text
			g.SetFont(boldFont)
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			fe := boldFont.FontExtents()
			textY := y + fe.Ascent + (rh-fe.Height)*0.5
			g.DrawText1(22, textY, cat.name)
			continue
		}

		// --- Widget row ---

		// Alternating row background
		if i%2 == 0 {
			g.SetBrush1(paint.Color{R: 250, G: 251, B: 253, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 243, G: 244, B: 247, A: 255})
		}
		g.Rectangle(0, y, vw, rh)
		g.Fill()

		// Selection highlight
		if this.ActiveIndex() == i {
			g.SetBrush1(paint.Color{R: 200, G: 220, B: 245, A: 200})
			g.Rectangle(0, y, vw, rh)
			g.Fill()
		}

		// Widget icon
		factoryName, _ := item.Data.(string)
		if factoryName != "" {
			drawWidgetIcon(g, factoryName, 22, y+5, 18)
		}

		// Widget name
		g.SetFont(normalFont)
		g.SetBrush1(paint.Color{R: 50, G: 55, B: 65, A: 255})
		fe := normalFont.FontExtents()
		textY := y + fe.Ascent + (rh-fe.Height)*0.5
		g.DrawText1(46, textY, item.Text)

		// Subtle bottom separator
		g.SetPen1(paint.Color{R: 230, G: 232, B: 236, A: 255}, 0)
		g.MoveTo(20, y+rh)
		g.LineTo(vw, y+rh)
		g.Stroke()
	}
}

// drawTriangleDown draws a downward-pointing filled triangle for expanded categories.
func drawTriangleDown(g paint.Painter, x, y, size float64) {
	half := size / 2
	g.MoveTo(x, y)
	g.LineTo(x+size, y)
	g.LineTo(x+half, y+half)
	g.LineTo(x, y)
	g.Fill()
}

// drawTriangleRight draws a right-pointing filled triangle for collapsed categories.
func drawTriangleRight(g paint.Painter, x, y, size float64) {
	half := size / 2
	g.MoveTo(x, y)
	g.LineTo(x+half, y+half)
	g.LineTo(x, y+size)
	g.LineTo(x, y)
	g.Fill()
}

// drawWidgetIcon renders a small recognizable icon for each widget type.
func drawWidgetIcon(g paint.Painter, factoryName string, x, y, size float64) {
	s := size
	shortName := factoryName
	if idx := strings.LastIndex(factoryName, "."); idx >= 0 {
		shortName = factoryName[idx+1:]
	}

	switch shortName {
	case "Button":
		// Filled rounded rectangle (blue)
		g.SetBrush1(paint.Color{R: 100, G: 149, B: 237, A: 255})
		g.Rectangle(x, y+2, s, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 70, G: 120, B: 210, A: 255}, 1)
		g.Rectangle(x, y+2, s, s-4)
		g.Stroke()

	case "Edit":
		// Text field outline with cursor
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.Rectangle(x, y+2, s, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 150, G: 150, B: 165, A: 255}, 1)
		g.Rectangle(x, y+2, s, s-4)
		g.Stroke()
		// Cursor line
		g.SetPen1(paint.Color{R: 80, G: 80, B: 100, A: 255}, 1)
		g.MoveTo(x+4, y+4)
		g.LineTo(x+4, y+s-4)
		g.Stroke()
		// Text hint lines
		g.SetPen1(paint.Color{R: 180, G: 180, B: 195, A: 255}, 1)
		g.MoveTo(x+6, y+s/2)
		g.LineTo(x+s-3, y+s/2)
		g.Stroke()

	case "CheckBox":
		// Checkbox square with checkmark
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Fill()
		g.SetPen1(paint.Color{R: 120, G: 120, B: 135, A: 255}, 1)
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Stroke()
		// Green checkmark
		g.SetPen1(paint.Color{R: 50, G: 160, B: 50, A: 255}, 2)
		g.MoveTo(x+4, y+s/2)
		g.LineTo(x+s/2-1, y+s-4)
		g.Stroke()
		g.MoveTo(x+s/2-1, y+s-4)
		g.LineTo(x+s-3, y+4)
		g.Stroke()

	case "RadioButton":
		// Circle with inner dot
		cx := x + s/2
		cy := y + s/2
		r := s/2 - 2
		g.SetPen1(paint.Color{R: 120, G: 120, B: 135, A: 255}, 1)
		g.Arc(cx, cy, r, 0, 2*math.Pi)
		g.Stroke()
		// Inner filled dot
		g.SetBrush1(paint.Color{R: 80, G: 140, B: 220, A: 255})
		g.Arc(cx, cy, r*0.45, 0, 2*math.Pi)
		g.Fill()

	case "ComboBox":
		// Rectangle with dropdown arrow
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.Rectangle(x, y+2, s, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 150, G: 150, B: 165, A: 255}, 1)
		g.Rectangle(x, y+2, s, s-4)
		g.Stroke()
		// Down arrow
		ax := x + s - 6
		ay := y + s/2 - 1
		g.SetPen1(paint.Color{R: 80, G: 80, B: 100, A: 255}, 1)
		g.MoveTo(ax-3, ay)
		g.LineTo(ax, ay+3)
		g.Stroke()
		g.MoveTo(ax, ay+3)
		g.LineTo(ax+3, ay)
		g.Stroke()

	case "SpinBox":
		// Rectangle with up/down arrows
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.Rectangle(x, y+2, s, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 150, G: 150, B: 165, A: 255}, 1)
		g.Rectangle(x, y+2, s, s-4)
		g.Stroke()
		// Up arrow
		ax := x + s - 5
		g.SetPen1(paint.Color{R: 80, G: 80, B: 100, A: 255}, 1)
		g.MoveTo(ax-2, y+s/2-1)
		g.LineTo(ax, y+4)
		g.Stroke()
		g.MoveTo(ax, y+4)
		g.LineTo(ax+2, y+s/2-1)
		g.Stroke()
		// Down arrow
		g.MoveTo(ax-2, y+s/2+1)
		g.LineTo(ax, y+s-4)
		g.Stroke()
		g.MoveTo(ax, y+s-4)
		g.LineTo(ax+2, y+s/2+1)
		g.Stroke()

	case "Slider":
		// Horizontal track with circle knob
		trackY := y + s/2
		g.SetPen1(paint.Color{R: 180, G: 180, B: 195, A: 255}, 2)
		g.MoveTo(x+1, trackY)
		g.LineTo(x+s-1, trackY)
		g.Stroke()
		// Filled portion
		g.SetPen1(paint.Color{R: 80, G: 140, B: 220, A: 255}, 2)
		g.MoveTo(x+1, trackY)
		g.LineTo(x+s*0.6, trackY)
		g.Stroke()
		// Knob
		g.SetBrush1(paint.Color{R: 80, G: 140, B: 220, A: 255})
		g.Arc(x+s*0.6, trackY, 3, 0, 2*math.Pi)
		g.Fill()

	case "Label":
		// "Aa" text representation
		g.SetFont(paint.NewFont(gui.Theme().Font.Family(), 13, true, false))
		g.SetBrush1(paint.Color{R: 100, G: 110, B: 130, A: 255})
		g.DrawText1(x+1, y+s-3, "Aa")

	case "ProgressBar":
		// Partially filled bar
		barY := y + 4
		barH := s - 8
		g.SetBrush1(paint.Color{R: 225, G: 228, B: 235, A: 255})
		g.Rectangle(x, barY, s, barH)
		g.Fill()
		g.SetBrush1(paint.Color{R: 80, G: 160, B: 80, A: 255})
		g.Rectangle(x, barY, s*0.65, barH)
		g.Fill()
		g.SetPen1(paint.Color{R: 150, G: 155, B: 165, A: 255}, 1)
		g.Rectangle(x, barY, s, barH)
		g.Stroke()

	case "GroupBox":
		// Dashed border rectangle with title area
		g.SetPen1(paint.Color{R: 140, G: 150, B: 170, A: 255}, 1)
		g.Rectangle(x+1, y+4, s-2, s-5)
		g.Stroke()
		// Title bar area
		g.SetBrush1(paint.Color{R: 245, G: 246, B: 250, A: 255})
		g.Rectangle(x+3, y+1, s*0.5, 5)
		g.Fill()
		g.SetPen1(paint.Color{R: 140, G: 150, B: 170, A: 255}, 1)
		g.MoveTo(x+3, y+2)
		g.LineTo(x+3+s*0.4, y+2)
		g.Stroke()

	case "VBox":
		// 3 horizontal stacked rectangles
		gap := 1.0
		boxH := (s - 2*gap) / 3
		for i := 0; i < 3; i++ {
			by := y + float64(i)*(boxH+gap)
			g.SetBrush1(paint.Color{R: 160, G: 190, B: 230, A: 255})
			g.Rectangle(x+1, by+1, s-2, boxH-1)
			g.Fill()
			g.SetPen1(paint.Color{R: 100, G: 130, B: 180, A: 255}, 1)
			g.Rectangle(x+1, by+1, s-2, boxH-1)
			g.Stroke()
		}

	case "HBox":
		// 3 vertical side-by-side rectangles
		gap := 1.0
		boxW := (s - 2*gap) / 3
		for i := 0; i < 3; i++ {
			bx := x + float64(i)*(boxW+gap)
			g.SetBrush1(paint.Color{R: 160, G: 190, B: 230, A: 255})
			g.Rectangle(bx+1, y+1, boxW-1, s-2)
			g.Fill()
			g.SetPen1(paint.Color{R: 100, G: 130, B: 180, A: 255}, 1)
			g.Rectangle(bx+1, y+1, boxW-1, s-2)
			g.Stroke()
		}

	case "GridLayout":
		// 2x2 grid of small squares
		gap := 2.0
		cellW := (s - gap) / 2
		cellH := (s - gap) / 2
		for row := 0; row < 2; row++ {
			for col := 0; col < 2; col++ {
				cx := x + float64(col)*(cellW+gap)
				cy := y + float64(row)*(cellH+gap)
				g.SetBrush1(paint.Color{R: 170, G: 195, B: 230, A: 255})
				g.Rectangle(cx, cy, cellW, cellH)
				g.Fill()
				g.SetPen1(paint.Color{R: 110, G: 140, B: 185, A: 255}, 1)
				g.Rectangle(cx, cy, cellW, cellH)
				g.Stroke()
			}
		}

	case "FormLayout":
		// Label:input pairs (2 rows)
		for i := 0; i < 2; i++ {
			fy := y + 2 + float64(i)*8
			// Label part
			g.SetBrush1(paint.Color{R: 130, G: 140, B: 160, A: 255})
			g.Rectangle(x, fy, s*0.35, 5)
			g.Fill()
			// Input part
			g.SetPen1(paint.Color{R: 160, G: 165, B: 180, A: 255}, 1)
			g.Rectangle(x+s*0.4, fy, s*0.55, 5)
			g.Stroke()
		}

	case "Splitter":
		// Two panels with vertical divider and arrows
		midX := x + s/2
		g.SetPen1(paint.Color{R: 120, G: 135, B: 160, A: 255}, 1)
		g.Rectangle(x+1, y+1, s/2-3, s-2)
		g.Stroke()
		g.Rectangle(midX+2, y+1, s/2-3, s-2)
		g.Stroke()
		// Grip dots
		g.SetBrush1(paint.Color{R: 120, G: 135, B: 160, A: 255})
		for i := 0; i < 3; i++ {
			dy := y + s/2 - 3 + float64(i)*3
			g.Arc(midX, dy, 1, 0, 2*math.Pi)
			g.Fill()
		}

	case "StackedWidget":
		// Stacked cards effect
		g.SetBrush1(paint.Color{R: 200, G: 210, B: 230, A: 255})
		g.Rectangle(x+4, y, s-4, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x+4, y, s-4, s-4)
		g.Stroke()
		g.SetBrush1(paint.Color{R: 220, G: 228, B: 240, A: 255})
		g.Rectangle(x+2, y+2, s-4, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x+2, y+2, s-4, s-4)
		g.Stroke()
		g.SetBrush1(paint.Color{R: 240, G: 244, B: 252, A: 255})
		g.Rectangle(x, y+4, s-4, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x, y+4, s-4, s-4)
		g.Stroke()

	case "TabWidget":
		// Tabs on top with content area
		tabW := s / 3
		for i := 0; i < 2; i++ {
			tx := x + float64(i)*tabW
			if i == 0 {
				g.SetBrush1(paint.Color{R: 240, G: 244, B: 252, A: 255})
			} else {
				g.SetBrush1(paint.Color{R: 200, G: 210, B: 225, A: 255})
			}
			g.Rectangle(tx, y, tabW, 5)
			g.Fill()
			g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
			g.Rectangle(tx, y, tabW, 5)
			g.Stroke()
		}
		// Content area
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x, y+5, s, s-6)
		g.Stroke()

	case "Table":
		// Grid with header row
		g.SetBrush1(paint.Color{R: 180, G: 195, B: 220, A: 255})
		g.Rectangle(x, y, s, 5)
		g.Fill()
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x, y, s, s)
		g.Stroke()
		// Vertical divider
		g.MoveTo(x+s/2, y)
		g.LineTo(x+s/2, y+s)
		g.Stroke()
		// Horizontal row lines
		for i := 1; i < 3; i++ {
			hy := y + 5 + float64(i)*((s-5)/3)
			g.MoveTo(x, hy)
			g.LineTo(x+s, hy)
			g.Stroke()
		}

	case "ListWidget":
		// 3 horizontal lines
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x, y, s, s)
		g.Stroke()
		for i := 0; i < 3; i++ {
			ly := y + 3 + float64(i)*5
			g.SetBrush1(paint.Color{R: 170, G: 185, B: 210, A: 255})
			g.Rectangle(x+3, ly, s*0.65, 3)
			g.Fill()
		}

	case "TreeView":
		// Tree structure with lines
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x, y, s, s)
		g.Stroke()
		// Tree lines
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.MoveTo(x+4, y+3)
		g.LineTo(x+4, y+s-3)
		g.Stroke()
		// Branches
		for i := 0; i < 3; i++ {
			by := y + 4 + float64(i)*5
			g.MoveTo(x+4, by)
			g.LineTo(x+8, by)
			g.Stroke()
			g.SetBrush1(paint.Color{R: 170, G: 185, B: 210, A: 255})
			g.Rectangle(x+9, by-1, s*0.4, 3)
			g.Fill()
		}

	case "LineChart":
		// Zigzag line chart
		g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
		// Axes
		g.MoveTo(x+2, y+2)
		g.LineTo(x+2, y+s-2)
		g.Stroke()
		g.MoveTo(x+2, y+s-2)
		g.LineTo(x+s-2, y+s-2)
		g.Stroke()
		// Data line
		g.SetPen1(paint.Color{R: 80, G: 160, B: 230, A: 255}, 2)
		g.MoveTo(x+3, y+s*0.7)
		g.LineTo(x+s*0.3, y+s*0.4)
		g.Stroke()
		g.MoveTo(x+s*0.3, y+s*0.4)
		g.LineTo(x+s*0.5, y+s*0.6)
		g.Stroke()
		g.MoveTo(x+s*0.5, y+s*0.6)
		g.LineTo(x+s*0.7, y+s*0.2)
		g.Stroke()
		g.MoveTo(x+s*0.7, y+s*0.2)
		g.LineTo(x+s-3, y+s*0.35)
		g.Stroke()

	case "BarChart":
		// 3 vertical bars
		g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
		g.MoveTo(x+2, y+s-2)
		g.LineTo(x+s-2, y+s-2)
		g.Stroke()
		barW := s * 0.2
		colors := []paint.Color{
			{R: 80, G: 160, B: 230, A: 255},
			{R: 100, G: 190, B: 120, A: 255},
			{R: 230, G: 160, B: 80, A: 255},
		}
		heights := []float64{0.6, 0.8, 0.5}
		for i := 0; i < 3; i++ {
			bx := x + 3 + float64(i)*(barW+2)
			bh := (s - 4) * heights[i]
			by := y + s - 2 - bh
			g.SetBrush1(colors[i])
			g.Rectangle(bx, by, barW, bh)
			g.Fill()
		}

	case "PieChart":
		// Circle with colored sector
		cx := x + s/2
		cy := y + s/2
		r := s/2 - 2
		// Full circle background
		g.SetBrush1(paint.Color{R: 100, G: 190, B: 120, A: 255})
		g.Arc(cx, cy, r, 0, 2*math.Pi)
		g.Fill()
		// Sector slice
		g.SetBrush1(paint.Color{R: 80, G: 160, B: 230, A: 255})
		g.MoveTo(cx, cy)
		g.Arc(cx, cy, r, -math.Pi/2, math.Pi*0.6)
		g.LineTo(cx, cy)
		g.Fill()
		// Another sector
		g.SetBrush1(paint.Color{R: 230, G: 160, B: 80, A: 255})
		g.MoveTo(cx, cy)
		g.Arc(cx, cy, r, math.Pi*0.6, math.Pi*1.2)
		g.LineTo(cx, cy)
		g.Fill()

	case "Gauge":
		// Half circle with needle
		cx := x + s/2
		cy := y + s*0.65
		r := s*0.45 - 1
		// Arc background
		g.SetPen1(paint.Color{R: 180, G: 190, B: 210, A: 255}, 2)
		g.Arc(cx, cy, r, math.Pi, 2*math.Pi)
		g.Stroke()
		// Colored portion
		g.SetPen1(paint.Color{R: 80, G: 180, B: 100, A: 255}, 2)
		g.Arc(cx, cy, r, math.Pi, math.Pi*1.4)
		g.Stroke()
		// Needle
		angle := math.Pi * 1.35
		nx := cx + r*0.75*math.Cos(angle)
		ny := cy + r*0.75*math.Sin(angle)
		g.SetPen1(paint.Color{R: 200, G: 60, B: 60, A: 255}, 1)
		g.MoveTo(cx, cy)
		g.LineTo(nx, ny)
		g.Stroke()
		// Center dot
		g.SetBrush1(paint.Color{R: 80, G: 85, B: 100, A: 255})
		g.Arc(cx, cy, 2, 0, 2*math.Pi)
		g.Fill()

	case "ScatterPlot":
		// Dots pattern with axes
		g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
		g.MoveTo(x+2, y+2)
		g.LineTo(x+2, y+s-2)
		g.Stroke()
		g.MoveTo(x+2, y+s-2)
		g.LineTo(x+s-2, y+s-2)
		g.Stroke()
		// Scatter dots
		g.SetBrush1(paint.Color{R: 80, G: 160, B: 230, A: 255})
		dots := [][2]float64{{0.25, 0.6}, {0.4, 0.35}, {0.55, 0.5}, {0.7, 0.25}, {0.8, 0.45}, {0.35, 0.7}}
		for _, d := range dots {
			dx := x + d[0]*s
			dy := y + d[1]*s
			g.Arc(dx, dy, 1.5, 0, 2*math.Pi)
			g.Fill()
		}

	case "Form":
		// Window icon
		g.SetBrush1(paint.Color{R: 220, G: 228, B: 240, A: 255})
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Fill()
		// Title bar
		g.SetBrush1(paint.Color{R: 100, G: 130, B: 180, A: 255})
		g.Rectangle(x+1, y+1, s-2, 5)
		g.Fill()
		// Border
		g.SetPen1(paint.Color{R: 100, G: 130, B: 180, A: 255}, 1)
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Stroke()

	case "Dialog":
		// Window with buttons
		g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Fill()
		// Title bar
		g.SetBrush1(paint.Color{R: 100, G: 130, B: 180, A: 255})
		g.Rectangle(x+1, y+1, s-2, 5)
		g.Fill()
		g.SetPen1(paint.Color{R: 100, G: 130, B: 180, A: 255}, 1)
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Stroke()
		// Bottom buttons
		g.SetBrush1(paint.Color{R: 180, G: 195, B: 220, A: 255})
		g.Rectangle(x+s*0.3, y+s-5, s*0.28, 3)
		g.Fill()
		g.Rectangle(x+s*0.65, y+s-5, s*0.28, 3)
		g.Fill()

	case "ToggleSwitch":
		// Pill-shaped track with circle knob
		trackH := s * 0.4
		trackY := y + (s-trackH)/2
		trackR := trackH / 2
		// Track
		g.SetBrush1(paint.Color{R: 80, G: 160, B: 230, A: 255})
		g.Arc(x+trackR, trackY+trackR, trackR, math.Pi/2, 3*math.Pi/2)
		g.Arc(x+s-trackR, trackY+trackR, trackR, -math.Pi/2, math.Pi/2)
		g.Rectangle(x+trackR, trackY, s-trackR*2, trackH)
		g.Fill()
		// Knob (right side = ON)
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.Arc(x+s-trackR-1, trackY+trackR, trackR-2, 0, 2*math.Pi)
		g.Fill()

	case "SearchBox":
		// Rounded rect with magnifier icon
		g.SetBrush1(paint.Color{R: 245, G: 245, B: 248, A: 255})
		g.Rectangle(x, y+2, s, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 160, G: 165, B: 180, A: 255}, 1)
		g.Rectangle(x, y+2, s, s-4)
		g.Stroke()
		// Magnifier
		mr := s * 0.18
		mx := x + 5 + mr
		my := y + s/2 - 1
		g.SetPen1(paint.Color{R: 130, G: 135, B: 155, A: 255}, 1.5)
		g.Arc(mx, my, mr, 0, 2*math.Pi)
		g.Stroke()
		hx := mx + mr*0.7
		hy := my + mr*0.7
		g.MoveTo(hx, hy)
		g.LineTo(hx+3, hy+3)
		g.Stroke()

	case "NumberInput":
		// Input field with up/down arrows
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.Rectangle(x, y+2, s, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 150, G: 150, B: 165, A: 255}, 1)
		g.Rectangle(x, y+2, s, s-4)
		g.Stroke()
		// "42" text
		g.SetFont(paint.NewFont(gui.Theme().Font.Family(), 9, false, false))
		g.SetBrush1(paint.Color{R: 80, G: 85, B: 100, A: 255})
		g.DrawText1(x+2, y+s-4, "42")
		// Arrows
		ax := x + s - 5
		g.SetPen1(paint.Color{R: 100, G: 105, B: 120, A: 255}, 1)
		g.MoveTo(ax-2, y+s/2-1)
		g.LineTo(ax, y+4)
		g.Stroke()
		g.MoveTo(ax, y+4)
		g.LineTo(ax+2, y+s/2-1)
		g.Stroke()
		g.MoveTo(ax-2, y+s/2+1)
		g.LineTo(ax, y+s-4)
		g.Stroke()
		g.MoveTo(ax, y+s-4)
		g.LineTo(ax+2, y+s/2+1)
		g.Stroke()

	case "DatePicker":
		// Calendar icon
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x+1, y+3, s-2, s-4)
		g.Stroke()
		// Header bar
		g.SetBrush1(paint.Color{R: 80, G: 140, B: 220, A: 255})
		g.Rectangle(x+1, y+3, s-2, 4)
		g.Fill()
		// Calendar dots
		g.SetBrush1(paint.Color{R: 100, G: 110, B: 140, A: 255})
		for row := 0; row < 2; row++ {
			for col := 0; col < 3; col++ {
				dx := x + 4 + float64(col)*4
				dy := y + 10 + float64(row)*4
				g.Rectangle(dx, dy, 2, 2)
				g.Fill()
			}
		}

	case "ColorPicker":
		// Color swatch grid
		colors := []paint.Color{
			{R: 220, G: 60, B: 60, A: 255}, {R: 60, G: 160, B: 60, A: 255},
			{R: 60, G: 120, B: 220, A: 255}, {R: 220, G: 180, B: 40, A: 255},
		}
		cellS := (s - 2) / 2
		for i, c := range colors {
			cx := x + 1 + float64(i%2)*cellS
			cy := y + 1 + float64(i/2)*cellS
			g.SetBrush1(c)
			g.Rectangle(cx, cy, cellS-1, cellS-1)
			g.Fill()
		}
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Stroke()

	case "Rating":
		// 5 stars
		starR := s * 0.14
		for i := 0; i < 5; i++ {
			sx := x + 2 + float64(i)*(starR*2+1)
			sy := y + s/2
			if i < 3 {
				g.SetBrush1(paint.Color{R: 245, G: 180, B: 40, A: 255})
			} else {
				g.SetBrush1(paint.Color{R: 200, G: 205, B: 215, A: 255})
			}
			g.Arc(sx+starR, sy, starR, 0, 2*math.Pi)
			g.Fill()
		}

	case "DropdownButton":
		// Button with dropdown arrow
		g.SetBrush1(paint.Color{R: 100, G: 149, B: 237, A: 255})
		g.Rectangle(x, y+2, s, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 70, G: 120, B: 210, A: 255}, 1)
		g.Rectangle(x, y+2, s, s-4)
		g.Stroke()
		// Divider line
		g.SetPen1(paint.Color{R: 255, G: 255, B: 255, A: 120}, 1)
		g.MoveTo(x+s-7, y+4)
		g.LineTo(x+s-7, y+s-4)
		g.Stroke()
		// Down arrow
		g.SetPen1(paint.Color{R: 255, G: 255, B: 255, A: 255}, 1)
		ax := x + s - 4
		ay := y + s/2
		g.MoveTo(ax-2, ay-1)
		g.LineTo(ax, ay+1)
		g.Stroke()
		g.MoveTo(ax, ay+1)
		g.LineTo(ax+2, ay-1)
		g.Stroke()

	case "SwitchGroup":
		// Segmented control (3 segments)
		segW := (s - 2) / 3
		for i := 0; i < 3; i++ {
			sx := x + 1 + float64(i)*segW
			if i == 0 {
				g.SetBrush1(paint.Color{R: 80, G: 140, B: 220, A: 255})
			} else {
				g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
			}
			g.Rectangle(sx, y+3, segW, s-6)
			g.Fill()
			g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
			g.Rectangle(sx, y+3, segW, s-6)
			g.Stroke()
		}

	case "ImageView":
		// Image placeholder (mountain landscape icon)
		g.SetBrush1(paint.Color{R: 230, G: 235, B: 242, A: 255})
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Fill()
		g.SetPen1(paint.Color{R: 160, G: 170, B: 190, A: 255}, 1)
		g.Rectangle(x+1, y+1, s-2, s-2)
		g.Stroke()
		// Mountain
		g.SetBrush1(paint.Color{R: 160, G: 180, B: 210, A: 255})
		g.MoveTo(x+3, y+s-3)
		g.LineTo(x+s/2, y+s*0.35)
		g.LineTo(x+s-3, y+s-3)
		g.Fill()
		// Sun
		g.SetBrush1(paint.Color{R: 245, G: 195, B: 80, A: 255})
		g.Arc(x+s*0.7, y+s*0.3, 2.5, 0, 2*math.Pi)
		g.Fill()

	case "Tag":
		// Colored pill tag
		tagH := s * 0.45
		tagY := y + (s-tagH)/2
		r := tagH / 2
		g.SetBrush1(paint.Color{R: 80, G: 140, B: 220, A: 255})
		g.Arc(x+r+1, tagY+r, r, math.Pi/2, 3*math.Pi/2)
		g.Arc(x+s-r-1, tagY+r, r, -math.Pi/2, math.Pi/2)
		g.Rectangle(x+r+1, tagY, s-2*r-2, tagH)
		g.Fill()

	case "Badge":
		// Circle with number
		cx := x + s/2
		cy := y + s/2
		g.SetBrush1(paint.Color{R: 235, G: 65, B: 65, A: 255})
		g.Arc(cx, cy, s*0.35, 0, 2*math.Pi)
		g.Fill()
		g.SetFont(paint.NewFont(gui.Theme().Font.Family(), 8, true, false))
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.DrawText1(cx-3, cy+3, "7")

	case "Avatar":
		// Circle with initials
		cx := x + s/2
		cy := y + s/2
		r := s/2 - 2
		g.SetBrush1(paint.Color{R: 80, G: 140, B: 220, A: 255})
		g.Arc(cx, cy, r, 0, 2*math.Pi)
		g.Fill()
		g.SetFont(paint.NewFont(gui.Theme().Font.Family(), 9, true, false))
		g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		g.DrawText1(cx-5, cy+3, "Av")

	case "Breadcrumb":
		// Path segments with arrows
		g.SetPen1(paint.Color{R: 130, G: 145, B: 175, A: 255}, 1)
		segW := s * 0.28
		for i := 0; i < 3; i++ {
			sx := x + float64(i)*(segW+3)
			sy := y + s/2
			g.SetBrush1(paint.Color{R: 170, G: 185, B: 215, A: 255})
			g.Rectangle(sx, sy-2, segW, 4)
			g.Fill()
			if i < 2 {
				g.SetPen1(paint.Color{R: 150, G: 160, B: 185, A: 255}, 1)
				g.MoveTo(sx+segW+1, sy)
				g.LineTo(sx+segW+3, sy)
				g.Stroke()
			}
		}

	case "Link":
		// Underlined text
		g.SetFont(paint.NewFont(gui.Theme().Font.Family(), 11, false, false))
		g.SetBrush1(paint.Color{R: 40, G: 100, B: 220, A: 255})
		g.DrawText1(x+1, y+s-4, "Link")
		g.SetPen1(paint.Color{R: 40, G: 100, B: 220, A: 255}, 1)
		g.MoveTo(x+1, y+s-2)
		g.LineTo(x+s-4, y+s-2)
		g.Stroke()

	case "LabelSeparator":
		// Line with text gap
		lineY := y + s/2
		g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
		g.MoveTo(x, lineY)
		g.LineTo(x+s*0.3, lineY)
		g.Stroke()
		g.MoveTo(x+s*0.7, lineY)
		g.LineTo(x+s, lineY)
		g.Stroke()
		g.SetBrush1(paint.Color{R: 140, G: 150, B: 170, A: 255})
		g.SetFont(paint.NewFont(gui.Theme().Font.Family(), 7, false, false))
		g.DrawText1(x+s*0.32, lineY+3, "OR")

	case "Placeholder":
		// Empty state icon
		g.SetPen1(paint.Color{R: 190, G: 195, B: 210, A: 255}, 1)
		g.Rectangle(x+2, y+2, s-4, s-4)
		g.Stroke()
		// Dotted circle
		cx := x + s/2
		cy := y + s/2 - 1
		g.SetPen1(paint.Color{R: 190, G: 200, B: 215, A: 255}, 1)
		for a := 0.0; a < 2*math.Pi; a += math.Pi / 4 {
			dx := cx + (s*0.25)*math.Cos(a)
			dy := cy + (s*0.25)*math.Sin(a)
			g.Arc(dx, dy, 1, 0, 2*math.Pi)
			g.Fill()
		}

	case "Timeline":
		// Vertical dots with lines
		for i := 0; i < 3; i++ {
			dy := y + 3 + float64(i)*6
			cx := x + s*0.3
			colors := []paint.Color{
				{R: 80, G: 180, B: 100, A: 255},
				{R: 80, G: 140, B: 220, A: 255},
				{R: 190, G: 195, B: 210, A: 255},
			}
			g.SetBrush1(colors[i])
			g.Arc(cx, dy, 2, 0, 2*math.Pi)
			g.Fill()
			if i < 2 {
				g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
				g.MoveTo(cx, dy+2)
				g.LineTo(cx, dy+4)
				g.Stroke()
			}
			// Text line
			g.SetBrush1(paint.Color{R: 160, G: 170, B: 190, A: 255})
			g.Rectangle(cx+4, dy-1, s*0.45, 2)
			g.Fill()
		}

	case "NotificationPanel":
		// Stacked notification cards
		for i := 0; i < 3; i++ {
			ny := y + 1 + float64(i)*6
			// Left color bar
			colors := []paint.Color{
				{R: 80, G: 140, B: 220, A: 255},
				{R: 80, G: 180, B: 100, A: 255},
				{R: 240, G: 170, B: 40, A: 255},
			}
			g.SetBrush1(colors[i])
			g.Rectangle(x+1, ny, 2, 4)
			g.Fill()
			// Card body
			g.SetPen1(paint.Color{R: 200, G: 205, B: 215, A: 255}, 1)
			g.Rectangle(x+3, ny, s-5, 4)
			g.Stroke()
		}

	default:
		// Generic widget icon: simple blue square with border
		g.SetBrush1(paint.Color{R: 175, G: 195, B: 225, A: 255})
		g.Rectangle(x+2, y+2, s-4, s-4)
		g.Fill()
		g.SetPen1(paint.Color{R: 130, G: 150, B: 185, A: 255}, 1)
		g.Rectangle(x+2, y+2, s-4, s-4)
		g.Stroke()
	}
}

// widgetFriendlyName extracts a readable name from a factory name like "gui.Button" -> "Button"
func widgetFriendlyName(factoryName string) string {
	if idx := strings.LastIndex(factoryName, "."); idx >= 0 {
		return factoryName[idx+1:]
	}
	return factoryName
}
