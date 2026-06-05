package ged

import (
	"testing"

	"silk/graph"
)

// TestFakeWidgetNestedSaveLoadRoundTrip builds a scene with a VBox
// container holding two child widgets, serialises the whole scene via
// SaveDesign, reloads it into a fresh scene with LoadDesign, and
// asserts the nesting (container + its two children, names, factory
// types) survives the round-trip. Before the nesting change,
// FakeWidget.SaveDesign dropped its children entirely, so a laid-out
// container lost everything inside it on the next save.
func TestFakeWidgetNestedSaveLoadRoundTrip(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Nested")
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetWidgetName("box1")
	vbox.SetBounds(10, 10, 80, 60)
	vbox.SetParent(scene)

	childA, _ := NewFakeWidgetFromFactory("gui.Button")
	childA.SetWidgetName("btnA")
	childA.SetBounds(12, 14, 30, 8)
	childA.SetParent(vbox)

	childB, _ := NewFakeWidgetFromFactory("gui.Label")
	childB.SetWidgetName("lblB")
	childB.SetBounds(12, 26, 30, 6)
	childB.SetParent(vbox)

	// Serialise + reload into a fresh scene.
	doc := scene.SaveDesign()
	fresh := NewGedScene()
	if err := fresh.LoadDesign(doc); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}

	// The fresh scene should have exactly one top-level child (the VBox).
	top := fresh.Children()
	if len(top) != 1 {
		t.Fatalf("top-level children = %d, want 1 (the container)", len(top))
	}
	box, ok := top[0].(*FakeWidget)
	if !ok {
		t.Fatalf("top child is %T, want *FakeWidget", top[0])
	}
	if box.WidgetFactoryName() != "gui.VBox" {
		t.Errorf("container factory = %q, want gui.VBox", box.WidgetFactoryName())
	}
	if box.WidgetName() != "box1" {
		t.Errorf("container name = %q, want box1", box.WidgetName())
	}

	// The container must have reconstructed its two nested children.
	kids := box.Children()
	if len(kids) != 2 {
		t.Fatalf("container children = %d, want 2 (the nested widgets were lost)", len(kids))
	}

	names := map[string]string{} // name -> factory
	for _, k := range kids {
		fw, ok := k.(*FakeWidget)
		if !ok {
			t.Fatalf("nested child is %T, want *FakeWidget", k)
		}
		names[fw.WidgetName()] = fw.WidgetFactoryName()
	}
	if names["btnA"] != "gui.Button" {
		t.Errorf("nested btnA factory = %q, want gui.Button (got names: %v)", names["btnA"], names)
	}
	if names["lblB"] != "gui.Label" {
		t.Errorf("nested lblB factory = %q, want gui.Label (got names: %v)", names["lblB"], names)
	}
}

// TestFakeWidgetFlatSaveUnchanged confirms a widget with NO nested
// children emits no "children" block, so flat designs serialise
// exactly as before the nesting change (no spurious empty node).
func TestFakeWidgetFlatSaveUnchanged(t *testing.T) {
	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("solo")
	btn.SetBounds(1, 2, 10, 5)

	doc := btn.SaveDesign()
	if doc.ChildByKey("children", false) != nil {
		t.Error("a childless FakeWidget should not emit a 'children' block")
	}
}

// TestFakeWidgetDeepNesting verifies recursion survives more than one
// level (container inside container).
func TestFakeWidgetDeepNesting(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 200)

	outer, _ := NewFakeWidgetFromFactory("gui.VBox")
	outer.SetWidgetName("outer")
	outer.SetParent(scene)

	inner, _ := NewFakeWidgetFromFactory("gui.HBox")
	inner.SetWidgetName("inner")
	inner.SetParent(outer)

	leaf, _ := NewFakeWidgetFromFactory("gui.Button")
	leaf.SetWidgetName("leaf")
	leaf.SetParent(inner)

	doc := scene.SaveDesign()
	fresh := NewGedScene()
	if err := fresh.LoadDesign(doc); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}

	outerR := firstFake(t, fresh.Children())
	if outerR.WidgetName() != "outer" {
		t.Fatalf("level0 = %q, want outer", outerR.WidgetName())
	}
	innerR := firstFake(t, outerR.Children())
	if innerR.WidgetName() != "inner" || innerR.WidgetFactoryName() != "gui.HBox" {
		t.Fatalf("level1 = %q/%q, want inner/gui.HBox", innerR.WidgetName(), innerR.WidgetFactoryName())
	}
	leafR := firstFake(t, innerR.Children())
	if leafR.WidgetName() != "leaf" {
		t.Fatalf("level2 = %q, want leaf", leafR.WidgetName())
	}
}

func firstFake(t *testing.T, items []graph.IItem) *FakeWidget {
	t.Helper()
	if len(items) == 0 {
		t.Fatal("expected at least one child")
	}
	fw, ok := items[0].(*FakeWidget)
	if !ok {
		t.Fatalf("child is %T, want *FakeWidget", items[0])
	}
	return fw
}
