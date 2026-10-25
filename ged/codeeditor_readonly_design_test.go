package ged

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func readonlyDesignFixture(t *testing.T) *GedScene {
	t.Helper()
	scene := NewGedScene()
	scene.SetFormTitle("ReadOnlyQA")
	scene.SetSize(240, 150)
	fake := addNamedWidget(t, scene, "gui.CodeEditor", "Viewer", 10)
	fake.SetBounds(10, 10, 210, 120)
	ed := fake.Widget().(*gui.CodeEditor)
	ed.SetText("// generated preview 中🙂\npackage demo\n")
	ed.SetReadOnly(true)
	return scene
}

func TestReadOnlyEditorDesignerRoundTripPreviewAndCodegen(t *testing.T) {
	scene := readonlyDesignFixture(t)
	fake := scene.Children()[0].(*FakeWidget)
	doc := fake.SaveDesign()
	for i := 0; i < 3; i++ {
		parsed, err := core.LoadTDocStr(doc.String())
		if err != nil {
			t.Fatal(err)
		}
		next, err := NewFakeWidgetFromFactory("gui.CodeEditor")
		if err != nil {
			t.Fatal(err)
		}
		if err := next.LoadDesign(parsed); err != nil {
			t.Fatal(err)
		}
		for _, w := range []gui.IWidget{next.Widget(), next.Generate()} {
			ed := w.(*gui.CodeEditor)
			if !ed.IsReadOnly() || ed.Text() != fake.Widget().(*gui.CodeEditor).Text() {
				t.Fatal("read-only property/text lost")
			}
			before := ed.Text()
			ed.DuplicateLines()
			ed.OnTextInput("blocked")
			if ed.Text() != before {
				t.Fatal("preview editable")
			}
		}
		doc = next.SaveDesign()
	}
	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "ReadOnlyQAUI"})
	if !strings.Contains(code, "ui.Viewer.SetReadOnly(true)") {
		t.Fatal("codegen lost read-only", code)
	}
	if !testing.Short() {
		vetGeneratedCode(t, code)
	}
}

func TestReadOnlyEditorWriteQAFixture(t *testing.T) {
	dir := os.Getenv("SILK_READONLY_QA_DIR")
	if dir == "" {
		t.Skip("set SILK_READONLY_QA_DIR for native QA fixture")
	}
	scene := readonlyDesignFixture(t)
	path := filepath.Join(dir, "workspace", "readonly.silkui")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	scene.SetFilename(path)
	if !scene.Save() {
		t.Fatal("fixture save")
	}
	t.Log(path)
}
