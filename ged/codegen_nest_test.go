package ged

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestGenerateCodeNestedContainer builds a scene with a VBox holding
// two child widgets and asserts the generated source: (1) declares a
// struct field for the container AND each nested child, (2) parents the
// children via the container's AddWidget (not SetParent(ui.Form)), and
// (3) parses as valid Go. Before the nesting change, codegen flattened
// every widget onto the form and never emitted AddWidget.
func TestGenerateCodeNestedContainer(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Nested")
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetWidgetName("box")
	vbox.SetBounds(10, 10, 80, 60)
	vbox.SetParent(scene)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("btnA")
	btn.SetBounds(12, 14, 30, 8)
	btn.SetParent(vbox)

	lbl, _ := NewFakeWidgetFromFactory("gui.Label")
	lbl.SetWidgetName("lblB")
	lbl.SetBounds(12, 26, 30, 6)
	lbl.SetParent(vbox)

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "NestedUI"})

	// Field names are exported-capitalised by sanitizeIdentifier
	// (box→Box, btnA→BtnA, lblB→LblB). Struct fields exist for the
	// container AND both nested children.
	for _, want := range []string{
		"Box *gui.VBox",
		"BtnA *gui.Button",
		"LblB *gui.Label",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("missing struct field %q\n--- code ---\n%s", want, code)
		}
	}

	// The container is parented to the form...
	if !strings.Contains(code, "ui.Box.SetParent(ui.Form)") {
		t.Errorf("container should parent to the form\n--- code ---\n%s", code)
	}
	// ...and the children are added INTO the container, not the form.
	if !strings.Contains(code, "ui.Box.AddWidget(ui.BtnA)") {
		t.Errorf("btnA should be added to the container via AddWidget\n--- code ---\n%s", code)
	}
	if !strings.Contains(code, "ui.Box.AddWidget(ui.LblB)") {
		t.Errorf("lblB should be added to the container via AddWidget\n--- code ---\n%s", code)
	}
	// Children must NOT be re-parented to the form.
	if strings.Contains(code, "ui.BtnA.SetParent(ui.Form)") {
		t.Errorf("btnA was flattened onto the form instead of nested\n--- code ---\n%s", code)
	}

	// The whole thing must be valid Go.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "nested.go", code, 0); err != nil {
		t.Fatalf("generated source did not parse: %v\n--- code ---\n%s", err, code)
	}
}

// TestGenerateCodeNonAddContainerFallsBack verifies that a container
// NOT in simpleAddContainers (e.g. a plain widget acting as parent)
// gets SetParent + SetBounds for its children rather than AddWidget,
// keeping the generated code compilable for any parent type. We use a
// GridLayout parent (excluded from simpleAddContainers because its
// AddWidget needs row/col).
func TestGenerateCodeNonAddContainerFallsBack(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)

	grid, err := NewFakeWidgetFromFactory("gui.GridLayout")
	if err != nil {
		t.Fatalf("create GridLayout: %v", err)
	}
	grid.SetWidgetName("grid")
	grid.SetBounds(10, 10, 100, 80)
	grid.SetParent(scene)

	cell, _ := NewFakeWidgetFromFactory("gui.Button")
	cell.SetWidgetName("cellBtn")
	cell.SetBounds(12, 14, 30, 8)
	cell.SetParent(grid)

	code := scene.GenerateCode(CodeGenOptions{TypeName: "GridUI"})

	// GridLayout's single-arg AddWidget doesn't exist, so the child must
	// use SetParent(ui.Grid) + SetBounds, NOT AddWidget. (Names are
	// exported-capitalised: grid→Grid, cellBtn→CellBtn.)
	if !strings.Contains(code, "ui.CellBtn.SetParent(ui.Grid)") {
		t.Errorf("grid child should SetParent to the grid\n--- code ---\n%s", code)
	}
	if strings.Contains(code, "ui.Grid.AddWidget(ui.CellBtn)") {
		t.Errorf("grid child must NOT use single-arg AddWidget (needs row/col)\n--- code ---\n%s", code)
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "grid.go", code, 0); err != nil {
		t.Fatalf("generated source did not parse: %v\n--- code ---\n%s", err, code)
	}
}
