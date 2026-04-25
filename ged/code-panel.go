package ged

import (
	"silk/core"
	"silk/graph"
	"silk/gui"
	"silk/paint"
	"fmt"
	"strings"
	"unicode"
)

func init() {
	core.RegisterFactory("ged.CodePanel", gui.TypeOf(CodePanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.CodePanel",
		Name: "代码",
		Icon: "edit",
		Desc: "事件处理代码编辑器",
	})
}

// CodePanel provides a code editing panel for writing Go event handler code.
// When a widget is selected on the GED canvas, the panel displays an editable
// code template for that widget's events, similar to Qt Creator's signal/slot
// code editor.
type CodePanel struct {
	gui.Widget
	titleBar *gui.Label      // shows which widget's code is being edited
	editor   *gui.CodeEditor // syntax-highlighted code editor
	saveBtn  *gui.Button
	fmtBtn   *gui.Button

	currentWidget *FakeWidget
	gedView       *GedView
}

func NewCodePanel() *CodePanel {
	p := new(CodePanel)
	p.Init(p)
	return p
}

func (this *CodePanel) Init(self gui.IWidget) {
	this.Widget.Init(self)

	// Title label
	this.titleBar = gui.NewLabel("选择控件查看代码")
	this.titleBar.SetParent(self)
	this.titleBar.SetFont(paint.NewFont(gui.Theme().Font.Family(), 12, true, false))

	// Syntax-highlighted code editor
	this.editor = gui.NewCodeEditor()
	this.editor.SetParent(self)
	this.editor.SetFont(paint.NewFont(gui.Theme().Font.Family(), 13, false, false))
	this.editor.SetText("// 在此编写事件处理代码\n")

	// Save button
	this.saveBtn = gui.NewButton1("保存代码", nil)
	this.saveBtn.SetParent(self)
	this.saveBtn.Action().BindFunc0(func() {
		this.SaveCode()
	})

	// Format button (runs gofmt)
	this.fmtBtn = gui.NewButton1("Format", nil)
	this.fmtBtn.SetParent(self)
	this.fmtBtn.Action().BindFunc0(func() {
		this.editor.FormatCode()
	})
}

func (this *CodePanel) Title() string {
	return "代码"
}

func (this *CodePanel) Layout() {
	w, h := this.Size()
	titleH := 24.0
	btnH := 26.0
	btnW := 80.0
	fmtW := 64.0

	this.titleBar.SetBounds(4, 2, w-btnW-fmtW-20, titleH)
	this.fmtBtn.SetBounds(w-btnW-fmtW-8, 2, fmtW, titleH)
	this.saveBtn.SetBounds(w-btnW-4, 2, btnW, titleH)
	this.editor.SetBounds(0, titleH+4, w, h-titleH-btnH-8)
}

func (this *CodePanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Background
	g.SetBrush1(gui.Theme().FormColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Top bar background
	g.SetBrush1(paint.Color{235, 238, 245, 255})
	g.Rectangle(0, 0, w, 28)
	g.Fill()

	// Separator
	g.SetPen1(paint.Color{200, 200, 210, 255}, 0)
	g.MoveTo(0, 28)
	g.LineTo(w, 28)
	g.Stroke()
}

// BindGedView connects the code panel to a GedView so that selecting a widget
// on the canvas automatically loads its event handler code into the editor.
func (this *CodePanel) BindGedView(gv *GedView) {
	this.gedView = gv
	if gv == nil {
		return
	}

	gv.AddSelectionCallback(func(items []graph.IItem) {
		if len(items) == 1 {
			if fake, ok := items[0].(*FakeWidget); ok {
				this.SetWidget(fake)
				return
			}
		}
		this.SetWidget(nil)
	})
}

// SetWidget shows the code for a specific widget.
func (this *CodePanel) SetWidget(fake *FakeWidget) {
	// Auto-save previous widget's code before switching
	if this.currentWidget != nil && this.currentWidget != fake {
		this.SaveCode()
	}

	this.currentWidget = fake
	if fake == nil {
		this.titleBar.SetText("选择控件查看代码")
		this.editor.SetText("// 选择画布上的控件以编辑事件代码\n")
		this.Self().Update()
		return
	}

	name := fake.WidgetName()
	if name == "" {
		name = fake.WidgetFactoryName()
	}
	this.titleBar.SetText("代码: " + name)

	// Load existing code or generate template
	code := fake.GetCode()
	if code == "" {
		code = this.generateTemplate(fake)
	}
	this.editor.SetText(code)
	this.Self().Update()
}

// SaveCode persists the editor content back to the FakeWidget.
func (this *CodePanel) SaveCode() {
	if this.currentWidget != nil {
		text := this.editor.Text()
		this.currentWidget.SetCode(text)
	}
}

// Focus activates the code editor and brings it into focus.
func (this *CodePanel) Focus() {
	this.editor.SetFocus()
	this.Self().Update()
}

// ScrollToHandler scrolls the editor to the handler for the given widget name.
func (this *CodePanel) ScrollToHandler(name string) {
	// Try to find a function matching "on<Name>" or a comment "// <name>"
	targets := []string{
		"func on" + capitalize(name),
		"// " + name,
	}
	for _, target := range targets {
		line := this.editor.FindLineContaining(target)
		if line >= 0 {
			this.editor.ScrollToLine(line)
			return
		}
	}
}

// generateTemplate creates a Go event handler code template based on widget type.
func (this *CodePanel) generateTemplate(fake *FakeWidget) string {
	name := fake.WidgetName()
	if name == "" {
		name = "widget"
	}
	capName := capitalize(name)
	factory := fake.WidgetFactoryName()

	shortFactory := factory
	if idx := strings.LastIndex(factory, "."); idx >= 0 {
		shortFactory = factory[idx+1:]
	}

	// Template signatures MUST match codegen.go callback wrappers exactly.
	// codegen wraps: Signal(func(args...) { handlerName(args...) })
	// So the template handler must accept the same args.
	var code string
	switch shortFactory {
	case "Button":
		code = fmt.Sprintf("// %s click handler\nfunc on%sClick() {\n\tfmt.Println(\"%s clicked\")\n}\n", name, capName, name)
	case "Edit":
		code = fmt.Sprintf("// %s text changed handler\nfunc on%sChanged(text string) {\n\tfmt.Println(\"text:\", text)\n}\n", name, capName)
	case "CheckBox":
		code = fmt.Sprintf("// %s check handler\nfunc on%sToggled(checked bool) {\n\t// checked: true/false\n}\n", name, capName)
	case "RadioButton":
		code = fmt.Sprintf("// %s changed handler\nfunc on%sChanged(selected bool) {\n\t// selected state\n}\n", name, capName)
	case "Slider":
		code = fmt.Sprintf("// %s value changed handler\nfunc on%sValueChanged(v float64) {\n\tfmt.Println(\"value:\", v)\n}\n", name, capName)
	case "SpinBox":
		code = fmt.Sprintf("// %s value changed handler\nfunc on%sValueChanged(v int) {\n\tfmt.Println(\"value:\", v)\n}\n", name, capName)
	case "ComboBox":
		code = fmt.Sprintf("// %s selection handler\nfunc on%sSelected(idx int) {\n\tfmt.Println(\"selected:\", idx)\n}\n", name, capName)
	case "ListWidget":
		code = fmt.Sprintf("// %s selection handler\nfunc on%sSelected(indices []int) {\n\tfmt.Println(\"selected:\", indices)\n}\n", name, capName)
	case "Table":
		code = fmt.Sprintf("// %s row selected handler\nfunc on%sRowSelected(row int) {\n\tfmt.Println(\"row:\", row)\n}\n", name, capName)
	case "TabWidget":
		code = fmt.Sprintf("// %s tab changed handler\nfunc on%sTabChanged(index int) {\n\t// handle tab change\n}\n", name, capName)
	case "ToggleSwitch":
		code = fmt.Sprintf("// %s toggle handler\nfunc on%sToggle(on bool) {\n\tfmt.Println(\"switch:\", on)\n}\n", name, capName)
	case "SearchBox":
		code = fmt.Sprintf("// %s search handler\nfunc on%sSearch(query string) {\n\tfmt.Println(\"search:\", query)\n}\n", name, capName)
	case "NumberInput":
		code = fmt.Sprintf("// %s value handler\nfunc on%sValueChanged(v float64) {\n\tfmt.Println(\"number:\", v)\n}\n", name, capName)
	case "Rating":
		code = fmt.Sprintf("// %s rating handler\nfunc on%sRatingChanged(v int) {\n\tfmt.Println(\"rating:\", v)\n}\n", name, capName)
	case "DatePicker":
		code = fmt.Sprintf("// %s date handler\nfunc on%sDateChanged(y, m, d int) {\n\tfmt.Println(\"date:\", y, m, d)\n}\n", name, capName)
	case "ColorPicker":
		code = fmt.Sprintf("// %s color handler\nfunc on%sColorChanged(c paint.Color) {\n\t// color changed\n}\n", name, capName)
	case "DropdownButton":
		code = fmt.Sprintf("// %s select handler\nfunc on%sSelect(idx int, text string) {\n\tfmt.Println(\"selected:\", idx, text)\n}\n", name, capName)
	case "SwitchGroup":
		code = fmt.Sprintf("// %s change handler\nfunc on%sChange(idx int, text string) {\n\tfmt.Println(\"tab:\", idx, text)\n}\n", name, capName)
	case "Link":
		code = fmt.Sprintf("// %s click handler\nfunc on%sClick(url string) {\n\tfmt.Println(\"link:\", url)\n}\n", name, capName)
	case "Label":
		code = fmt.Sprintf("// %s click handler\nfunc on%sClick() {\n\t// label clicked\n}\n", name, capName)
	case "ProgressBar":
		code = fmt.Sprintf("// %s handler\nfunc on%sComplete() {\n\t// progress reached 100%%\n}\n", name, capName)
	case "GroupBox":
		code = fmt.Sprintf("// %s handler\nfunc on%sEvent() {\n\t// group event\n}\n", name, capName)
	case "ImageView":
		code = fmt.Sprintf("// %s click handler\nfunc on%sClick() {\n\t// image clicked\n}\n", name, capName)
	case "Tag":
		code = fmt.Sprintf("// %s close handler\nfunc on%sClose() {\n\t// tag close button clicked\n}\n", name, capName)
	case "Badge":
		code = fmt.Sprintf("// %s click handler\nfunc on%sClick() {\n\t// badge clicked\n}\n", name, capName)
	case "Avatar":
		code = fmt.Sprintf("// %s click handler\nfunc on%sClick() {\n\t// avatar clicked\n}\n", name, capName)
	case "Breadcrumb":
		code = fmt.Sprintf("// %s navigate handler\nfunc on%sNavigate(idx int, text string) {\n\tfmt.Println(\"navigate:\", idx, text)\n}\n", name, capName)
	case "LabelSeparator":
		code = fmt.Sprintf("// %s handler\nfunc on%sEvent() {\n\t// separator event\n}\n", name, capName)
	case "Placeholder":
		code = fmt.Sprintf("// %s action handler\nfunc on%sAction() {\n\t// placeholder action\n}\n", name, capName)
	case "Timeline":
		code = fmt.Sprintf("// %s step click handler\nfunc on%sStepClick(idx int) {\n\tfmt.Println(\"step:\", idx)\n}\n", name, capName)
	case "NotificationPanel":
		code = fmt.Sprintf("// %s item click handler\nfunc on%sItemClick(idx int) {\n\tfmt.Println(\"notification:\", idx)\n}\n", name, capName)
	case "Card":
		code = fmt.Sprintf("// %s handler\nfunc on%sEvent() {\n\t// card event\n}\n", name, capName)
	case "Accordion":
		code = fmt.Sprintf("// %s section toggle handler\nfunc on%sSectionToggle(idx int, expanded bool) {\n\tfmt.Println(\"section:\", idx, expanded)\n}\n", name, capName)
	case "TreeView":
		code = fmt.Sprintf("// %s item selected handler\nfunc on%sItemSelected() {\n\t// tree item selected\n}\n", name, capName)
	case "LineChart", "BarChart", "PieChart", "Gauge", "ScatterPlot":
		code = fmt.Sprintf("// %s click handler\nfunc on%sClick() {\n\t// chart clicked\n}\n", name, capName)
	case "VBox", "HBox", "GridLayout", "FormLayout":
		code = fmt.Sprintf("// %s layout handler\nfunc on%sEvent() {\n\t// layout event\n}\n", name, capName)
	case "Splitter":
		code = fmt.Sprintf("// %s resize handler\nfunc on%sResize() {\n\t// splitter resized\n}\n", name, capName)
	case "StackedWidget":
		code = fmt.Sprintf("// %s page changed handler\nfunc on%sPageChanged(idx int) {\n\tfmt.Println(\"page:\", idx)\n}\n", name, capName)
	case "Form":
		code = fmt.Sprintf("// %s handler\nfunc on%sEvent() {\n\t// form event\n}\n", name, capName)
	case "Dialog":
		code = fmt.Sprintf("// %s result handler\nfunc on%sResult(result string) {\n\tfmt.Println(\"dialog:\", result)\n}\n", name, capName)
	case "ScrollArea":
		code = fmt.Sprintf("// %s scroll handler\nfunc on%sScroll() {\n\t// scroll event\n}\n", name, capName)
	default:
		code = fmt.Sprintf("// %s event handler\nfunc on%sEvent() {\n\t// implement event logic\n}\n", name, capName)
	}
	return code
}

// capitalize returns s with its first letter upper-cased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
