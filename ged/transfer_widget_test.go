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

func TestTransferWidgetDropPersistenceAndGeneratedRuntime(t *testing.T) {
	palette := NewWidgetList()
	found := false
	for i := 0; i < palette.Count(); i++ {
		if palette.Item(i).Data == "gui.TransferProgress" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing palette control")
	}
	v := NewGedView()
	v.GedScene().SetSize(600, 200)
	x, y := v.MapFromScene(10, 10)
	v.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.TransferProgress"})
	if len(v.Scene().Children()) != 1 {
		t.Fatal("drop failed")
	}
	fake := v.Scene().Children()[0].(*FakeWidget)
	fake.SetWidgetName("transfer")
	view := fake.Widget().(*gui.TransferProgress)
	view.SetLabel("资源下载 中文😀")
	view.SetShowBytes(false)
	fake.SetEventHandler("OnStateChanged", "onState")
	fake.SetCode(`func onState(state string) { stateSeen = state }`)
	view.SetProgress(123456789, 987654321)
	_ = view.SetState(gui.TransferRunning)
	doc := fake.SaveDesign()
	if strings.Contains(doc.String(), "123456789") || strings.Contains(doc.String(), "987654321") {
		t.Fatal("runtime state persisted")
	}
	dst, err := NewFakeWidgetFromFactory("gui.TransferProgress")
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
		if !reflect.DeepEqual(captureWidgetProperties(w.(core.IEnumProperties)), captureWidgetProperties(view)) {
			t.Fatal("design properties lost")
		}
		if w.(*gui.TransferProgress).State() != gui.TransferIdle || w.(*gui.TransferProgress).ReceivedBytes() != 0 {
			t.Fatal("runtime state restored")
		}
	}
	pair, err := v.GedScene().GenerateCodeFile(filepath.Join(t.TempDir(), "view.go"), CodeGenOptions{PackageName: "ui", TypeName: "TransferUI"})
	if err != nil {
		t.Fatal(err)
	}
	code, _ := os.ReadFile(pair.MachineFile)
	for _, want := range []string{"gui.NewTransferProgress()", "SetLabel(", "SetShowBytes(false)", "SigStateChanged(ui.onState)"} {
		if !strings.Contains(string(code), want) {
			t.Fatal("missing generated code", want)
		}
	}
	if testing.Short() {
		return
	}
	check := filepath.Join(filepath.Dir(pair.UserFile), "runtime_test.go")
	if err := os.WriteFile(check, []byte(`package ui
import("testing";"github.com/uk0/silk/gui")
var stateSeen string
func TestRuntime(t *testing.T){
 ui:=NewTransferUI()
 if ui.Transfer.Label()!="资源下载 中文😀"||ui.Transfer.ShowBytes(){t.Fatal("properties lost")}
 ui.Transfer.SetProgress(9007199254740993,9223372036854775807)
 if ui.Transfer.ReceivedBytes()!=9007199254740993{t.Fatal("precision lost")}
 if err:=ui.Transfer.SetState(gui.TransferRunning);err!=nil{t.Fatal(err)}
 if stateSeen!="running"{t.Fatal("event disconnected",stateSeen)}
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", pair.MachineFile, pair.UserFile, check)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated runtime: %v\n%s", err, out)
	}
}
func TestDeclarativeTransferProgress(t *testing.T) {
	w, err := decl.TransferProgress(decl.ID("transfer"), decl.P("Label", "Download"), decl.P("Show Bytes", false)).Build()
	if err != nil {
		t.Fatal(err)
	}
	v := w.(*gui.TransferProgress)
	if v.Label() != "Download" || v.ShowBytes() || v.State() != gui.TransferIdle {
		t.Fatal("declarative properties")
	}
}
