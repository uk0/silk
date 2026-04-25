package ged

import (
	"silk/graph"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	// Create scene with widgets
	scene := NewGedScene()
	scene.SetFormTitle("RoundTrip")
	scene.SetSize(120, 90)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal("failed to create button:", err)
	}
	btn.SetWidgetName("myButton")
	btn.SetBounds(10, 10, 30, 8)
	btn.SetCode("func onTest() {}\n")
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	// Save
	doc := scene.SaveDesign()
	if doc == nil {
		t.Fatal("SaveDesign returned nil")
	}

	// Create new scene and load
	scene2 := NewGedScene()
	err = scene2.LoadDesign(doc)
	if err != nil {
		t.Fatal("LoadDesign failed:", err)
	}

	// Verify title
	if scene2.FormTitle() != "RoundTrip" {
		t.Errorf("title = %q, want %q", scene2.FormTitle(), "RoundTrip")
	}

	// Verify bounds
	_, _, w2, h2 := scene2.Bounds()
	if w2 != 120 || h2 != 90 {
		t.Errorf("scene2 size = (%f, %f), want (120, 90)", w2, h2)
	}

	// Verify children
	children := scene2.Children()
	if len(children) != 1 {
		t.Fatalf("children count = %d, want 1", len(children))
	}

	fake, ok := children[0].(*FakeWidget)
	if !ok {
		t.Fatalf("child type = %T, want *FakeWidget", children[0])
	}
	if fake.WidgetName() != "myButton" {
		t.Errorf("widget name = %q, want %q", fake.WidgetName(), "myButton")
	}
	if fake.GetCode() != "func onTest() {}\n" {
		t.Errorf("code = %q, want %q", fake.GetCode(), "func onTest() {}\n")
	}
}

func TestSaveLoadMultipleWidgets(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("MultiWidget")
	scene.SetSize(200, 150)

	widgets := []struct {
		factory string
		name    string
		x, y    float64
	}{
		{"gui.Button", "btn1", 10, 10},
		{"gui.Label", "lbl1", 10, 25},
		{"gui.Edit", "edit1", 10, 40},
		{"gui.CheckBox", "cb1", 10, 55},
		{"gui.Slider", "sld1", 10, 70},
	}

	for _, w := range widgets {
		fake, err := NewFakeWidgetFromFactory(w.factory)
		if err != nil {
			t.Fatalf("failed to create %s: %v", w.factory, err)
		}
		fake.SetWidgetName(w.name)
		fake.SetBounds(w.x, w.y, 30, 8)
		cmd := graph.NewAddCommand()
		cmd.AddItem(fake, scene)
		scene.PushCommand(cmd)
	}

	// Save and load
	doc := scene.SaveDesign()
	scene2 := NewGedScene()
	err := scene2.LoadDesign(doc)
	if err != nil {
		t.Fatal("LoadDesign failed:", err)
	}

	children := scene2.Children()
	if len(children) != len(widgets) {
		t.Fatalf("children count = %d, want %d", len(children), len(widgets))
	}

	// Verify names preserved
	for i, child := range children {
		fake, ok := child.(*FakeWidget)
		if !ok {
			t.Fatalf("child %d type = %T, want *FakeWidget", i, child)
		}
		if fake.WidgetName() != widgets[i].name {
			t.Errorf("child %d name = %q, want %q", i, fake.WidgetName(), widgets[i].name)
		}
	}
}

func TestSaveLoadPreservesEventHandlers(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Events")
	scene.SetSize(100, 80)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("btnAction")
	btn.SetBounds(5, 5, 25, 7)
	btn.SetEventHandler("OnClick", "handleClick")
	btn.SetEventHandler("OnHover", "handleHover")

	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	doc := scene.SaveDesign()
	scene2 := NewGedScene()
	err := scene2.LoadDesign(doc)
	if err != nil {
		t.Fatal("LoadDesign failed:", err)
	}

	children := scene2.Children()
	if len(children) != 1 {
		t.Fatalf("children count = %d, want 1", len(children))
	}

	fake := children[0].(*FakeWidget)
	handlers := fake.EventHandlers()
	if handlers == nil {
		t.Fatal("event handlers not restored")
	}
	if handlers["OnClick"] != "handleClick" {
		t.Errorf("OnClick handler = %q, want %q", handlers["OnClick"], "handleClick")
	}
	if handlers["OnHover"] != "handleHover" {
		t.Errorf("OnHover handler = %q, want %q", handlers["OnHover"], "handleHover")
	}
}

func TestSaveLoadEmptyScene(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Empty")
	scene.SetSize(80, 60)

	doc := scene.SaveDesign()
	scene2 := NewGedScene()
	err := scene2.LoadDesign(doc)
	if err != nil {
		t.Fatal("LoadDesign of empty scene failed:", err)
	}

	if scene2.FormTitle() != "Empty" {
		t.Errorf("title = %q, want %q", scene2.FormTitle(), "Empty")
	}
	children := scene2.Children()
	if len(children) != 0 {
		t.Errorf("expected 0 children, got %d", len(children))
	}
}

func TestSaveLoadPreservesLockState(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Lock")
	scene.SetSize(100, 80)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("locked_btn")
	btn.SetBounds(5, 5, 25, 7)
	btn.SetLocked(true)

	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	doc := scene.SaveDesign()
	scene2 := NewGedScene()
	err := scene2.LoadDesign(doc)
	if err != nil {
		t.Fatal(err)
	}

	fake := scene2.Children()[0].(*FakeWidget)
	if !fake.IsLocked() {
		t.Error("lock state not preserved after save/load")
	}
}
