package ged

import (
	"github.com/uk0/silk/graph"
	"strings"
	"testing"
)

func TestGenerateCodeEmpty(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("TestForm")
	scene.SetSize(100, 80)

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "TestFormUI"})

	if !strings.Contains(code, "package main") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(code, "type TestFormUI struct") {
		t.Error("missing struct declaration")
	}
	if !strings.Contains(code, "func NewTestFormUI()") {
		t.Error("missing constructor")
	}
	if !strings.Contains(code, `Form.SetTitle("TestForm")`) {
		t.Error("missing form title")
	}
	if !strings.Contains(code, "func main()") {
		t.Error("missing main function")
	}
	if !strings.Contains(code, "core.EventLoop()") {
		t.Error("missing EventLoop call")
	}
}

func TestGenerateCodeWithWidgets(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("MyApp")
	scene.SetSize(120, 80)

	// Add a button
	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal("failed to create button:", err)
	}
	btn.SetWidgetName("btnOK")
	btn.SetBounds(5, 5, 25, 7)
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	// Add a label
	lbl, err := NewFakeWidgetFromFactory("gui.Label")
	if err != nil {
		t.Fatal("failed to create label:", err)
	}
	lbl.SetWidgetName("lblTitle")
	lbl.SetBounds(5, 15, 35, 5)
	cmd2 := graph.NewAddCommand()
	cmd2.AddItem(lbl, scene)
	scene.PushCommand(cmd2)

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "MyAppUI"})

	// Check struct fields
	if !strings.Contains(code, "BtnOK *gui.Button") {
		t.Error("missing BtnOK field")
	}
	if !strings.Contains(code, "LblTitle *gui.Label") {
		t.Error("missing LblTitle field")
	}

	// Check constructor creates widgets
	if !strings.Contains(code, "gui.NewButton1") {
		t.Error("missing button constructor")
	}
	if !strings.Contains(code, "gui.NewLabel") {
		t.Error("missing label constructor")
	}

	// Check SetParent calls
	if !strings.Contains(code, "BtnOK.SetParent(ui.Form)") {
		t.Error("missing SetParent for button")
	}

	// Check SetBounds with pixel coords (not mm)
	if !strings.Contains(code, "BtnOK.SetBounds(") {
		t.Error("missing SetBounds for button")
	}

	// Check default text is set
	if !strings.Contains(code, `SetText("Button")`) {
		t.Error("missing default text for button")
	}
}

func TestGenerateCodeWithEventHandler(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Test")
	scene.SetSize(100, 80)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("btn")
	btn.SetBounds(5, 5, 20, 7)
	btn.SetCode("func onBtnClick() {\n\tfmt.Println(\"clicked\")\n}")
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateCode(CodeGenOptions{})

	// Event handler code should be included
	if !strings.Contains(code, "func onBtnClick()") {
		t.Error("missing event handler")
	}
	if !strings.Contains(code, "Event Handlers") {
		t.Error("missing event handlers section")
	}

	// fmt should be auto-detected in imports
	if !strings.Contains(code, `"fmt"`) {
		t.Error("missing auto-detected fmt import")
	}
}

// TestGenerateCodeBindsCodeEditorChanged pins the *compilable*
// gui.CodeEditor codegen output: a concrete *gui.CodeEditor field
// (from factoryMap) plus the OnTextChanged → SigChanged binding.
// Before the factoryMap entry existed the field degraded to
// gui.IWidget while the switch still emitted SigChanged — a method
// gui.IWidget lacks — so the generated program failed to build. The
// binding must also not fall through to the "// codegen: no binding"
// guidance comment.
func TestGenerateCodeBindsCodeEditorChanged(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Editor")
	scene.SetSize(120, 80)

	ed, err := NewFakeWidgetFromFactory("gui.CodeEditor")
	if err != nil {
		t.Fatalf("create CodeEditor: %v", err)
	}
	ed.SetWidgetName("editor")
	ed.SetBounds(5, 5, 100, 60)
	ed.SetEventHandler("OnTextChanged", "onEditorTextChanged")
	cmd := graph.NewAddCommand()
	cmd.AddItem(ed, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateCode(CodeGenOptions{})

	// The field must be the concrete *gui.CodeEditor, not the gui.IWidget
	// interface fallback — that is what makes the SigChanged binding below
	// actually type-check. Asserting the concrete field pins the compilable
	// output, not merely the presence of the binding text.
	if !strings.Contains(code, "Editor *gui.CodeEditor") {
		t.Errorf("CodeEditor field degraded to interface; want concrete *gui.CodeEditor\n----\n%s", code)
	}
	if !strings.Contains(code, "ui.Editor = gui.NewCodeEditor()") {
		t.Errorf("CodeEditor not constructed via gui.NewCodeEditor()\n----\n%s", code)
	}
	if strings.Contains(code, `core.New("gui.CodeEditor")`) {
		t.Errorf("CodeEditor still using core.New interface fallback\n----\n%s", code)
	}
	if !strings.Contains(code, "ui.Editor.SigChanged(onEditorTextChanged)") {
		t.Errorf("missing CodeEditor.SigChanged binding\n----\n%s", code)
	}
	if strings.Contains(code, "// codegen: no binding") {
		t.Errorf("unknown-pair fall-through fired for known pair\n----\n%s", code)
	}
}

// TestGenerateCodeAutoDefaultRoutesThroughHelper: when the user
// writes a func body (no explicit eventHandlers map) the codegen
// auto-default path picks a widget-natural event via
// defaultEventForFactory and dispatches through emitEventBinding —
// the same helper the explicit eventHandlers path uses. This
// guards against the auto-default and eventHandlers tables drifting
// after they were unified.
//
// CodeEditor is the canary widget here because its auto-default
// (OnTextChanged → SigChanged) was added in the unification pass;
// a regression that broke the routing would surface as a missing
// SigChanged binding even though the eventHandlers path still
// works.
func TestGenerateCodeAutoDefaultRoutesThroughHelper(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Editor")
	scene.SetSize(120, 80)

	ed, err := NewFakeWidgetFromFactory("gui.CodeEditor")
	if err != nil {
		t.Fatalf("create CodeEditor: %v", err)
	}
	ed.SetWidgetName("editor")
	ed.SetBounds(5, 5, 100, 60)
	ed.SetCode("func onEditorChanged(s string) { _ = s }")
	cmd := graph.NewAddCommand()
	cmd.AddItem(ed, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateCode(CodeGenOptions{})
	if !strings.Contains(code, "ui.Editor.SigChanged(onEditorChanged)") {
		t.Errorf("auto-default CodeEditor binding missing\n----\n%s", code)
	}
}

// TestDefaultEventForFactoryHasEntryForEveryHandledFactory: every
// factory that emitEventBinding knows about should also have a
// defaultEventForFactory mapping (or be deliberately excluded).
// The unification relies on the auto-default switch picking SOME
// event — a factory without a default but with a handler in the
// eventHandlers table would silently lose its auto binding.
func TestDefaultEventForFactoryHasEntryForEveryHandledFactory(t *testing.T) {
	wantDefault := []string{
		"gui.Button", "gui.Edit", "gui.CheckBox", "gui.Slider",
		"gui.SpinBox", "gui.RadioButton", "gui.ToggleSwitch",
		"gui.SearchBox", "gui.NumberInput", "gui.Rating",
		"gui.DatePicker", "gui.ColorPicker", "gui.DropdownButton",
		"gui.SwitchGroup", "gui.Link", "gui.ComboBox",
		"gui.ListWidget", "gui.Table", "gui.Tag", "gui.Breadcrumb",
		"gui.Accordion", "gui.NotificationPanel", "gui.TabWidget",
		"gui.CodeEditor",
	}
	for _, f := range wantDefault {
		if got := defaultEventForFactory(f); got == "" {
			t.Errorf("defaultEventForFactory(%q) = \"\"; want a non-empty event", f)
		}
	}
}

// TestGenerateCodeUnknownEventEmitsGuidance: an event name that
// isn't in the codegen switch falls through to the new guidance
// comment instead of the old terse "TODO: bind X.Y -> Z" line. The
// guidance points at where to extend the table.
func TestGenerateCodeUnknownEventEmitsGuidance(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Misc")
	scene.SetSize(60, 60)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	btn.SetWidgetName("btn")
	btn.SetBounds(0, 0, 30, 8)
	btn.SetEventHandler("OnDragStart", "onBtnDrag") // not in codegen switch
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateCode(CodeGenOptions{})

	for _, want := range []string{
		"codegen: no binding for gui.Button.OnDragStart",
		`Handler "onBtnDrag" is not connected at runtime`,
		"add a case to ged/codegen.go",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %q\n----\n%s", want, code)
		}
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"button", "Button"},
		{"my_button", "MyButton"},
		{"my-widget", "MyWidget"},
		{"btn OK", "BtnOK"},
		{"123abc", "X123abc"},
		{"", "Widget"},
		{"a.b.c", "ABC"},
	}
	for _, tt := range tests {
		got := sanitizeIdentifier(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRoundPx(t *testing.T) {
	// 5mm at 96 DPI should be ~19px
	px := roundPx(5.0)
	if px < 18 || px > 20 {
		t.Errorf("roundPx(5.0) = %v, expected ~19", px)
	}

	// 0mm = 0px
	if roundPx(0) != 0 {
		t.Errorf("roundPx(0) = %v, expected 0", roundPx(0))
	}
}
