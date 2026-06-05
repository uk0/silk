package ged

import (
	"testing"
)

// TestLayOutUndoRedo: Lay Out is undoable. After laying two widgets into
// a VBox, Undo restores the original flat scene; Redo re-creates the
// container with both children.
func TestLayOutUndoRedo(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	a := sceneWidget(t, scene, "gui.Button", "a", 10, 10, 30, 8)
	b := sceneWidget(t, scene, "gui.Button", "b", 10, 30, 30, 8)

	sel := view.Selection()
	sel.Clear()
	sel.Add(a)
	sel.Add(b)
	view.layOutSelection(false)

	// Applied: one container holding both.
	if len(scene.Children()) != 1 {
		t.Fatalf("after layout, scene top-level = %d, want 1", len(scene.Children()))
	}

	stack := scene.UndoStack()
	if !stack.CanUndo() {
		t.Fatal("layout did not push an undoable command")
	}

	// Undo → flat scene again (a and b back at top level, no container).
	stack.Undo()
	top := scene.Children()
	if len(top) != 2 {
		t.Fatalf("after undo, scene top-level = %d, want 2 (flat restored)", len(top))
	}
	for _, c := range top {
		if c.(*FakeWidget).WidgetFactoryName() == "gui.VBox" {
			t.Error("container should be gone after undo")
		}
	}

	// Redo → container with both children back.
	if !stack.CanRedo() {
		t.Fatal("cannot redo after undo")
	}
	stack.Redo()
	top = scene.Children()
	if len(top) != 1 {
		t.Fatalf("after redo, scene top-level = %d, want 1 (container)", len(top))
	}
	box := top[0].(*FakeWidget)
	if box.WidgetFactoryName() != "gui.VBox" || len(box.Children()) != 2 {
		t.Errorf("after redo, container = %q with %d children, want gui.VBox/2",
			box.WidgetFactoryName(), len(box.Children()))
	}
}

// TestBreakLayoutUndoRedo: Break Layout is undoable. After dissolving a
// container, Undo restores the container with its children; Redo
// dissolves it again.
func TestBreakLayoutUndoRedo(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	vbox := sceneWidget(t, scene, "gui.VBox", "box", 10, 10, 40, 40)
	a, _ := NewFakeWidgetFromFactory("gui.Button")
	a.SetWidgetName("a")
	a.SetBounds(12, 14, 30, 8)
	a.SetParent(vbox)
	b, _ := NewFakeWidgetFromFactory("gui.Label")
	b.SetWidgetName("b")
	b.SetBounds(12, 26, 30, 6)
	b.SetParent(vbox)

	sel := view.Selection()
	sel.Clear()
	sel.Add(vbox)
	view.breakLayoutSelection()

	// Applied: flat (container gone, a + b at top).
	if len(scene.Children()) != 2 {
		t.Fatalf("after break, scene top-level = %d, want 2", len(scene.Children()))
	}

	stack := scene.UndoStack()
	if !stack.CanUndo() {
		t.Fatal("break did not push an undoable command")
	}

	// Undo → container restored with both children.
	stack.Undo()
	top := scene.Children()
	if len(top) != 1 {
		t.Fatalf("after undo, scene top-level = %d, want 1 (container restored)", len(top))
	}
	box := top[0].(*FakeWidget)
	if box.WidgetFactoryName() != "gui.VBox" {
		t.Fatalf("restored item = %q, want gui.VBox", box.WidgetFactoryName())
	}
	if len(box.Children()) != 2 {
		t.Errorf("restored container has %d children, want 2", len(box.Children()))
	}

	// Redo → dissolved again.
	stack.Redo()
	if len(scene.Children()) != 2 {
		t.Errorf("after redo, scene top-level = %d, want 2 (dissolved again)", len(scene.Children()))
	}
}

// TestLayOutThenBreakUndoChain: a Lay Out followed by a Break pushes two
// commands; undoing both walks back through Break then Lay Out to the
// original flat scene.
func TestLayOutThenBreakUndoChain(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	a := sceneWidget(t, scene, "gui.Button", "a", 10, 10, 30, 8)
	b := sceneWidget(t, scene, "gui.Button", "b", 10, 30, 30, 8)
	_ = a
	_ = b

	sel := view.Selection()
	sel.Clear()
	sel.Add(a)
	sel.Add(b)
	view.layOutSelection(false) // group
	view.breakLayoutSelection() // ungroup (container is selected after layout)

	// Net effect is flat again.
	if len(scene.Children()) != 2 {
		t.Fatalf("after layout+break, scene top-level = %d, want 2", len(scene.Children()))
	}

	stack := scene.UndoStack()
	// Undo the Break → container with 2 children.
	stack.Undo()
	if len(scene.Children()) != 1 {
		t.Fatalf("after undo break, scene top-level = %d, want 1", len(scene.Children()))
	}
	// Undo the Lay Out → flat again.
	stack.Undo()
	if len(scene.Children()) != 2 {
		t.Fatalf("after undo layout, scene top-level = %d, want 2 (original)", len(scene.Children()))
	}
}
