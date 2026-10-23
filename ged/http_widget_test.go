package ged

import (
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
)

func TestHTTPWidgetPaletteDropPersistencePreviewAndGeneration(t *testing.T) {
	palette := NewWidgetList()
	found := false
	for i := 0; i < palette.Count(); i++ {
		if palette.Item(i).Data == "gui.HTTPButton" {
			found = true
		}
	}
	if !found {
		t.Fatal("HTTP widget missing from palette")
	}
	v := NewGedView()
	v.GedScene().SetSize(240, 160)
	x, y := v.MapFromScene(10, 10)
	v.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.HTTPButton"})
	children := v.Scene().Children()
	if len(children) != 1 {
		t.Fatal("drop failed")
	}
	fake := children[0].(*FakeWidget)
	b := fake.Widget().(*gui.HTTPButton)
	b.SetText("Fetch count")
	b.SetURL("http://127.0.0.1:8080/api?name=中文")
	b.SetMethod("post")
	b.SetHeadersJSON(`{"Content-Type":"application/json"}`)
	b.SetBody(`{"name":"中文😀"}`)
	b.SetResultType("int")
	b.SetJSONPointer("/items/0/count")
	b.SetTimeoutMS(900)
	b.SetMaxResponseKB(512)
	b.SetHeader("Authorization", "runtime-only-secret")
	fake.SetWidgetName("request")
	doc := fake.SaveDesign()
	if strings.Contains(doc.String(), "runtime-only-secret") {
		t.Fatal("runtime credentials persisted")
	}
	dst, err := NewFakeWidgetFromFactory("gui.HTTPButton")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := core.LoadTDocStr(doc.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := dst.LoadDesign(parsed); err != nil {
		t.Fatal(err)
	}
	want := captureWidgetProperties(b)
	for _, w := range []gui.IWidget{dst.Widget(), dst.Generate()} {
		if got := captureWidgetProperties(w.(core.IEnumProperties)); !reflect.DeepEqual(got, want) {
			t.Fatalf("properties lost: %v / %v", got, want)
		}
	}
	panel := NewCodePanel()
	panel.BindGedView(v)
	defer panel.BindGedView(nil)
	for _, event := range widgetEvents["gui.HTTPButton"] {
		if err := panel.OpenEventHandler(fake, event.name); err != nil {
			t.Fatal(event.name, err)
		}
	}
	generated := v.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "probe", TypeName: "HTTPTool"})
	if generated.Err != nil {
		t.Fatal(generated.Err)
	}
	for _, want := range []string{"gui.NewHTTPButton()", "SetURL(", "SetMethod(\"POST\")", "SetHeadersJSON(", "SetBody(", "SetResultType(\"int\")", "SetJSONPointer(", "SetTimeoutMS(900)", "SetMaxResponseKB(512)", "SigIntResult(", "SigFloatResult(", "SigBoolResult(", "SigResult(", "SigError(", "SigBusyChanged("} {
		if !strings.Contains(generated.Code, want) {
			t.Fatal("missing generated API", want)
		}
	}
	if strings.Contains(generated.Code, "runtime-only-secret") {
		t.Fatal("runtime credentials generated")
	}
	if !testing.Short() {
		vetGeneratedFiles(t, map[string]string{"main.go": generated.Code})
	}
}
func TestDeclarativeHTTPButtonBuild(t *testing.T) {
	tree := decl.HTTPButton(decl.ID("request"), decl.P("URL", "https://example.invalid/api"), decl.P("Result Type", "json"), decl.P("JSON Pointer", "/data"))
	built, err := tree.Build()
	if err != nil {
		t.Fatal(err)
	}
	b, ok := built.(*gui.HTTPButton)
	if !ok || b.URL() != "https://example.invalid/api" || b.ResultType() != "json" || b.JSONPointer() != "/data" || b.IsBusy() {
		t.Fatalf("bad declarative control %T", built)
	}
}
