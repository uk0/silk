package ged

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"go/token"
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

// Code panel tab indices. The panel holds two views of the same design: the
// handler source the user writes, and the file the generator produces from it.
const (
	codeTabHandler = 0
	codeTabGen     = 1
)

// codeRegenDelayMs is the debounce window between a design change and the
// regeneration it triggers. A drag emits a command per gesture and a drop
// fires the scene's attach signal on top of it; regenerating on each would
// re-run the whole walk several times for one user action.
const codeRegenDelayMs = 200

// CodePanel provides a code editing panel for writing Go event handler code.
// When a widget is selected on the GED canvas, the panel displays an editable
// code template for that widget's events, similar to Qt Creator's signal/slot
// code editor.
//
// Its second tab (生成代码) shows the full generated file for the active design,
// read-only, refreshed from the design's own change signals.
type CodePanel struct {
	gui.Widget
	tabs     *gui.SwitchGroup // 事件代码 / 生成代码
	titleBar *gui.Label       // shows which widget's code is being edited
	editor   *gui.CodeEditor  // syntax-highlighted code editor
	genView  *generatedView   // read-only view of the generated file
	saveBtn  *gui.Button
	fmtBtn   *gui.Button

	currentWidget         *FakeWidget
	gedView               *GedView
	selectionSubscription int
	designSubscription    int
	draftSubscription     int
	loadedRevision        uint64
	loadedText            string
	loadedCode            string

	genTimer   gui.Timer
	genLines   map[*FakeWidget]int
	genPending bool // a design change the 生成代码 view has not caught up with
	// onRegenForTest counts regenerations. The debounce's whole purpose is
	// collapsing a burst into one run, and "how many times did it run" cannot
	// be read off the resulting text — five identical regenerations and one
	// leave the same view.
	onRegenForTest func()
}

func NewCodePanel() *CodePanel {
	p := new(CodePanel)
	p.Init(p)
	return p
}

func (this *CodePanel) Init(self gui.IWidget) {
	this.Widget.Init(self)

	// Tab strip
	this.tabs = gui.NewSwitchGroup()
	this.tabs.SetParent(self)
	this.tabs.SetItems([]string{"事件代码", "生成代码"})
	this.tabs.SigChange(func(idx int, _ string) {
		this.ShowTab(idx)
	})

	// Title label
	this.titleBar = gui.NewLabel("选择控件查看代码")
	this.titleBar.SetParent(self)
	this.titleBar.SetFont(paint.NewFont(gui.Theme().Font.Family(), 12, true, false))

	// Syntax-highlighted code editor
	this.editor = gui.NewCodeEditor()
	this.editor.SetParent(self)
	this.editor.SetFont(paint.NewFont(gui.Theme().Font.Family(), 13, false, false))
	this.editor.SetText("// 在此编写事件处理代码\n")
	this.editor.SigChanged(func(string) { this.refreshTitle() })

	// Generated-file view, hidden until its tab is picked
	this.genView = newGeneratedView()
	this.genView.SetParent(self)
	this.genView.SetFont(paint.NewFont(gui.Theme().Font.Family(), 13, false, false))
	this.genView.SetText("// 打开或新建设计后在此查看生成的代码\n")
	this.genView.SetVisible(false)

	// Save button
	this.saveBtn = gui.NewButton1("保存代码", nil)
	this.saveBtn.SetParent(self)
	this.saveBtn.Action().BindFunc0(func() {
		this.SaveCode()
	})

	// Format button (bounded background formatting)
	this.fmtBtn = gui.NewButton1("Format", nil)
	this.fmtBtn.SetParent(self)
	this.fmtBtn.Action().BindFunc0(func() {
		this.editor.RequestFormat()
	})
}

func (this *CodePanel) Title() string {
	return "代码"
}

func (this *CodePanel) Layout() {
	w, h := this.Size()
	w, h = max(0, w), max(0, h)
	header := this.headerHeight()
	tabsW := min(150.0, max(0, w-8))
	buttonY := 2.0
	if header > 28 {
		buttonY = 30
	}
	btnW := min(80.0, max(0, (w-12)/2))
	fmtW := min(64.0, max(0, w-btnW-12))
	left := max(4, w-btnW-fmtW-8)
	titleW := max(0, left-tabsW-14)
	if header > 28 {
		titleW = max(0, w-tabsW-14)
	}
	this.tabs.SetBounds(min(4, w), min(2, h), tabsW, min(24, max(0, h-2)))
	this.titleBar.SetBounds(min(w, tabsW+10), min(2, h), titleW, min(24, max(0, h-2)))
	this.fmtBtn.SetBounds(min(w, left), min(buttonY, h), fmtW, min(24, max(0, h-buttonY)))
	this.saveBtn.SetBounds(min(w, max(0, w-btnW-4)), min(buttonY, h), btnW, min(24, max(0, h-buttonY)))
	this.editor.SetBounds(0, min(header, h), w, max(0, h-header))
	this.genView.SetBounds(0, min(header, h), w, max(0, h-header))
}

func (this *CodePanel) headerHeight() float64 {
	if this.Width() < 326 {
		return 56
	}
	return 28
}

// ShowTab brings one of the two views forward. 保存代码/Format belong to the
// handler tab only — the generated file has nothing to save them into.
func (this *CodePanel) ShowTab(idx int) {
	gen := idx == codeTabGen
	this.tabs.SetSelected(idx)
	this.editor.SetVisible(!gen)
	this.genView.SetVisible(gen)
	this.saveBtn.SetVisible(!gen)
	this.fmtBtn.SetVisible(!gen)
	this.refreshTitle()
	if gen {
		if err := this.CommitPendingChanges(); err != nil {
			this.genView.SetText("// " + err.Error() + "\n")
			return
		}
	}
	if gen && this.genPending {
		// Regeneration is skipped while this tab is behind the other one, so
		// coming forward is where it catches up with everything it missed.
		this.RegenerateNow()
	}
	this.Layout()
	this.Self().Update()
}

func (this *CodePanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Background
	g.SetBrush1(gui.Theme().FormColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Top bar background
	g.SetBrush1(gui.Theme().FormColor)
	header := min(h, this.headerHeight())
	g.Rectangle(0, 0, w, header)
	g.Fill()

	// Separator
	g.SetPen1(gui.Theme().BorderColor, 0)
	g.MoveTo(0, header)
	g.LineTo(w, header)
	g.Stroke()
}

// BindGedView connects the code panel to a GedView so that selecting a widget
// on the canvas automatically loads its event handler code into the editor,
// and so that the 生成代码 view follows that design.
func (this *CodePanel) BindGedView(gv *GedView) {
	if this.gedView == gv {
		return
	}
	if err := this.CommitPendingChanges(); err != nil {
		core.Warn(err)
		return
	}
	if old := this.gedView; old != nil {
		old.RemoveSelectionListener(this.selectionSubscription)
		if old.eventCodePanel == this {
			old.eventCodePanel = nil
		}
		if scene := old.GedScene(); scene != nil {
			scene.RemoveDesignChangedCallback(this.designSubscription)
			scene.RemovePendingEditor(this.draftSubscription)
		}
	}
	this.genTimer.Stop()
	this.currentWidget = nil
	this.gedView = gv
	if gv != nil {
		gv.eventCodePanel = this
	}
	this.SetWidget(nil)
	this.genLines = nil
	this.genPending = true
	if gv == nil {
		this.RegenerateNow()
		return
	}

	// Every mutation that can be undone reaches the generated view from here:
	// property-sheet edits, moves, resizes, drops, reparents, morphs. The
	// scene's attach/detach slots are NOT bound — they hold one callback each
	// and the status bar owns them (design.go bindStatusBarTo), which chains
	// this panel's refresh onto them instead.
	if scene := gv.GedScene(); scene != nil {
		this.draftSubscription = scene.AddPendingEditor(this)
		this.designSubscription = scene.AddDesignChangedCallback(func() { this.syncWidgetCode(); this.ScheduleRegenerate() })
	}

	this.selectionSubscription = gv.AddSelectionListener(func(items []graph.IItem) {
		if len(items) == 1 {
			if fake, ok := items[0].(*FakeWidget); ok {
				this.SetWidget(fake)
				this.ScrollToWidget(fake)
				return
			}
		}
		this.SetWidget(nil)
	})

	this.ScheduleRegenerate()
}

// ScheduleRegenerate arms the debounce timer. Start() re-arms rather than
// stacks, so a burst of design changes collapses into one regeneration.
func (this *CodePanel) ScheduleRegenerate() {
	this.genPending = true
	if this.tabs.Selected() != codeTabGen {
		// Nothing is showing the generated code; ShowTab regenerates when the
		// tab comes forward. Walking the whole design behind a hidden view on
		// every nudge of a drag is exactly the cost the debounce exists to avoid.
		return
	}
	this.genTimer.Start(codeRegenDelayMs, func() {
		// gui.Timer repeats; this one is a one-shot delay.
		this.genTimer.Stop()
		this.RegenerateNow()
	})
}

// RegenerateNow rebuilds the generated view from the bound design immediately.
func (this *CodePanel) RegenerateNow() {
	if this.onRegenForTest != nil {
		this.onRegenForTest()
	}
	this.genPending = false
	scene := this.scene()
	if scene == nil {
		this.genLines = nil
		this.refreshTitle()
		this.genView.SetText("// 打开或新建设计后在此查看生成的代码\n")
		this.Self().Update()
		return
	}

	gen := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main"})
	this.genLines = gen.Lines
	this.refreshTitle()
	if gen.Err != nil {
		// A stale-but-green listing is worse than an honest error: the user
		// would take it for the code their design produces.
		this.genView.SetText("// " + gen.Err.Error() + "\n")
	} else {
		this.genView.SetText(gen.Code)
	}
	this.Self().Update()
	if this.currentWidget != nil {
		this.ScrollToWidget(this.currentWidget)
	}
}

// ScrollToWidget brings the line that constructs fake into view. The line comes
// from the generation walk, so it survives a widget whose name also appears in
// a comment or a handler body.
func (this *CodePanel) ScrollToWidget(fake *FakeWidget) {
	if fake == nil || this.tabs.Selected() != codeTabGen {
		return
	}
	if line, ok := this.genLines[fake]; ok {
		this.genView.ScrollToLine(line)
	}
}

// scene returns the design the panel is bound to, or nil.
func (this *CodePanel) scene() *GedScene {
	if this.gedView == nil {
		return nil
	}
	return this.gedView.GedScene()
}

// refreshTitle writes the title bar for whichever tab is forward. One label
// serves both, so every path that changes what is shown goes through here.
func (this *CodePanel) refreshTitle() {
	if this.tabs.Selected() == codeTabGen {
		if scene := this.scene(); scene != nil {
			this.titleBar.SetText("生成代码: " + scene.FormTitle())
		} else {
			this.titleBar.SetText("生成代码")
		}
		return
	}
	if this.currentWidget == nil {
		this.titleBar.SetText("选择控件查看代码")
		return
	}
	name := this.currentWidget.WidgetName()
	if name == "" {
		name = this.currentWidget.WidgetFactoryName()
	}
	if this.HasPendingChanges() {
		name += " *"
	}
	this.titleBar.SetText("代码: " + name)
}

// SetWidget shows the code for a specific widget.
func (this *CodePanel) SetWidget(fake *FakeWidget) {
	// Repeated selection notifications must not overwrite unsaved handler text.
	if fake != nil && this.currentWidget == fake {
		this.syncWidgetCode()
		return
	}
	// Auto-save previous widget's code before switching
	if this.currentWidget != nil && this.currentWidget != fake {
		if err := this.CommitPendingChanges(); err != nil {
			core.Warn(err)
			return
		}
	}

	this.currentWidget = fake
	if fake == nil {
		this.refreshTitle()
		this.editor.SetText("// 选择画布上的控件以编辑事件代码\n")
		this.loadedText = this.editor.Text()
		this.loadedRevision = this.editor.TextRevision()
		this.Self().Update()
		return
	}

	this.refreshTitle()

	// Load existing code or generate template
	code := fake.GetCode()
	if code == "" {
		code = this.generateTemplate(fake)
	}
	this.editor.SetText(code)
	this.loadedText = code
	this.loadedRevision = this.editor.TextRevision()
	this.loadedCode = fake.GetCode()
	this.refreshTitle()
	this.Self().Update()
}

// SaveCode persists the editor content back to the FakeWidget.
func (this *CodePanel) SaveCode() {
	if err := this.commitCode(); err != nil {
		core.Warn(err)
	}
}

// HasPendingChanges excludes an untouched suggested template.
func (this *CodePanel) HasPendingChanges() bool {
	if this.currentWidget == nil || this.editor.TextRevision() == this.loadedRevision {
		return false
	}
	if this.editor.Text() == this.loadedText {
		this.loadedRevision = this.editor.TextRevision()
		return false
	}
	return true
}

func (this *CodePanel) CommitPendingChanges() error {
	if !this.HasPendingChanges() {
		return nil
	}
	return this.commitCode()
}

func (this *CodePanel) commitCode() error {
	if this.currentWidget == nil {
		return nil
	}
	text := this.editor.Text()
	if this.HasPendingChanges() && this.currentWidget.GetCode() != this.loadedCode && text != this.currentWidget.GetCode() {
		return fmt.Errorf("事件代码已在其他位置更改；未覆盖代码面板中的草稿")
	}
	scene := this.scene()
	if scene != nil && !scene.containsWidget(this.currentWidget) {
		return fmt.Errorf("事件代码所属控件已离开当前设计；草稿仍保留在代码面板")
	}
	if text != this.currentWidget.GetCode() {
		// Advance the baseline before notifying listeners. Otherwise our own
		// command re-enters the pending-edit flush or overwrites local undo state.
		this.loadedText, this.loadedCode = text, text
		this.loadedRevision = this.editor.TextRevision()
		if scene != nil {
			scene.SceneItem.PushCommand(&handlerCodeCommand{widget: this.currentWidget, before: this.currentWidget.GetCode(), after: text})
			scene.NotifyDesignChanged()
		} else {
			this.currentWidget.SetCode(text)
		}
	}
	this.loadedText, this.loadedCode = text, text
	this.loadedRevision = this.editor.TextRevision()
	this.refreshTitle()
	return nil
}

type handlerCodeCommand struct {
	widget        *FakeWidget
	before, after string
}

func (c *handlerCodeCommand) Redo()        { c.widget.SetCode(c.after) }
func (c *handlerCodeCommand) Undo()        { c.widget.SetCode(c.before) }
func (c *handlerCodeCommand) Text() string { return "编辑事件代码" }

// Undo/redo can change the model while this panel remains selected. Refresh a
// clean editor, but never overwrite locally edited text with a model update.
func (this *CodePanel) syncWidgetCode() {
	fake := this.currentWidget
	if fake == nil || this.editor.Text() != this.loadedText || fake.GetCode() == this.loadedCode {
		return
	}
	code := fake.GetCode()
	if code == "" {
		code = this.generateTemplate(fake)
	}
	pos := this.editor.NavigationPosition()
	this.editor.SetText(code)
	this.editor.ScrollToLineCol(pos.Line, pos.Column)
	this.loadedText, this.loadedCode = code, fake.GetCode()
	this.loadedRevision = this.editor.TextRevision()
}

// Focus activates the code editor and brings it into focus.
func (this *CodePanel) Focus() {
	this.editor.SetFocus()
	this.Self().Update()
}

// ScrollToHandler scrolls the editor to the handler for the given widget name.
func (this *CodePanel) ScrollToHandler(name string) {
	// Try to find a function matching "on<Name>" or a comment "// <name>".
	// The method form comes first — it is what generateTemplate writes and what
	// the export owns — with the free-function form kept for designs saved
	// before the split.
	targets := []string{
		") on" + capitalize(name),
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
	event := defaultEventForFactory(fake.WidgetFactoryName())
	def, ok := eventDefinition(fake.WidgetFactoryName(), event)
	if !ok {
		return "// 此控件没有可生成的事件处理函数。\n"
	}
	name := fake.EventHandlers()[event]
	if name == "" {
		name = defaultHandlerName(fake, event)
	}
	if !token.IsIdentifier(name) || name == "_" {
		return "// 无效的事件处理函数名称。\n"
	}
	recv := "func "
	if scene := this.scene(); scene != nil {
		recv = handlerReceiver(defaultCodeGenOptions(scene, CodeGenOptions{}).TypeName)
	}
	return fmt.Sprintf("%s%s(%s) {\n\t// TODO: implement %s.\n}\n", recv, name, def.params, event)
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

// HandlerEditor exposes the editable embedded-handler buffer to SDK hosts.
func (this *CodePanel) HandlerEditor() *gui.CodeEditor { return this.editor }

func (this *CodePanel) DiscardPendingChanges() {
	if this.currentWidget == nil {
		return
	}
	this.editor.SetText(this.loadedText)
	this.loadedRevision = this.editor.TextRevision()
	this.syncWidgetCode()
	this.refreshTitle()
}
