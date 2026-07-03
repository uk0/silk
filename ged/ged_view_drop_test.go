package ged

import (
	"testing"

	"github.com/uk0/silk/graph"
)

// TestIsContainerItem verifies the drop-into-container predicate: every
// layout/AddWidget/window container reports true, leaf widgets report false,
// and a non-FakeWidget (the scene root) reports false. This is the headless
// core of the drag-drop nesting decision in OnDrop.
func TestIsContainerItem(t *testing.T) {
	containers := []string{
		"gui.VBox", "gui.HBox", "gui.GridLayout", "gui.FormLayout",
		"gui.Card", "gui.GroupBox", "gui.Accordion", "gui.StackedWidget",
		"gui.TabWidget", "gui.Splitter", "gui.ScrollArea", "gui.Form", "gui.Dialog",
	}
	for _, name := range containers {
		fw, err := NewFakeWidgetFromFactory(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if !isContainerItem(fw) {
			t.Errorf("isContainerItem(%s) = false, want true", name)
		}
	}

	leaves := []string{"gui.Button", "gui.Label", "gui.Edit", "gui.CheckBox"}
	for _, name := range leaves {
		fw, err := NewFakeWidgetFromFactory(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if isContainerItem(fw) {
			t.Errorf("isContainerItem(%s) = true, want false", name)
		}
	}

	// The scene root is not a FakeWidget → never a container.
	if isContainerItem(NewGedScene()) {
		t.Error("isContainerItem(scene) = true, want false")
	}
}

// TestContainerUnderPoint builds a scene with a VBox holding a Button and
// asserts the container hit-test finds the VBox both when the drop lands on
// the nested Button (walk up from the leaf) and on the container's own body,
// and returns nil over empty canvas (so OnDrop falls back to the scene root).
func TestContainerUnderPoint(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(10, 10, 80, 60)
	vbox.SetParent(scene)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetBounds(20, 20, 25, 7)
	btn.SetParent(vbox)

	// Drop over the nested Button → walk up to the containing VBox.
	if got := nearestContainerAncestor(scene.FindItemAt(25, 24, nil)); got != graph.IItem(vbox) {
		t.Errorf("container under nested button = %v, want vbox", got)
	}
	// Drop over the VBox body (not on the Button) → the VBox itself.
	if got := nearestContainerAncestor(scene.FindItemAt(12, 12, nil)); got != graph.IItem(vbox) {
		t.Errorf("container under vbox body = %v, want vbox", got)
	}
	// Drop over empty canvas → nil (fall back to scene root).
	if got := nearestContainerAncestor(scene.FindItemAt(150, 130, nil)); got != nil {
		t.Errorf("container under empty canvas = %v, want nil", got)
	}
}
