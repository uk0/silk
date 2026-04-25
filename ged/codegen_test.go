package ged

import (
	"silk/graph"
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
