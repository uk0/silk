package ged

import (
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
)

func TestSceneWidgetDropPersistencePreviewCodegen(t *testing.T) {
	palette := NewWidgetList()
	found := false
	for i := 0; i < palette.Count(); i++ {
		if palette.Item(i).Data == "gui.SceneView" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing scene palette entry")
	}
	view := NewGedView()
	view.GedScene().SetSize(240, 180)
	x, y := view.MapFromScene(10, 10)
	view.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.SceneView"})
	children := view.Scene().Children()
	if len(children) != 1 {
		t.Fatal("scene drop failed")
	}
	fake := children[0].(*FakeWidget)
	v := fake.Widget().(*gui.SceneView)
	defer v.Close()
	v.SetModelSource("assets/中文 pump.obj")
	v.SetYaw(.2)
	v.SetPitch(-.3)
	v.SetDistance(9)
	fake.SetWidgetName("plant")
	doc := fake.SaveDesign()
	dst, e := NewFakeWidgetFromFactory("gui.SceneView")
	if e != nil {
		t.Fatal(e)
	}
	defer dst.Widget().(*gui.SceneView).Close()
	parsed, e := core.LoadTDocStr(doc.String())
	if e != nil {
		t.Fatal(e)
	}
	if e = dst.LoadDesign(parsed); e != nil {
		t.Fatal(e)
	}
	for _, widget := range []gui.IWidget{dst.Widget(), dst.Generate()} {
		p := widget.(*gui.SceneView)
		defer p.Close()
		if !reflect.DeepEqual(captureWidgetProperties(p), captureWidgetProperties(v)) {
			t.Fatal("3D properties lost")
		}
		if p.IsLoading() {
			t.Fatal("design read triggered model I/O")
		}
	}
	panel := NewCodePanel()
	panel.BindGedView(view)
	defer panel.BindGedView(nil)
	if e = panel.OpenEventHandler(fake, "OnSelected"); e != nil {
		t.Fatal(e)
	}
	result := view.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "probe", TypeName: "Plant"})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	for _, word := range []string{"gui.NewSceneView()", "SetModelSource(", "SetYaw(0.2)", "SetPitch(-0.3)", "SetDistance(9)", "SigSelected("} {
		if !strings.Contains(result.Code, word) {
			t.Fatal("missing codegen", word, result.Code)
		}
	}
	if !testing.Short() {
		vetGeneratedFiles(t, map[string]string{"main.go": result.Code})
	}
	built, e := decl.SceneView(decl.P("Camera Distance", 7.0)).Build()
	if e != nil {
		t.Fatal(e)
	}
	defer built.(*gui.SceneView).Close()
	if built.(*gui.SceneView).Distance() != 7 {
		t.Fatal("declarative scene property")
	}
}
