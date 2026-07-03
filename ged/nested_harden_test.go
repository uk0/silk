package ged

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
)

// helper: make a scene-level fake widget.
func sceneWidget(t *testing.T, scene *GedScene, factory, name string, x, y, w, h float64) *FakeWidget {
	t.Helper()
	fw, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("create %s: %v", factory, err)
	}
	fw.SetWidgetName(name)
	fw.SetBounds(x, y, w, h)
	fw.SetParent(scene)
	return fw
}

// TestLayOutSelectionSkipsLocked: a position-locked item is excluded
// from the layout, so selecting one locked + one unlocked widget leaves
// fewer than two layout-eligible items and the op no-ops (no container
// created, nothing reparented).
func TestLayOutSelectionSkipsLocked(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	a := sceneWidget(t, scene, "gui.Button", "a", 10, 10, 30, 8)
	b := sceneWidget(t, scene, "gui.Button", "b", 10, 30, 30, 8)
	a.SetLocked(true) // pins position → excluded from layout

	sel := view.Selection()
	sel.Clear()
	sel.Add(a)
	sel.Add(b)
	view.layOutSelection(false)

	if n := len(scene.Children()); n != 2 {
		t.Fatalf("scene children = %d, want 2 (no container — locked item left only 1 eligible)", n)
	}
	for _, c := range scene.Children() {
		if c.(*FakeWidget).WidgetFactoryName() == "gui.VBox" {
			t.Error("a container was created despite a locked item leaving <2 eligible")
		}
	}
}

// TestLayOutSelectionSkipsSceneRoot: if the scene root is somehow part
// of the selection it must never be reparented — only the real widgets
// get wrapped.
func TestLayOutSelectionSkipsSceneRoot(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	a := sceneWidget(t, scene, "gui.Button", "a", 10, 10, 30, 8)
	b := sceneWidget(t, scene, "gui.Button", "b", 10, 30, 30, 8)

	sel := view.Selection()
	sel.Clear()
	sel.Add(graph.IItem(scene)) // pathological: scene root in the selection
	sel.Add(a)
	sel.Add(b)
	view.layOutSelection(false)

	// Scene root survives at top with one new container holding a + b.
	top := scene.Children()
	if len(top) != 1 {
		t.Fatalf("scene top-level = %d, want 1 container", len(top))
	}
	box := top[0].(*FakeWidget)
	if box.WidgetFactoryName() != "gui.VBox" || len(box.Children()) != 2 {
		t.Errorf("container = %q with %d children, want gui.VBox with 2 (scene root must not be wrapped)",
			box.WidgetFactoryName(), len(box.Children()))
	}
}

// TestLayOutSelectionSkipsNestedAncestor: selecting a container, one of
// its own children, AND a third scene widget must lay out only the
// container + the third widget — the nested child is dropped (its
// ancestor is selected), so it stays inside its container rather than
// being double-moved into the new one.
func TestLayOutSelectionSkipsNestedAncestor(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	vbox := sceneWidget(t, scene, "gui.VBox", "vbox", 10, 10, 40, 30)
	child, _ := NewFakeWidgetFromFactory("gui.Button")
	child.SetWidgetName("child")
	child.SetBounds(12, 14, 20, 6)
	child.SetParent(vbox) // nested inside vbox
	other := sceneWidget(t, scene, "gui.Button", "other", 80, 10, 30, 8)

	sel := view.Selection()
	sel.Clear()
	sel.Add(vbox)
	sel.Add(child) // ancestor (vbox) also selected → must be dropped
	sel.Add(other)
	view.layOutSelection(false)

	// New container wraps vbox + other (2), NOT child.
	top := scene.Children()
	if len(top) != 1 {
		t.Fatalf("scene top-level = %d, want 1 (the new container)", len(top))
	}
	newBox := top[0].(*FakeWidget)
	if len(newBox.Children()) != 2 {
		t.Fatalf("new container children = %d, want 2 (vbox + other, not the nested child)", len(newBox.Children()))
	}
	// child must still be inside its original vbox, not double-moved.
	if len(vbox.Children()) != 1 || vbox.Children()[0].(*FakeWidget).WidgetName() != "child" {
		t.Errorf("nested child was double-moved out of its container")
	}
}

// TestSelectAllRecursesIntoContainers: Ctrl+A must select widgets nested
// inside layout containers, not just scene-level items.
func TestSelectAllRecursesIntoContainers(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	vbox := sceneWidget(t, scene, "gui.VBox", "vbox", 10, 10, 40, 30)
	child, _ := NewFakeWidgetFromFactory("gui.Button")
	child.SetWidgetName("child")
	child.SetParent(vbox)

	view.selectAll()

	found := false
	for _, it := range view.Selection().ItemList() {
		if fw, ok := it.(*FakeWidget); ok && fw.WidgetName() == "child" {
			found = true
		}
	}
	if !found {
		t.Error("selectAll did not select the widget nested inside the container")
	}
}

// TestGenerateCodeUniqueFieldNames: two widgets that resolve to the same
// field identifier must be disambiguated so the generated struct never
// declares duplicate fields (which would not compile).
func TestGenerateCodeUniqueFieldNames(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	// Two buttons explicitly given the SAME name → collision.
	a := sceneWidget(t, scene, "gui.Button", "dup", 10, 10, 30, 8)
	b := sceneWidget(t, scene, "gui.Button", "dup", 10, 30, 30, 8)
	_ = a
	_ = b

	code := scene.GenerateCode(CodeGenOptions{TypeName: "DupUI"})

	// Must compile: no duplicate identifiers.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "dup.go", code, 0); err != nil {
		t.Fatalf("generated source did not parse (likely duplicate field): %v\n--- code ---\n%s", err, code)
	}
	// The first keeps "Dup"; the second is suffixed.
	if !strings.Contains(code, "Dup ") || !strings.Contains(code, "Dup_2 ") {
		t.Errorf("expected fields Dup and Dup_2 (collision disambiguation)\n--- code ---\n%s", code)
	}
}

// TestObjectInspectorShowsNesting: a widget nested in a container appears
// in the inspector at a deeper indent level, not hidden.
func TestObjectInspectorShowsNesting(t *testing.T) {
	scene := NewGedScene()
	vbox := sceneWidget(t, scene, "gui.VBox", "vbox", 10, 10, 40, 30)
	child, _ := NewFakeWidgetFromFactory("gui.Button")
	child.SetWidgetName("child")
	child.SetParent(vbox)

	insp := NewObjectInspector()
	insp.SetScene(scene)
	insp.Rebuild()

	var childDepth = -1
	for _, it := range insp.items {
		if it.name == "child" {
			childDepth = it.depth
		}
	}
	if childDepth < 0 {
		t.Fatal("nested child is missing from the object inspector")
	}
	if childDepth != 2 {
		t.Errorf("nested child depth = %d, want 2 (scene=0, vbox=1, child=2)", childDepth)
	}
}

// TestGenerateRuntimeNestsChildren: the runtime preview build
// (scene.Generate) must populate a container with its nested children,
// matching the generated code — not leave it empty.
func TestGenerateRuntimeNestsChildren(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	vbox := sceneWidget(t, scene, "gui.VBox", "box", 10, 10, 40, 30)
	child, _ := NewFakeWidgetFromFactory("gui.Button")
	child.SetWidgetName("child")
	child.SetBounds(12, 14, 20, 6)
	child.SetParent(vbox)

	design := scene.Generate()
	w := design.index["box"]
	if w == nil {
		t.Fatal("container widget not found in the generated design index")
	}
	if len(w.Children()) != 1 {
		t.Errorf("runtime container has %d children, want 1 (preview flattened the nesting)", len(w.Children()))
	}
}
