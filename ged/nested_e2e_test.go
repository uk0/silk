package ged

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestNestedContainerEndToEnd exercises the full layout-container
// pipeline across all four pieces — designer Lay Out, persistence,
// imperative codegen — proving they compose:
//
//  1. Add two widgets to a designer scene and select them.
//  2. Lay Out vertically  -> they get wrapped in a VBox container.
//  3. SaveDesign -> LoadDesign into a fresh scene -> nesting survives.
//  4. GenerateCode on the reloaded scene -> the children are emitted
//     via the container's AddWidget and the source parses as valid Go.
func TestNestedContainerEndToEnd(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("E2E")
	scene.SetSize(200, 150)

	a, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create button: %v", err)
	}
	a.SetWidgetName("ok")
	a.SetBounds(10, 10, 30, 8)
	a.SetParent(scene)

	b, _ := NewFakeWidgetFromFactory("gui.Button")
	b.SetWidgetName("cancel")
	b.SetBounds(10, 30, 30, 8)
	b.SetParent(scene)

	// Step 2: select both and lay out vertically.
	sel := view.Selection()
	sel.Clear()
	sel.Add(a)
	sel.Add(b)
	view.layOutSelection(false)

	if n := len(scene.Children()); n != 1 {
		t.Fatalf("after layout, scene top-level = %d, want 1 container", n)
	}

	// Step 3: round-trip through save/load.
	doc := scene.SaveDesign()
	reloaded := NewGedScene()
	if err := reloaded.LoadDesign(doc); err != nil {
		t.Fatalf("LoadDesign: %v", err)
	}
	top := reloaded.Children()
	if len(top) != 1 {
		t.Fatalf("reloaded top-level = %d, want 1 container", len(top))
	}
	box := top[0].(*FakeWidget)
	if box.WidgetFactoryName() != "gui.VBox" {
		t.Fatalf("reloaded container = %q, want gui.VBox", box.WidgetFactoryName())
	}
	if len(box.Children()) != 2 {
		t.Fatalf("reloaded container children = %d, want 2", len(box.Children()))
	}

	// Step 4: codegen on the reloaded scene nests the children.
	code := reloaded.GenerateCode(CodeGenOptions{TypeName: "E2EUI"})
	if !strings.Contains(code, ".AddWidget(ui.Ok)") || !strings.Contains(code, ".AddWidget(ui.Cancel)") {
		t.Errorf("codegen did not nest children via AddWidget\n--- code ---\n%s", code)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "e2e.go", code, 0); err != nil {
		t.Fatalf("generated source did not parse: %v\n--- code ---\n%s", err, code)
	}
}
