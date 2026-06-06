package ged

import (
	"fmt"
	"math"
	"os"
	"strings"
	"unicode"

	"silk/core"
	"silk/graph"
)

// simpleAddContainers are factory names whose Go type exposes a
// single-argument AddWidget(iw) — so codegen can place a nested child
// with `ui.parent.AddWidget(ui.child)` and let the container own its
// arrangement. GridLayout (needs row/col) and FormLayout (needs a row
// label) are intentionally excluded: the designer doesn't yet track
// the extra placement args, so children of those fall back to
// SetParent + absolute SetBounds.
var simpleAddContainers = map[string]bool{
	"gui.VBox":     true,
	"gui.HBox":     true,
	"gui.Card":     true,
	"gui.GroupBox": true,
}

func isSimpleAddContainer(factoryName string) bool {
	return simpleAddContainers[factoryName]
}

// CodeGenOptions controls code generation output.
type CodeGenOptions struct {
	PackageName string // "main" by default
	TypeName    string // struct name, derived from form title if empty
	FileName    string // output file path (optional, for GenerateCodeFile)
	ModulePath  string // module path from go.mod; emitted as "// Module: <path>" when non-empty
}

// widgetMapping maps a factory name to its Go type, import path, and
// constructor expression used during code generation.
type widgetMapping struct {
	goType      string // e.g. "*gui.Button"
	importPath  string // e.g. "silk/gui"
	constructor string // e.g. `gui.NewButton1("", nil)`
}

var factoryMap = map[string]widgetMapping{
	"gui.Button":        {goType: "*gui.Button", importPath: "silk/gui", constructor: `gui.NewButton1("", nil)`},
	"gui.Edit":          {goType: "*gui.Edit", importPath: "silk/gui", constructor: `gui.NewEdit()`},
	"gui.Label":         {goType: "*gui.Label", importPath: "silk/gui", constructor: `gui.NewLabel("")`},
	"gui.CheckBox":      {goType: "*gui.CheckBox", importPath: "silk/gui", constructor: `gui.NewCheckBox()`},
	"gui.RadioButton":   {goType: "*gui.RadioButton", importPath: "silk/gui", constructor: `gui.NewRadioButton("", nil)`},
	"gui.ProgressBar":   {goType: "*gui.ProgressBar", importPath: "silk/gui", constructor: `gui.NewProgressBar()`},
	"gui.Slider":        {goType: "*gui.Slider", importPath: "silk/gui", constructor: `gui.NewSlider(0, 100)`},
	"gui.ComboBox":      {goType: "*gui.ComboBox", importPath: "silk/gui", constructor: `gui.NewComboBox()`},
	"gui.ListWidget":    {goType: "*gui.ListWidget", importPath: "silk/gui", constructor: `gui.NewListWidget()`},
	"gui.TreeView":      {goType: "*gui.TreeView", importPath: "silk/gui", constructor: `gui.NewTreeView()`},
	"gui.VBox":          {goType: "*gui.VBox", importPath: "silk/gui", constructor: `gui.NewVBox()`},
	"gui.HBox":          {goType: "*gui.HBox", importPath: "silk/gui", constructor: `gui.NewHBox()`},
	"gui.Table":         {goType: "*gui.Table", importPath: "silk/gui", constructor: `gui.NewTable()`},
	"gui.ScrollArea":    {goType: "*gui.ScrollArea", importPath: "silk/gui", constructor: `gui.NewScrollArea()`},
	"gui.Form":          {goType: "*gui.Form", importPath: "silk/gui", constructor: `gui.NewForm()`},
	"gui.SpinBox":       {goType: "*gui.SpinBox", importPath: "silk/gui", constructor: `gui.NewSpinBox()`},
	"gui.GroupBox":      {goType: "*gui.GroupBox", importPath: "silk/gui", constructor: `gui.NewGroupBox("")`},
	"gui.StackedWidget": {goType: "*gui.StackedWidget", importPath: "silk/gui", constructor: `gui.NewStackedWidget()`},
	"gui.TabWidget":     {goType: "*gui.TabWidget", importPath: "silk/gui", constructor: `gui.NewTabWidget()`},
	"gui.GridLayout":    {goType: "*gui.GridLayout", importPath: "silk/gui", constructor: `gui.NewGridLayout()`},
	"gui.FormLayout":    {goType: "*gui.FormLayout", importPath: "silk/gui", constructor: `gui.NewFormLayout()`},
	"gui.Splitter":      {goType: "*gui.Splitter", importPath: "silk/gui", constructor: `gui.NewSplitter(false)`},
	"gui.ToolBar":       {goType: "*gui.ToolBar", importPath: "silk/gui", constructor: `gui.NewToolBar()`},
	"gui.StatusBar":     {goType: "*gui.StatusBar", importPath: "silk/gui", constructor: `gui.NewStatusBar()`},
	"gui.Dialog":        {goType: "*gui.Dialog", importPath: "silk/gui", constructor: `gui.NewDialog("", nil)`},
	"gui.LineChart":     {goType: "*gui.LineChart", importPath: "silk/gui", constructor: `gui.NewLineChart()`},
	"gui.BarChart":      {goType: "*gui.BarChart", importPath: "silk/gui", constructor: `gui.NewBarChart()`},
	"gui.PieChart":      {goType: "*gui.PieChart", importPath: "silk/gui", constructor: `gui.NewPieChart()`},
	"gui.Gauge":         {goType: "*gui.Gauge", importPath: "silk/gui", constructor: `gui.NewGauge()`},
	"gui.ScatterPlot":   {goType: "*gui.ScatterPlot", importPath: "silk/gui", constructor: `gui.NewScatterPlot()`},
	"gui.ToggleSwitch":  {goType: "*gui.ToggleSwitch", importPath: "silk/gui", constructor: `gui.NewToggleSwitch()`},
	"gui.SearchBox":     {goType: "*gui.SearchBox", importPath: "silk/gui", constructor: `gui.NewSearchBox()`},
	"gui.NumberInput":   {goType: "*gui.NumberInput", importPath: "silk/gui", constructor: `gui.NewNumberInput()`},
	"gui.ImageView":     {goType: "*gui.ImageView", importPath: "silk/gui", constructor: `gui.NewImageView()`},
	"gui.Tag":           {goType: "*gui.Tag", importPath: "silk/gui", constructor: `gui.NewTag("")`},
	"gui.Card":          {goType: "*gui.Card", importPath: "silk/gui", constructor: `gui.NewCard("")`},
	"gui.Badge":         {goType: "*gui.Badge", importPath: "silk/gui", constructor: `gui.NewBadge()`},
	"gui.Avatar":        {goType: "*gui.Avatar", importPath: "silk/gui", constructor: `gui.NewAvatar()`},
	"gui.Breadcrumb":    {goType: "*gui.Breadcrumb", importPath: "silk/gui", constructor: `gui.NewBreadcrumb()`},
	"gui.Accordion":     {goType: "*gui.Accordion", importPath: "silk/gui", constructor: `gui.NewAccordion()`},
	"gui.DatePicker":    {goType: "*gui.DatePicker", importPath: "silk/gui", constructor: `gui.NewDatePicker()`},
	"gui.ColorPicker":   {goType: "*gui.ColorPicker", importPath: "silk/gui", constructor: `gui.NewColorPicker()`},
	"gui.Rating":            {goType: "*gui.Rating", importPath: "silk/gui", constructor: `gui.NewRating()`},
	"gui.DropdownButton":    {goType: "*gui.DropdownButton", importPath: "silk/gui", constructor: `gui.NewDropdownButton()`},
	"gui.SwitchGroup":       {goType: "*gui.SwitchGroup", importPath: "silk/gui", constructor: `gui.NewSwitchGroup()`},
	"gui.Link":              {goType: "*gui.Link", importPath: "silk/gui", constructor: `gui.NewLink()`},
	"gui.LabelSeparator":    {goType: "*gui.LabelSeparator", importPath: "silk/gui", constructor: `gui.NewLabelSeparator()`},
	"gui.Placeholder":       {goType: "*gui.Placeholder", importPath: "silk/gui", constructor: `gui.NewPlaceholder()`},
	"gui.Timeline":          {goType: "*gui.Timeline", importPath: "silk/gui", constructor: `gui.NewTimeline()`},
	"gui.NotificationPanel": {goType: "*gui.NotificationPanel", importPath: "silk/gui", constructor: `gui.NewNotificationPanel()`},
}

// GenerateRunnable controls whether a complete runnable main() is generated.
type GenerateRunnable bool

// GenerateCode generates Go source code from the scene's design.
// It produces a complete, compilable Go program with main().
func (scene *GedScene) GenerateCode(opts CodeGenOptions) string {
	if opts.PackageName == "" {
		opts.PackageName = "main"
	}
	if opts.TypeName == "" {
		opts.TypeName = sanitizeIdentifier(scene.FormTitle()) + "UI"
	}

	imports := make(map[string]bool)
	imports["silk/gui"] = true  // always needed for Form
	imports["silk/core"] = true // needed for EventLoop in main()

	type fieldInfo struct {
		name          string
		goType        string
		constructor   string
		factoryName   string
		x, y, w, h    float64
		defaultText   string
		eventHandlers map[string]string
		code          string // user-written event handler code
		parentField   string // owning container field; "" = top-level (ui.Form)
		parentAdd     bool   // parent is a simple-AddWidget container
	}

	var fields []fieldInfo
	nameCount := make(map[string]int)
	usedNames := make(map[string]bool) // every field identifier emitted, for collision-free uniqueness

	// collect walks the scene tree depth-first, appending a field for
	// every FakeWidget and recursing into container children. parentField
	// is the owning widget's field name ("" for scene-level widgets, which
	// parent to ui.Form); parentAdd records whether that parent is a
	// simple-AddWidget container so the emit loop can choose AddWidget vs
	// SetParent+SetBounds. Parents are appended before their children, so
	// the constructor emission order guarantees a container exists before
	// AddWidget is called on it.
	var collect func(items []graph.IItem, parentField string, parentAdd bool)
	collect = func(items []graph.IItem, parentField string, parentAdd bool) {
		for _, child := range items {
			fake, ok := child.(*FakeWidget)
			if !ok {
				continue
			}

			factoryName := fake.WidgetFactoryName()
			mapping, known := factoryMap[factoryName]

			fieldName := fake.WidgetName()
			if fieldName == "" {
				// derive from factory name, e.g. "gui.Button" -> "Button"
				parts := strings.Split(factoryName, ".")
				base := parts[len(parts)-1]
				nameCount[base]++
				fieldName = fmt.Sprintf("%s_%d", base, nameCount[base])
			}
			fieldName = sanitizeIdentifier(fieldName)
			// Guarantee struct-field uniqueness across the whole tree —
			// an explicit widget name can collide with another widget's
			// auto-derived name (e.g. a user-named "Button_1" vs the first
			// unnamed Button). Suffix until unique so generated Go never
			// declares two fields with the same identifier.
			if base := fieldName; usedNames[fieldName] {
				for i := 2; usedNames[fieldName]; i++ {
					fieldName = fmt.Sprintf("%s_%d", base, i)
				}
			}
			usedNames[fieldName] = true

			var goType, constructor string
			if known {
				goType = mapping.goType
				constructor = mapping.constructor
				imports[mapping.importPath] = true
			} else {
				goType = "gui.IWidget"
				constructor = fmt.Sprintf(`core.New("%s").(gui.IWidget)`, factoryName)
				imports["silk/gui"] = true
				imports["silk/core"] = true
			}

			x := roundPx(fake.X())
			y := roundPx(fake.Y())
			w := roundPx(fake.Width())
			h := roundPx(fake.Height())

			// Get default text from the widget if it has one
			var defaultText string
			if fake.Widget() != nil {
				if t, ok := fake.Widget().(interface{ Text() string }); ok {
					defaultText = t.Text()
				} else if t, ok := fake.Widget().(interface{ Title() string }); ok {
					defaultText = t.Title()
				}
			}

			fields = append(fields, fieldInfo{
				name:        fieldName,
				goType:      goType,
				constructor: constructor,
				factoryName: factoryName,
				x:           x, y: y, w: w, h: h,
				defaultText:   defaultText,
				eventHandlers: fake.EventHandlers(),
				code:          fake.GetCode(),
				parentField:   parentField,
				parentAdd:     parentAdd,
			})

			if fake.HasChildren() {
				collect(fake.Children(), fieldName, isSimpleAddContainer(factoryName))
			}
		}
	}
	collect(scene.Children(), "", false)

	var buf strings.Builder

	// Header
	buf.WriteString("// Code generated by Silk Designer Editor. DO NOT EDIT.\n")
	buf.WriteString(fmt.Sprintf("package %s\n\n", opts.PackageName))

	// Imports
	buf.WriteString("import (\n")
	for imp := range imports {
		buf.WriteString(fmt.Sprintf("\t%q\n", imp))
	}
	buf.WriteString(")\n\n")

	// Struct
	buf.WriteString(fmt.Sprintf("type %s struct {\n", opts.TypeName))
	buf.WriteString("\tForm *gui.Form\n")
	for _, f := range fields {
		buf.WriteString(fmt.Sprintf("\t%s %s\n", f.name, f.goType))
	}
	buf.WriteString("}\n\n")

	// Constructor
	constructorName := "New" + opts.TypeName
	formW := roundPx(scene.Width())
	formH := roundPx(scene.Height())
	title := scene.FormTitle()

	buf.WriteString(fmt.Sprintf("func %s() *%s {\n", constructorName, opts.TypeName))
	buf.WriteString(fmt.Sprintf("\tui := new(%s)\n\n", opts.TypeName))

	// Form setup
	buf.WriteString("\tui.Form = gui.NewForm()\n")
	buf.WriteString(fmt.Sprintf("\tui.Form.SetTitle(%q)\n", title))
	buf.WriteString(fmt.Sprintf("\tui.Form.SetSize(%s, %s)\n\n", fmtFloat(formW), fmtFloat(formH)))

	// Children
	for _, f := range fields {
		buf.WriteString(fmt.Sprintf("\tui.%s = %s\n", f.name, f.constructor))
		switch {
		case f.parentField == "":
			// Top-level widget: parent to the form at absolute bounds.
			buf.WriteString(fmt.Sprintf("\tui.%s.SetParent(ui.Form)\n", f.name))
			buf.WriteString(fmt.Sprintf("\tui.%s.SetBounds(%s, %s, %s, %s)\n",
				f.name, fmtFloat(f.x), fmtFloat(f.y), fmtFloat(f.w), fmtFloat(f.h)))
		case f.parentAdd:
			// Nested in a simple-AddWidget container (VBox/HBox/Card/
			// GroupBox): AddWidget reparents AND lets the container
			// arrange it, so no explicit SetBounds is emitted.
			buf.WriteString(fmt.Sprintf("\tui.%s.AddWidget(ui.%s)\n", f.parentField, f.name))
		default:
			// Nested in a non-AddWidget container: reparent + absolute
			// bounds (relative to the parent's coordinate space).
			buf.WriteString(fmt.Sprintf("\tui.%s.SetParent(ui.%s)\n", f.name, f.parentField))
			buf.WriteString(fmt.Sprintf("\tui.%s.SetBounds(%s, %s, %s, %s)\n",
				f.name, fmtFloat(f.x), fmtFloat(f.y), fmtFloat(f.w), fmtFloat(f.h)))
		}
		// Wire event handlers based on widget type and user code.
		// When the user wrote a func body, pick the "natural" event
		// for the widget (Button → OnClick, Slider → OnValueChanged,
		// etc.) via defaultEventForFactory, then emit through the
		// shared emitEventBinding helper so this auto path stays in
		// lockstep with the explicit eventHandlers path below.
		handlerName := extractHandlerName(f.code)
		if handlerName != "" {
			if evt := defaultEventForFactory(f.factoryName); evt != "" {
				emitEventBinding(&buf, imports, f.factoryName, f.name, evt, handlerName)
			}
		}
		// Generate event handler bindings from the eventHandlers map
		if len(f.eventHandlers) > 0 && handlerName == "" {
			for evtName, handler := range f.eventHandlers {
				if !emitEventBinding(&buf, imports, f.factoryName, f.name, evtName, handler) {
					// Unknown (factory, event) pair. The codegen
					// switch is a hand-maintained table of
					// widget→signal mappings; new pairs need an
					// explicit entry. Emit guidance instead of a
					// silent miss so reviewers see the gap. The line
					// still compiles (it's a comment), so generated
					// files stay buildable — the binding just doesn't
					// fire until codegen learns the pair.
					buf.WriteString(fmt.Sprintf(
						"\t// codegen: no binding for %s.%s — add a case to ged/codegen.go's emitEventBinding.\n"+
							"\t//          Handler %q is not connected at runtime.\n",
						f.factoryName, evtName, handler))
				}
			}
		}
		buf.WriteString("\n")
	}

	// Set default text/content for widgets that have it
	for _, f := range fields {
		if f.defaultText != "" {
			switch {
			case strings.Contains(f.goType, "Button"):
				buf.WriteString(fmt.Sprintf("\tui.%s.SetText(%q)\n", f.name, f.defaultText))
			case strings.Contains(f.goType, "Label"):
				buf.WriteString(fmt.Sprintf("\tui.%s.SetText(%q)\n", f.name, f.defaultText))
			case strings.Contains(f.goType, "Edit"):
				buf.WriteString(fmt.Sprintf("\tui.%s.SetText(%q)\n", f.name, f.defaultText))
			case strings.Contains(f.goType, "CheckBox"):
				buf.WriteString(fmt.Sprintf("\tui.%s.SetText(%q)\n", f.name, f.defaultText))
			case strings.Contains(f.goType, "RadioButton"):
				buf.WriteString(fmt.Sprintf("\tui.%s.SetText(%q)\n", f.name, f.defaultText))
			case strings.Contains(f.goType, "GroupBox"):
				buf.WriteString(fmt.Sprintf("\tui.%s.SetTitle(%q)\n", f.name, f.defaultText))
			}
		}
	}
	buf.WriteString("\n")

	buf.WriteString("\treturn ui\n")
	buf.WriteString("}\n")

	// Generate main() function for runnable program
	if opts.PackageName == "main" {
		imports["silk/core"] = true
		buf.WriteString(fmt.Sprintf(`
func main() {
	ui := %s()
	ui.Form.AttachWindow(gui.WtForm)
	ui.Form.Window().SetIcon(nil)
	ui.Form.Window().MoveToCenter()
	ui.Form.Show()
	core.EventLoop()
}
`, constructorName))
	}

	// Append user-written event handler code
	hasHandlerCode := false
	for _, f := range fields {
		code := strings.TrimSpace(f.code)
		if code != "" {
			if !hasHandlerCode {
				buf.WriteString("\n// --- Event Handlers ---\n")
				hasHandlerCode = true
			}
			buf.WriteString("\n")
			buf.WriteString(code)
			buf.WriteString("\n")
		}
	}

	// Scan user event code for common stdlib imports
	allCode := buf.String()
	stdlibScan := map[string]string{
		"fmt.":     "fmt",
		"log.":     "log",
		"os.":      "os",
		"strings.": "strings",
		"strconv.": "strconv",
		"time.":    "time",
		"math.":    "math",
	}
	for prefix, pkg := range stdlibScan {
		if strings.Contains(allCode, prefix) {
			imports[pkg] = true
		}
	}

	// Rebuild imports string with all detected packages
	var result strings.Builder
	result.WriteString("// Code generated by Silk Designer Editor.\n")
	// When the caller wired in a module path (typically via
	// GenerateCodeWithMod feeding core.LoadGoMod into ModulePath),
	// surface it so the developer can tell at a glance which module
	// the generated file targets. First step toward full import-path
	// resolution; for now the comment is informational only.
	if opts.ModulePath != "" {
		result.WriteString(fmt.Sprintf("// Module: %s\n", opts.ModulePath))
	}
	result.WriteString(fmt.Sprintf("package %s\n\n", opts.PackageName))
	result.WriteString("import (\n")
	for imp := range imports {
		result.WriteString(fmt.Sprintf("\t%q\n", imp))
	}
	result.WriteString(")\n\n")

	// Get everything after the old import block
	full := buf.String()
	// Find struct definition start
	structIdx := strings.Index(full, fmt.Sprintf("type %s struct", opts.TypeName))
	if structIdx >= 0 {
		result.WriteString(full[structIdx:])
	}

	return result.String()
}

// GenerateCodeFile writes the generated code to a file.
func (scene *GedScene) GenerateCodeFile(filename string, opts CodeGenOptions) error {
	code := scene.GenerateCode(opts)
	return os.WriteFile(filename, []byte(code), 0644)
}

// GenerateCodeWithMod is a thin wrapper around GenerateCode that
// resolves the project's go.mod (walking up from projectDir) and
// fills opts.ModulePath when the caller hasn't set it explicitly.
// A missing or malformed go.mod is non-fatal — codegen falls back to
// the plain GenerateCode behaviour. This is the first step toward
// proper import-path resolution: today only the "// Module: <path>"
// comment is emitted.
func (scene *GedScene) GenerateCodeWithMod(projectDir string, opts CodeGenOptions) string {
	if opts.ModulePath == "" {
		if gm, err := core.LoadGoMod(projectDir); err == nil && gm != nil {
			opts.ModulePath = gm.Module
		}
	}
	return scene.GenerateCode(opts)
}

// sanitizeIdentifier makes a string safe for use as a Go identifier.
// It uppercases the first letter and removes non-alphanumeric characters.
func sanitizeIdentifier(s string) string {
	if s == "" {
		return "Widget"
	}
	var buf strings.Builder
	upper := true
	for _, r := range s {
		if r == '_' || r == '-' || r == ' ' || r == '.' {
			upper = true
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		if buf.Len() == 0 && unicode.IsDigit(r) {
			buf.WriteRune('X')
		}
		if upper {
			buf.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			buf.WriteRune(r)
		}
	}
	if buf.Len() == 0 {
		return "Widget"
	}
	return buf.String()
}

// roundPx converts mm scene coordinates to pixel values,
// using a standard 96 DPI conversion (1mm ≈ 3.78px).
func roundPx(mm float64) float64 {
	return math.Round(mm * 3.78) // 96 DPI: 25.4mm per inch, 96px per inch
}

// fmtFloat formats a float64 for code output: integers print without decimal.
func fmtFloat(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}

// emitEventBinding writes the Go code that wires `handler` to the
// signal slot for `evtName` on the FakeWidget `f`. Returns true when
// a binding was emitted; false when the (factory, event) pair has
// no entry in the table so the caller can emit guidance.
//
// Centralising the table here means the auto-default switch (the
// "user wrote a func; pick the obvious event for this widget" path)
// and the explicit eventHandlers switch (the "designer panel
// recorded an event handler" path) can share the same source of
// truth. New widget→signal mappings get added in one place rather
// than two parallel switches that drift over time.
//
// imports is the running import set the caller threads through; some
// bindings (e.g. ColorPicker) need to pull in extra packages and
// tag those here.
// defaultEventForFactory returns the "natural" event a widget gets
// auto-bound to when the user writes a single Go func and no
// explicit event metadata is recorded — Button's natural event is
// OnClick, a Slider's is OnValueChanged, and so on. Empty string
// means the factory has no auto-default; the user must record an
// explicit eventHandlers entry to bind anything.
//
// Co-located with emitEventBinding so both halves of the codegen's
// event story live in one file. New factories that gain an auto-
// default get one new line here plus the matching case in
// emitEventBinding — both checked by the existing tests.
func defaultEventForFactory(factoryName string) string {
	switch factoryName {
	case "gui.Button", "gui.Link":
		return "OnClick"
	case "gui.Edit":
		return "OnChanged"
	case "gui.CheckBox":
		return "OnToggled"
	case "gui.Slider", "gui.SpinBox", "gui.NumberInput":
		return "OnValueChanged"
	case "gui.RadioButton":
		return "OnChanged"
	case "gui.ToggleSwitch":
		return "OnToggle"
	case "gui.SearchBox":
		return "OnSearch"
	case "gui.Rating":
		return "OnRatingChanged"
	case "gui.DatePicker":
		return "OnDateChanged"
	case "gui.ColorPicker":
		return "OnColorChanged"
	case "gui.DropdownButton":
		return "OnSelect"
	case "gui.SwitchGroup":
		return "OnChange"
	case "gui.ComboBox", "gui.ListWidget", "gui.Table":
		return "OnSelectionChanged"
	case "gui.Tag":
		return "OnClose"
	case "gui.Breadcrumb":
		return "OnNavigate"
	case "gui.Accordion":
		return "OnSectionToggle"
	case "gui.NotificationPanel":
		return "OnItemClick"
	case "gui.TabWidget":
		return "OnTabChanged"
	case "gui.CodeEditor":
		return "OnTextChanged"
	}
	return ""
}

func emitEventBinding(buf *strings.Builder, imports map[string]bool, factoryName, fieldName, evtName, handler string) bool {
	// Locally-named aliases match the prior literal-source style in
	// the switch bodies — keeps the table grep-friendly when readers
	// search for "f.factoryName" / "f.name" in older codegen pulls.
	type _f = struct {
		factoryName string
		name        string
	}
	f := _f{factoryName: factoryName, name: fieldName}
	switch f.factoryName {
	case "gui.Button":
		if evtName == "OnClick" {
			fmt.Fprintf(buf, "\tui.%s.Action().BindFunc0(%s)\n", f.name, handler)
			return true
		}
	case "gui.Edit":
		if evtName == "OnChanged" {
			fmt.Fprintf(buf, "\tui.%s.SigTextChanged(func(_ interface{}, s string) { %s(s) })\n", f.name, handler)
			return true
		}
	case "gui.CheckBox":
		if evtName == "OnToggled" {
			fmt.Fprintf(buf, "\tui.%s.SigCheck(func(checked bool) { %s(checked) })\n", f.name, handler)
			return true
		}
	case "gui.Slider":
		if evtName == "OnValueChanged" {
			fmt.Fprintf(buf, "\tui.%s.SetValueChangedCallback(func(_ interface{}, v float64) { %s(v) })\n", f.name, handler)
			return true
		}
	case "gui.SpinBox":
		if evtName == "OnValueChanged" {
			fmt.Fprintf(buf, "\tui.%s.SetValueChangedCallback(func(_ interface{}, v int) { %s(v) })\n", f.name, handler)
			return true
		}
	case "gui.RadioButton":
		if evtName == "OnChanged" {
			fmt.Fprintf(buf, "\tui.%s.SetChangedCallback(func(_ interface{}, v bool) { %s(v) })\n", f.name, handler)
			return true
		}
	case "gui.ToggleSwitch":
		if evtName == "OnToggle" {
			fmt.Fprintf(buf, "\tui.%s.SigToggle(func(on bool) { %s(on) })\n", f.name, handler)
			return true
		}
	case "gui.SearchBox":
		if evtName == "OnSearch" {
			fmt.Fprintf(buf, "\tui.%s.SigSearch(func(q string) { %s(q) })\n", f.name, handler)
			return true
		}
		if evtName == "OnTextChanged" {
			fmt.Fprintf(buf, "\tui.%s.SigTextChanged(func(s string) { %s(s) })\n", f.name, handler)
			return true
		}
	case "gui.NumberInput":
		if evtName == "OnValueChanged" {
			fmt.Fprintf(buf, "\tui.%s.SigValueChanged(func(v float64) { %s(v) })\n", f.name, handler)
			return true
		}
	case "gui.Rating":
		if evtName == "OnRatingChanged" {
			fmt.Fprintf(buf, "\tui.%s.SigRatingChanged(func(v int) { %s(v) })\n", f.name, handler)
			return true
		}
	case "gui.DatePicker":
		if evtName == "OnDateChanged" {
			fmt.Fprintf(buf, "\tui.%s.SigDateChanged(func(y, m, d int) { %s(y, m, d) })\n", f.name, handler)
			return true
		}
	case "gui.ColorPicker":
		if evtName == "OnColorChanged" {
			imports["silk/paint"] = true
			fmt.Fprintf(buf, "\tui.%s.SigColorChanged(func(c paint.Color) { %s(c) })\n", f.name, handler)
			return true
		}
	case "gui.DropdownButton":
		if evtName == "OnSelect" {
			fmt.Fprintf(buf, "\tui.%s.SigSelect(func(idx int, text string) { %s(idx, text) })\n", f.name, handler)
			return true
		}
	case "gui.SwitchGroup":
		if evtName == "OnChange" {
			fmt.Fprintf(buf, "\tui.%s.SigChange(func(idx int, text string) { %s(idx, text) })\n", f.name, handler)
			return true
		}
	case "gui.Link":
		if evtName == "OnClick" {
			fmt.Fprintf(buf, "\tui.%s.SigClick(func(url string) { %s(url) })\n", f.name, handler)
			return true
		}
	case "gui.ComboBox":
		if evtName == "OnSelectionChanged" {
			fmt.Fprintf(buf, "\tui.%s.SigSelectionChanged(func(_ interface{}, idx int) { %s(idx) })\n", f.name, handler)
			return true
		}
	case "gui.ListWidget":
		if evtName == "OnSelectionChanged" {
			fmt.Fprintf(buf, "\tui.%s.SigSelectionChanged(func(_ interface{}, idx []int) { %s(idx) })\n", f.name, handler)
			return true
		}
	case "gui.Table":
		if evtName == "OnSelectionChanged" {
			fmt.Fprintf(buf, "\tui.%s.SetSelectionChangedCallback(func(_ interface{}, row int) { %s(row) })\n", f.name, handler)
			return true
		}
	case "gui.Tag":
		if evtName == "OnClose" {
			fmt.Fprintf(buf, "\tui.%s.SigClose(func() { %s() })\n", f.name, handler)
			return true
		}
	case "gui.Breadcrumb":
		if evtName == "OnNavigate" {
			fmt.Fprintf(buf, "\tui.%s.SigClick(func(idx int, item gui.BreadcrumbItem) { %s(idx, item.Text) })\n", f.name, handler)
			return true
		}
	case "gui.Accordion":
		if evtName == "OnSectionToggle" {
			fmt.Fprintf(buf, "\tui.%s.SigExpand(func(idx int, expanded bool) { %s(idx, expanded) })\n", f.name, handler)
			return true
		}
	case "gui.NotificationPanel":
		if evtName == "OnItemClick" {
			fmt.Fprintf(buf, "\tui.%s.SigClick(func(idx int) { %s(idx) })\n", f.name, handler)
			return true
		}
	case "gui.TabWidget":
		if evtName == "OnTabChanged" {
			fmt.Fprintf(buf, "\tui.%s.SetCurrentChangedCallback(func(_ interface{}, idx int) { %s(idx) })\n", f.name, handler)
			return true
		}
	case "gui.CodeEditor":
		if evtName == "OnTextChanged" {
			fmt.Fprintf(buf, "\tui.%s.SigChanged(%s)\n", f.name, handler)
			return true
		}
		if evtName == "OnClick" {
			fmt.Fprintf(buf, "\tui.%s.SigWidgetClicked(%s)\n", f.name, handler)
			return true
		}
	}
	return false
}

// extractHandlerName parses the function name from user-written Go event code.
// It looks for the first "func <name>(" declaration and returns the name.
func extractHandlerName(code string) string {
	for _, line := range strings.Split(code, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "func ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := parts[1]
				// Remove parameters
				if idx := strings.Index(name, "("); idx >= 0 {
					name = name[:idx]
				}
				return name
			}
		}
	}
	return ""
}
