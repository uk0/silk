package ged

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/internal/silkgen"
)

func TestProcessDeviceLibraryDesignerPersistenceAndGoGeneration(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(1100, 800)
	x, y := view.MapFromScene(10, 10)
	view.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.ProcessEditor"})
	fake := view.Scene().Children()[0].(*FakeWidget)
	fake.SetWidgetName("Process")
	w := fake.Widget().(*gui.ProcessEditor)
	lib := hmi.BuiltinDeviceLibrary()
	lib.Devices[0].Name = "Custom pump station"
	data, err := lib.JSON()
	if err != nil {
		t.Fatal(err)
	}
	w.SetDeviceLibraryJSON(string(data))
	before := w.DeviceLibraryJSON()
	doc, err := core.LoadTDocStr(fake.SaveDesign().String())
	if err != nil {
		t.Fatal(err)
	}
	dst, err := NewFakeWidgetFromFactory("gui.ProcessEditor")
	if err != nil {
		t.Fatal(err)
	}
	if err = dst.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	for _, item := range []gui.IWidget{dst.Widget(), dst.Generate()} {
		if item.(*gui.ProcessEditor).DeviceLibraryJSON() != before {
			t.Fatal("library config not preserved")
		}
	}
	result := view.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "ui", TypeName: "ReusableProcessUI"})
	if result.Err != nil || !strings.Contains(result.Code, "SetDeviceLibraryJSON(") {
		t.Fatal("library setter missing", result.Err)
	}
	if !testing.Short() {
		path := filepath.Join(t.TempDir(), "device-library.silkui")
		view.GedScene().SetFilename(path)
		if !view.GedScene().Save() {
			t.Fatal("save library fixture")
		}
		doc, err := core.LoadTDocFile(path)
		if err != nil {
			t.Fatal(err)
		}
		tree, err := decl.FromTDoc(doc)
		if err != nil {
			t.Fatal(err)
		}
		generated, err := silkgen.GenerateSource(path, "ui", "", tree)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		files := map[string]string{
			"go.mod":        fmt.Sprintf("module devicefixture\ngo 1.26.0\nrequire github.com/uk0/silk v0.0.0\nreplace github.com/uk0/silk => %s\n", strconv.Quote(findModuleRoot(t))),
			"concrete.go":   result.Code,
			"decl.go":       string(generated),
			"scene_decl.go": view.GedScene().GenerateDeclCode(CodeGenOptions{PackageName: "ui", TypeName: "SceneLibrary"}),
			"runtime_test.go": fmt.Sprintf(`package ui
import("testing"; "github.com/uk0/silk/gui"; "github.com/uk0/silk/decl")
func TestDeviceLibraryConsumer(t *testing.T) {
 check:=func(w *gui.ProcessEditor){ t.Helper(); defer w.Close(); if w.LastError()!=nil || w.DeviceLibraryJSON()!=%q { t.Fatal("library replay changed",w.LastError()) } }
 concrete:=NewReusableProcessUI(); check(concrete.Process)
 for _,tree:=range []*decl.Node{%s(),BuildSceneLibrary()} {
  _,index,err:=tree.BuildWithIndex(); if err!=nil { t.Fatal(err) }; check(index["Process"].(*gui.ProcessEditor))
 }
}`, before, silkgen.DefaultBuildName(path)),
		}
		for name, source := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "test", "-mod=mod", "-p", "1", "-count=1", "-run", "TestDeviceLibraryConsumer", ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOMAXPROCS=2")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generated library consumer: %v\n%s", err, out)
		}
	}
}
