package ged

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
)

func TestJSONWidgetPaletteDropPersistenceAndGeneratedEvent(t *testing.T) {
	palette := NewWidgetList()
	found := false
	for i := 0; i < palette.Count(); i++ {
		if palette.Item(i).Data == "gui.JSONView" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing JSON palette control")
	}
	v := NewGedView()
	v.GedScene().SetSize(240, 160)
	x, y := v.MapFromScene(10, 10)
	v.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.JSONView"})
	children := v.Scene().Children()
	if len(children) != 1 {
		t.Fatal("drop failed")
	}
	fake := children[0].(*FakeWidget)
	fake.SetWidgetName("inspector")
	view := fake.Widget().(*gui.JSONView)
	sample := `{"data":{"count":9007199254740993,"name":"中文😀"}}`
	view.SetJSONText(sample)
	view.SetPageSize(32)
	view.SetRootPointer("/data")
	fake.SetEventHandler("OnSelected", "onSelected")
	fake.SetCode(`func onSelected(pointer string) { selectedPath = pointer }`)
	doc := fake.SaveDesign()
	dst, err := NewFakeWidgetFromFactory("gui.JSONView")
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
	for _, w := range []gui.IWidget{dst.Widget(), dst.Generate()} {
		got := captureWidgetProperties(w.(core.IEnumProperties))
		if !reflect.DeepEqual(got, captureWidgetProperties(view)) {
			t.Fatalf("persisted properties differ: %v", got)
		}
	}
	if err := view.LoadJSON(`{"data":{"secret":"runtime-only-value"}}`); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fake.SaveDesign().String(), "runtime-only-value") {
		t.Fatal("runtime payload persisted")
	}
	pair, err := v.GedScene().GenerateCodeFile(filepath.Join(t.TempDir(), "view.go"), CodeGenOptions{PackageName: "ui", TypeName: "InspectorUI"})
	if err != nil {
		t.Fatal(err)
	}
	machine, _ := os.ReadFile(pair.MachineFile)
	for _, want := range []string{"gui.NewJSONView()", "SetJSONText(", "SetPageSize(32)", "SetRootPointer(\"/data\")", "SigSelected(ui.onSelected)"} {
		if !strings.Contains(string(machine), want) {
			t.Fatal("missing generated API", want)
		}
	}
	if testing.Short() {
		return
	}
	check := filepath.Join(filepath.Dir(pair.UserFile), "runtime_test.go")
	if err := os.WriteFile(check, []byte(`package ui
import "testing"
var selectedPath string
func TestJSONGeneratedRuntime(t *testing.T){
 ui:=NewInspectorUI()
 if ui.Inspector.PageSize()!=32||ui.Inspector.RootPointer()!="/data"{t.Fatal("settings lost")}
 if err:=ui.Inspector.SelectPointer("/data/count");err!=nil{t.Fatal(err)}
 if selectedPath!="/data/count"{t.Fatal("generated signal not wired",selectedPath)}
 n,ok:=ui.Inspector.SelectedNode();if !ok{t.Fatal("no selected value")};value,err:=n.Int();if err!=nil||value!=9007199254740993{t.Fatal(value,err)}
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", pair.MachineFile, pair.UserFile, check)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated event runtime: %v\n%s\n%s", err, out, machine)
	}
}
func TestDeclarativeJSONView(t *testing.T) {
	tree := decl.JSONView(decl.ID("inspector"), decl.P("JSON Text", `{"data":[1,2,3]}`), decl.P("Root Pointer", "/data"), decl.P("Page Size", 64))
	widget, err := tree.Build()
	if err != nil {
		t.Fatal(err)
	}
	v, ok := widget.(*gui.JSONView)
	if !ok || v.RootPointer() != "/data" || v.PageSize() != 64 || v.LastError() != nil {
		t.Fatal(widget)
	}
	if err := v.SelectPointer("/data/2"); err != nil {
		t.Fatal(err)
	}
}
