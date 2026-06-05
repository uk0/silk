package ged

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestBuildFakeDeclNodeNests verifies the declarative codegen tree
// mirrors the designer's nesting: a VBox containing two widgets yields
// a decl node whose Children are those two widgets. Tests the node
// builder directly (no string matching) so the structure is checked
// exactly.
func TestBuildFakeDeclNodeNests(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetWidgetName("box")
	vbox.SetParent(scene)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("btnA")
	btn.SetParent(vbox)

	lbl, _ := NewFakeWidgetFromFactory("gui.Label")
	lbl.SetWidgetName("lblB")
	lbl.SetParent(vbox)

	root := buildSceneDeclNode(scene)

	// Scene root has exactly one child node: the VBox.
	if len(root.Children) != 1 {
		t.Fatalf("scene decl children = %d, want 1 (the container)", len(root.Children))
	}
	boxNode := root.Children[0]
	if boxNode.Type != "gui.VBox" || boxNode.ID != "box" {
		t.Fatalf("container node = %q/%q, want gui.VBox/box", boxNode.Type, boxNode.ID)
	}

	// The VBox node nests both widgets as decl children.
	if len(boxNode.Children) != 2 {
		t.Fatalf("container decl children = %d, want 2 (nesting lost)", len(boxNode.Children))
	}
	ids := map[string]string{} // id -> type
	for _, ch := range boxNode.Children {
		ids[ch.ID] = ch.Type
	}
	if ids["btnA"] != "gui.Button" {
		t.Errorf("nested btnA type = %q, want gui.Button (got %v)", ids["btnA"], ids)
	}
	if ids["lblB"] != "gui.Label" {
		t.Errorf("nested lblB type = %q, want gui.Label (got %v)", ids["lblB"], ids)
	}
}

// TestGenerateDeclCodeNestedParses confirms the full decl-mode source
// for a nested scene still parses as valid Go and mentions all three
// widget builders.
func TestGenerateDeclCodeNestedParses(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Nested")
	scene.SetSize(200, 150)

	vbox, _ := NewFakeWidgetFromFactory("gui.VBox")
	vbox.SetWidgetName("box")
	vbox.SetParent(scene)
	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("btnA")
	btn.SetParent(vbox)

	code := scene.GenerateDeclCode(CodeGenOptions{TypeName: "Nested"})

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "nested_decl.go", code, 0); err != nil {
		t.Fatalf("decl source did not parse: %v\n--- code ---\n%s", err, code)
	}
	for _, want := range []string{`decl.ID("box")`, `decl.ID("btnA")`} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %q\n--- code ---\n%s", want, code)
		}
	}
}
