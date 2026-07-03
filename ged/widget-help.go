package ged

import (
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.WidgetHelp", gui.TypeOf(WidgetHelp{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.WidgetHelp",
		Name: "帮助",
		Icon: "edit",
		Desc: "显示当前选中控件的说明文档",
	})
}

// widgetDoc describes one widget in the built-in documentation set.
type widgetDoc struct {
	Name    string   // display name (e.g. "Button")
	Desc    string   // short human description
	Props   []string // property summaries, one per line
	Events  []string // event summaries, one per line
	Example string   // runnable example snippet
}

// widgetDocs is the static documentation corpus keyed by factory name
// (e.g. "gui.Button"). Used to look up help for the selected FakeWidget.
var widgetDocs = map[string]widgetDoc{
	"gui.Button": {
		Name: "Button",
		Desc: "按钮控件。用户点击触发动作，支持文字与图标。",
		Props: []string{
			"Text — 按钮文字",
			"Icon — 图标 (可选)",
			"Enabled — 是否可用",
		},
		Events: []string{
			"OnClick — 点击时触发",
		},
		Example: "btn := gui.NewButton1(\"确定\", nil)\nbtn.Action().BindFunc0(func(){ /* do something */ })",
	},
	"gui.Label": {
		Name: "Label",
		Desc: "文本标签控件。显示只读文本，支持对齐与换行。",
		Props: []string{
			"Text — 显示文本",
			"Align — 对齐方式",
			"Wrap — 是否换行",
		},
		Example: "lbl := gui.NewLabel(\"Hello, Silk!\")",
	},
	"gui.Edit": {
		Name: "Edit",
		Desc: "单/多行文本输入框。支持撤销、剪贴板、只读、密码等模式。",
		Props: []string{
			"Text — 文本内容",
			"ReadOnly — 是否只读",
			"Password — 密码模式",
			"Multiline — 多行输入",
		},
		Events: []string{
			"OnEdited — 文本内容变化时触发",
			"OnSubmit — 单行模式按下 Enter 时触发",
		},
		Example: "e := gui.NewEdit(\"\")\ne.SetReadOnly(false)\ne.Action().BindFunc0(func(){ /* on edit */ })",
	},
	"gui.CheckBox": {
		Name: "CheckBox",
		Desc: "复选框。用于二元或三态勾选。",
		Props: []string{
			"Text — 标签文字",
			"Checked — 是否选中",
			"TriState — 是否三态",
		},
		Events: []string{
			"OnChanged — 状态变化时触发",
		},
		Example: "cb := gui.NewCheckBox(\"同意条款\")\ncb.SetChecked(true)",
	},
	"gui.RadioButton": {
		Name: "RadioButton",
		Desc: "单选按钮。与其他同组的 RadioButton 互斥选中。",
		Props: []string{
			"Text — 标签文字",
			"Checked — 是否选中",
		},
		Events: []string{
			"OnChanged — 选中状态变化时触发",
		},
		Example: "rb := gui.NewRadioButton(\"选项 A\")\nrb.SetChecked(true)",
	},
	"gui.ComboBox": {
		Name: "ComboBox",
		Desc: "下拉选择框。允许从一组选项中选择一项。",
		Props: []string{
			"Items — 选项列表",
			"Selected — 当前选中项索引",
			"Editable — 是否允许输入自定义文本",
		},
		Events: []string{
			"OnChanged — 选中项变化时触发",
		},
		Example: "cb := gui.NewComboBox()\ncb.Append(gui.ListItem{Text: \"Apple\"})\ncb.Append(gui.ListItem{Text: \"Banana\"})",
	},
	"gui.SpinBox": {
		Name: "SpinBox",
		Desc: "数字微调框。通过上/下按钮或键盘调整数值。",
		Props: []string{
			"Value — 当前值",
			"Min / Max — 取值范围",
			"Step — 步长",
		},
		Example: "sb := gui.NewSpinBox()\nsb.SetRange(0, 100)\nsb.SetValue(50)",
	},
	"gui.Slider": {
		Name: "Slider",
		Desc: "滑动条。拖动滑块选择一个连续数值。",
		Props: []string{
			"Value — 当前值",
			"Min / Max — 取值范围",
			"Orientation — 水平 / 垂直",
		},
		Events: []string{
			"OnChanged — 值变化时触发",
		},
		Example: "s := gui.NewSlider()\ns.SetRange(0, 100)\ns.SetValue(30)",
	},
	"gui.ProgressBar": {
		Name: "ProgressBar",
		Desc: "进度条。按比例显示任务完成度。",
		Props: []string{
			"Value — 0.0–1.0 的完成比例",
			"ShowText — 是否显示百分比文本",
		},
		Example: "pb := gui.NewProgressBar()\npb.SetValue(0.65)\npb.SetShowText(true)",
	},
	"gui.ToggleSwitch": {
		Name: "ToggleSwitch",
		Desc: "开关控件。表达 on/off 二元状态，视觉上比 CheckBox 更突出。",
		Props: []string{
			"Checked — 是否打开",
			"Text — 标签文字",
		},
		Events: []string{
			"OnChanged — 状态变化时触发",
		},
		Example: "t := gui.NewToggleSwitch()\nt.SetText(\"通知\")\nt.SetChecked(true)",
	},
	"gui.SearchBox": {
		Name: "SearchBox",
		Desc: "搜索输入框。带占位文本和清除按钮，适合过滤列表场景。",
		Props: []string{
			"Placeholder — 占位提示",
			"Text — 当前输入",
		},
		Events: []string{
			"OnChanged — 每次键入都会触发",
			"OnSubmit — Enter 确认时触发",
		},
		Example: "sb := gui.NewSearchBox()\nsb.SetPlaceholder(\"搜索…\")",
	},
	"gui.NumberInput": {
		Name: "NumberInput",
		Desc: "数字输入框。支持步进与范围校验，不含旋转按钮。",
		Props: []string{
			"Value — 当前值",
			"Min / Max — 范围",
			"Step — 步长",
		},
		Example: "n := gui.NewNumberInput()\nn.SetRange(0, 100)\nn.SetValue(42)",
	},
	"gui.DatePicker": {
		Name: "DatePicker",
		Desc: "日期选择控件。展开后显示月历，支持键盘导航。",
		Props: []string{
			"Date — 当前日期 (Y/M/D)",
		},
		Example: "dp := gui.NewDatePicker()\ndp.SetDate(2026, 4, 17)",
	},
	"gui.ColorPicker": {
		Name: "ColorPicker",
		Desc: "颜色选择器。点击弹出颜色调色板。",
		Props: []string{
			"Color — 当前颜色 (RGBA)",
		},
		Example: "cp := gui.NewColorPicker()\ncp.SetColor(paint.Color{R: 66, G: 133, B: 244, A: 255})",
	},
	"gui.Rating": {
		Name: "Rating",
		Desc: "星级评分控件。点击一颗星设置评分值。",
		Props: []string{
			"Value — 当前评分 (0..Max)",
			"Max — 最大星数",
		},
		Example: "r := gui.NewRating()\nr.SetMax(5)\nr.SetValue(4)",
	},
	"gui.GroupBox": {
		Name: "GroupBox",
		Desc: "带标题的分组容器。在视觉上包裹一组相关控件。",
		Props: []string{
			"Title — 分组标题",
		},
		Example: "gb := gui.NewGroupBox()\ngb.SetTitle(\"设置\")",
	},
	"gui.ImageView": {
		Name: "ImageView",
		Desc: "图片显示控件。支持本地加载并按需缩放。",
		Props: []string{
			"Image — 当前图片",
			"ScaleMode — 缩放策略",
		},
		Example: "iv := gui.NewImageView()\niv.LoadFile(\"/path/to/pic.png\")",
	},
	"gui.Tag": {
		Name: "Tag",
		Desc: "标签徽章。用于标注分类、状态等，常见于列表项。",
		Props: []string{
			"Text — 标签文本",
			"Color — 背景色",
		},
		Example: "t := gui.NewTag(\"NEW\")",
	},
	"gui.Badge": {
		Name: "Badge",
		Desc: "通知徽章。显示数字或小红点，通常叠加在图标上。",
		Props: []string{
			"Count — 计数 (0 则隐藏)",
			"MaxCount — 超过此数显示为 N+",
		},
		Example: "b := gui.NewBadge()\nb.SetCount(12)",
	},
	"gui.Avatar": {
		Name: "Avatar",
		Desc: "头像控件。支持图片或文字首字母回退。",
		Props: []string{
			"Image — 头像图片",
			"Initials — 文字首字母回退",
			"Shape — 圆形 / 方形",
		},
		Example: "a := gui.NewAvatar()\na.SetInitials(\"UK\")",
	},
	"gui.HBox": {
		Name: "HBox",
		Desc: "水平布局容器。从左到右按顺序排列子控件，支持拉伸系数。",
		Props: []string{
			"Spacing — 子控件之间的间距",
			"Stretch — 子控件的拉伸权重",
		},
		Example: "h := gui.NewHBox()\nh.AddChild(btn1)\nh.AddChild(btn2)",
	},
	"gui.VBox": {
		Name: "VBox",
		Desc: "垂直布局容器。从上到下按顺序排列子控件。",
		Props: []string{
			"Spacing — 子控件间距",
			"Stretch — 子控件的拉伸权重",
		},
		Example: "v := gui.NewVBox()\nv.AddChild(label)\nv.AddChild(edit)",
	},
	"gui.GridLayout": {
		Name: "GridLayout",
		Desc: "网格布局容器。按行列组织子控件，支持跨行跨列。",
		Props: []string{
			"Rows / Cols — 网格尺寸",
			"RowSpacing / ColSpacing — 间距",
		},
		Example: "g := gui.NewGridLayout()\ng.SetSize(3, 3)",
	},
	"gui.FormLayout": {
		Name: "FormLayout",
		Desc: "两列表单布局。左列显示标签，右列放置输入控件，自动对齐。",
		Props: []string{
			"LabelAlign — 标签列对齐",
			"Spacing — 行间距",
		},
		Example: "f := gui.NewFormLayout()\nf.AddRow(gui.NewLabel(\"姓名:\"), edit)",
	},
	"gui.Splitter": {
		Name: "Splitter",
		Desc: "可拖动分隔的多窗格容器。用户拖拽中间条调整比例。",
		Props: []string{
			"Orientation — 水平 / 垂直",
			"Ratio — 初始比例",
		},
		Example: "sp := gui.NewSplitter()\nsp.AddPane(leftView)\nsp.AddPane(rightView)",
	},
	"gui.TabWidget": {
		Name: "TabWidget",
		Desc: "选项卡容器。在多个页面之间快速切换，常用于设置面板。",
		Props: []string{
			"Tabs — 选项卡列表",
			"ActiveIndex — 当前激活页",
		},
		Events: []string{
			"OnTabChanged — 切换页面时触发",
		},
		Example: "tw := gui.NewTabWidget()\ntw.AddTab(\"常规\", page1)\ntw.AddTab(\"高级\", page2)",
	},
	"gui.ListWidget": {
		Name: "ListWidget",
		Desc: "列表控件。显示一列条目，支持选中、滚动、多选。",
		Props: []string{
			"Items — 条目列表",
			"SelectionMode — 单选 / 多选",
		},
		Events: []string{
			"OnSelected — 选中变化时触发",
		},
		Example: "lw := gui.NewListWidget()\nlw.Append(gui.ListItem{Text: \"Apple\"})",
	},
	"gui.TreeView": {
		Name: "TreeView",
		Desc: "树视图。展示层级数据，支持展开/收起、缩进。",
		Props: []string{
			"Model — 数据模型",
			"Expanded — 已展开的节点",
		},
		Example: "tv := gui.NewTreeView()\ntv.SetModel(model)",
	},
	"gui.Table": {
		Name: "Table",
		Desc: "表格控件。多列数据展示，支持排序、行选中、表头。",
		Props: []string{
			"Columns — 列定义",
			"Rows — 行数据",
		},
		Events: []string{
			"OnRowActivated — 双击行时触发",
		},
		Example: "t := gui.NewTable()\nt.SetColumns([]string{\"名称\", \"大小\"})",
	},
	"gui.Card": {
		Name: "Card",
		Desc: "卡片容器。带圆角与阴影，突出一组关联内容。",
		Props: []string{
			"Title — 卡片标题",
			"Elevation — 阴影强度",
		},
		Example: "c := gui.NewCard()\nc.SetTitle(\"用户信息\")",
	},
	"gui.Accordion": {
		Name: "Accordion",
		Desc: "折叠面板。点击标题展开或收起内容块，节省垂直空间。",
		Props: []string{
			"Sections — 折叠项列表",
		},
		Example: "a := gui.NewAccordion()\na.AddSection(\"详情\", detailPanel)",
	},
	"gui.LineChart": {
		Name: "LineChart",
		Desc: "折线图。绘制一条或多条序列，支持网格与图例。",
		Props: []string{
			"Series — 数据序列",
			"Title — 图表标题",
		},
		Example: "c := gui.NewLineChart()\nc.AddSeries(\"CPU\", color, []float64{10, 20, 30})",
	},
	"gui.BarChart": {
		Name: "BarChart",
		Desc: "柱状图。用于对比离散类别的数值。",
		Props: []string{
			"Bars — 柱状项列表",
			"Title — 图表标题",
		},
		Example: "c := gui.NewBarChart()\nc.AddBar(\"A\", 30, color)",
	},
	"gui.PieChart": {
		Name: "PieChart",
		Desc: "饼图。显示各部分占整体的比例。",
		Props: []string{
			"Slices — 扇区列表",
			"Title — 图表标题",
		},
		Example: "c := gui.NewPieChart()\nc.AddSlice(\"A\", 40, color)",
	},
	"gui.Gauge": {
		Name: "Gauge",
		Desc: "仪表盘。用指针或弧线显示一个关键指标。",
		Props: []string{
			"Value — 当前值",
			"Unit — 单位",
			"Zones — 阈值颜色带",
		},
		Example: "g := gui.NewGauge()\ng.SetValue(65)\ng.SetUnit(\"%\")",
	},
}

// WidgetHelp shows documentation for the currently selected widget. It is
// a passive panel: callers (design.go) push selection updates via SetWidget.
type WidgetHelp struct {
	gui.Widget
	currentFactory string
	scrollY        float64
}

// NewWidgetHelp creates a new help panel.
func NewWidgetHelp() *WidgetHelp {
	p := new(WidgetHelp)
	p.Init(p)
	return p
}

func (this *WidgetHelp) Init(self gui.IWidget) {
	this.Widget.Init(self)
}

// SetWidget updates the panel to show docs for the given FakeWidget. A nil
// argument clears the panel (shows the placeholder text). Callers should
// pass nil when no selection, or when the selection is not a FakeWidget.
func (this *WidgetHelp) SetWidget(fw *FakeWidget) {
	newFactory := ""
	if fw != nil {
		newFactory = fw.WidgetFactoryName()
	}
	if newFactory == this.currentFactory {
		return
	}
	this.currentFactory = newFactory
	this.scrollY = 0
	this.Self().Update()
}

// CurrentFactory returns the factory name currently displayed (or "").
func (this *WidgetHelp) CurrentFactory() string {
	return this.currentFactory
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

// Draw paints the help content. When no widget is selected, or no doc exists
// for the current factory, a friendly placeholder message is shown instead.
func (this *WidgetHelp) Draw(g paint.Painter) {
	t := gui.Theme()
	w, h := this.Size()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header
	headerH := 28.0
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, headerH)
	g.Fill()
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, headerH)
	g.LineTo(w, headerH)
	g.Stroke()

	titleFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(titleFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, headerH-9, "Widget Help")

	// No selection → placeholder
	if this.currentFactory == "" {
		font := paint.NewFont(t.Font.Family(), 12, false, true)
		g.SetFont(font)
		g.SetBrush1(paint.Color{R: 140, G: 140, B: 155, A: 255})
		g.DrawText1(14, headerH+30, "Select a widget to see documentation")
		g.DrawText1(14, headerH+50, "选中画布上的控件，此处将显示其说明文档。")
		return
	}

	doc, ok := widgetDocs[this.currentFactory]
	if !ok {
		// Fallback: build a placeholder doc from the factory name so the
		// panel still feels alive for widgets without curated text.
		short := this.currentFactory
		if idx := strings.LastIndex(short, "."); idx >= 0 {
			short = short[idx+1:]
		}
		doc = widgetDoc{
			Name: short,
			Desc: "尚无文档。此控件工厂名: " + this.currentFactory,
		}
	}

	// Fonts
	h1Font := paint.NewFont(t.Font.Family(), 16, true, false)
	h2Font := paint.NewFont(t.Font.Family(), 12, true, false)
	bodyFont := paint.NewFont(t.Font.Family(), 12, false, false)
	codeFont := paint.NewFont("Menlo", 11, false, false)

	leftPad := 14.0
	y := headerH + 12 - this.scrollY

	// Title -- widget name
	g.SetFont(h1Font)
	g.SetBrush1(t.HighLightColor)
	g.DrawText1(leftPad, y+16, doc.Name)
	y += 26

	// Factory name as a subtitle in dim color
	g.SetFont(bodyFont)
	g.SetBrush1(paint.Color{R: 130, G: 130, B: 145, A: 255})
	g.DrawText1(leftPad, y+12, "("+this.currentFactory+")")
	y += 20

	// Separator line under the title
	g.SetPen1(paint.Color{R: 220, G: 220, B: 228, A: 255}, 1)
	g.MoveTo(leftPad, y)
	g.LineTo(w-leftPad, y)
	g.Stroke()
	y += 10

	// Description section
	if doc.Desc != "" {
		y = this.drawHeading(g, h2Font, t.TextColor, leftPad, y, "Description")
		g.SetFont(bodyFont)
		g.SetBrush1(t.TextColor)
		y = this.drawWrapped(g, bodyFont, leftPad, y, w-leftPad*2, doc.Desc)
		y += 8
	}

	// Properties section
	if len(doc.Props) > 0 {
		y = this.drawHeading(g, h2Font, t.TextColor, leftPad, y, "Properties")
		g.SetFont(bodyFont)
		g.SetBrush1(t.TextColor)
		for _, p := range doc.Props {
			g.DrawText1(leftPad+12, y+12, "• "+p)
			y += 18
		}
		y += 6
	}

	// Events section
	if len(doc.Events) > 0 {
		y = this.drawHeading(g, h2Font, t.TextColor, leftPad, y, "Events")
		g.SetFont(bodyFont)
		g.SetBrush1(t.TextColor)
		for _, e := range doc.Events {
			g.DrawText1(leftPad+12, y+12, "• "+e)
			y += 18
		}
		y += 6
	}

	// Example section
	if doc.Example != "" {
		y = this.drawHeading(g, h2Font, t.TextColor, leftPad, y, "Example")

		exampleLines := strings.Split(doc.Example, "\n")
		codeH := float64(len(exampleLines))*16 + 12
		boxX, boxY := leftPad, y+2
		boxW := w - leftPad*2
		g.SetBrush1(paint.Color{R: 30, G: 34, B: 42, A: 255})
		g.Rectangle(boxX, boxY, boxW, codeH)
		g.Fill()
		g.SetPen1(paint.Color{R: 70, G: 75, B: 90, A: 255}, 1)
		g.Rectangle(boxX, boxY, boxW, codeH)
		g.Stroke()

		g.SetFont(codeFont)
		g.SetBrush1(paint.Color{R: 220, G: 220, B: 230, A: 255})
		ty := boxY + 14
		for _, line := range exampleLines {
			g.DrawText1(boxX+10, ty, line)
			ty += 16
		}
		y = boxY + codeH + 12
	}
}

// drawHeading paints a bold heading and returns the new y cursor.
func (this *WidgetHelp) drawHeading(g paint.Painter, font paint.Font, color paint.Color, x, y float64, text string) float64 {
	g.SetFont(font)
	g.SetBrush1(color)
	g.DrawText1(x, y+14, text)
	// underline accent
	g.SetPen1(paint.Color{R: 51, G: 120, B: 215, A: 180}, 2)
	ext := font.TextExtents(text)
	g.MoveTo(x, y+18)
	g.LineTo(x+ext.Width, y+18)
	g.Stroke()
	return y + 24
}

// drawWrapped draws text wrapped to maxWidth and returns the new y cursor.
// Uses simple space-based wrapping; fine for short doc descriptions.
func (this *WidgetHelp) drawWrapped(g paint.Painter, font paint.Font, x, y, maxWidth float64, text string) float64 {
	lineHeight := 17.0
	words := strings.Fields(text)
	if len(words) == 0 {
		// Honor pure non-space text (e.g. Chinese without spaces).
		g.DrawText1(x, y+12, text)
		return y + lineHeight
	}
	line := ""
	cursorY := y
	for _, w := range words {
		candidate := line
		if candidate != "" {
			candidate += " "
		}
		candidate += w
		if font.TextExtents(candidate).Width > maxWidth && line != "" {
			g.DrawText1(x, cursorY+12, line)
			cursorY += lineHeight
			line = w
		} else {
			line = candidate
		}
	}
	if line != "" {
		g.DrawText1(x, cursorY+12, line)
		cursorY += lineHeight
	}
	return cursorY
}

// ---------------------------------------------------------------------------
// Interaction
// ---------------------------------------------------------------------------

// OnMouseWheel scrolls the content.
func (this *WidgetHelp) OnMouseWheel(x, y, delta float64) {
	this.scrollY -= delta * 20
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	// No content-length tracking yet; cap at a generous max.
	if this.scrollY > 2000 {
		this.scrollY = 2000
	}
	this.Self().Update()
}

// SizeHints returns the panel's preferred size.
func (this *WidgetHelp) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 320, Height: 400}
}
