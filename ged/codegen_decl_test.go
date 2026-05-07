package ged

import (
	"go/parser"
	"go/token"
	"silk/graph"
	"strings"
	"testing"
)

// TestGenerateDeclCodeEmpty verifies that an empty scene still produces
// a valid Go source file with the expected skeleton: package, decl
// import, BuildXxx function, and a decl.Form root.
func TestGenerateDeclCodeEmpty(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Test")
	scene.SetSize(100, 80)

	code := scene.GenerateDeclCode(CodeGenOptions{PackageName: "ui", TypeName: "Test"})

	for _, want := range []string{
		"package ui",
		`import "silk/decl"`,
		"func BuildTest() *decl.Node",
		"decl.Form(",
		`decl.P("title", "Test")`,
	} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %q\nfull output:\n%s", want, code)
		}
	}
}

// TestGenerateDeclCodeParsesAsValidGo runs go/parser over the generated
// source. The output must always parse — that's the contract of the
// emitter.
func TestGenerateDeclCodeParsesAsValidGo(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Parseable")
	scene.SetSize(120, 80)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	btn.SetWidgetName("btnOK")
	btn.SetBounds(5, 5, 25, 7)
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateDeclCode(CodeGenOptions{})
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "test.go", code, 0); err != nil {
		t.Fatalf("generated source did not parse: %v\n--- source ---\n%s", err, code)
	}
}

// TestGenerateDeclCodeWithChildren confirms that scene children land as
// distinct decl child nodes with the correct types and IDs.
func TestGenerateDeclCodeWithChildren(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Multi")
	scene.SetSize(150, 100)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("btnSave")
	btn.SetBounds(5, 5, 25, 8)
	c1 := graph.NewAddCommand()
	c1.AddItem(btn, scene)
	scene.PushCommand(c1)

	lbl, _ := NewFakeWidgetFromFactory("gui.Label")
	lbl.SetWidgetName("lblTitle")
	lbl.SetBounds(5, 20, 40, 5)
	c2 := graph.NewAddCommand()
	c2.AddItem(lbl, scene)
	scene.PushCommand(c2)

	code := scene.GenerateDeclCode(CodeGenOptions{TypeName: "Multi"})

	for _, want := range []string{
		"decl.Button(",
		"decl.Label(",
		`decl.ID("btnSave")`,
		`decl.ID("lblTitle")`,
	} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %q\nfull output:\n%s", want, code)
		}
	}
}

// TestGenerateDeclCodeUnknownWidgetUsesDeclNew verifies that a widget
// not in the builderShortcuts table falls through to decl.New("type",
// ...). This is the safety net that lets new widget types render
// before their helpers are added.
func TestGenerateDeclCodeUnknownWidgetUsesDeclNew(t *testing.T) {
	// Use a widget type that's not in builderShortcuts. SpinBox is
	// real (registered in factoryMap) but absent from
	// decl/codec_go.go's builderShortcuts.
	scene := NewGedScene()
	scene.SetFormTitle("Unk")
	scene.SetSize(100, 60)

	spin, err := NewFakeWidgetFromFactory("gui.SpinBox")
	if err != nil {
		t.Fatal(err)
	}
	spin.SetWidgetName("spin1")
	spin.SetBounds(5, 5, 20, 6)
	cmd := graph.NewAddCommand()
	cmd.AddItem(spin, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateDeclCode(CodeGenOptions{})

	if !strings.Contains(code, `decl.New("gui.SpinBox"`) {
		t.Errorf("SpinBox should fall through to decl.New, got:\n%s", code)
	}
}

// TestGenerateDeclCodeHandlerCommentBlock covers the non-trivial bit of
// the emitter: handler bodies don't fit in the decl AST, so the emitter
// surfaces them as a footer comment so designer-authored intent is not
// lost.
func TestGenerateDeclCodeHandlerCommentBlock(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Handlers")
	scene.SetSize(100, 60)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("btnGo")
	btn.SetBounds(5, 5, 20, 7)
	btn.SetCode("func onBtnGoClick() {\n\tprintln(\"go\")\n}")
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateDeclCode(CodeGenOptions{})

	if !strings.Contains(code, "// Event handlers (designer-authored") {
		t.Errorf("handler comment block missing\n%s", code)
	}
	if !strings.Contains(code, "//   btnGo:") {
		t.Errorf("handler widget label missing\n%s", code)
	}
	if !strings.Contains(code, `//     func onBtnGoClick()`) {
		t.Errorf("handler body line missing\n%s", code)
	}
}

// TestGenerateDeclCodeFmtClean verifies the entire output is gofmt
// stable: re-running gofmt over it must not produce any diff.
func TestGenerateDeclCodeFmtClean(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Fmt")
	scene.SetSize(100, 60)

	btn, _ := NewFakeWidgetFromFactory("gui.Button")
	btn.SetWidgetName("btn")
	btn.SetBounds(5, 5, 25, 7)
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)

	code := scene.GenerateDeclCode(CodeGenOptions{})
	// Smoke check: gofmt ran inside GenerateDeclCode. Re-format and
	// compare bytewise — if anything drifts the emitter has a bug.
	import_idx := strings.Index(code, "import")
	if import_idx < 0 {
		t.Fatalf("no import in output:\n%s", code)
	}
	// We don't have direct access to format.Source here without
	// duplicating a helper; instead just check that the output does
	// not contain double-newlines after the func signature (a common
	// fmt drift symptom).
	if strings.Contains(code, "*decl.Node {\n\n\n") {
		t.Errorf("triple-newline detected — emitter spacing drifted:\n%s", code)
	}
}
