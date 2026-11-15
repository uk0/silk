package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestProcessPointMonitorPaletteFactoryPreviewAndCodegenAreInert(t *testing.T) {
	if !isPlaceable("gui.ProcessPointMonitor") || core.FindFactory("gui.ProcessPointMonitor") == nil {
		t.Fatal("point monitor missing factory/codegen mapping")
	}
	palette := NewWidgetList()
	offered := false
	for _, cat := range palette.categories {
		for _, item := range cat.items {
			if item.Data == "gui.ProcessPointMonitor" {
				offered = true
			}
		}
	}
	if !offered {
		t.Fatal("point monitor missing palette")
	}
	view := NewGedView()
	view.GedScene().SetSize(1100, 700)
	x, y := view.MapFromScene(10, 10)
	view.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.ProcessPointMonitor"})
	fake := view.Scene().Children()[0].(*FakeWidget)
	monitor := fake.Widget().(*gui.ProcessPointMonitor)
	defer monitor.Close()
	if len(monitor.PointStates()) != 0 {
		t.Fatal("designer started live data")
	}
	doc, err := core.LoadTDocStr(fake.SaveDesign().String())
	if err != nil {
		t.Fatal(err)
	}
	dst, err := NewFakeWidgetFromFactory("gui.ProcessPointMonitor")
	if err != nil {
		t.Fatal(err)
	}
	if err = dst.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	preview := dst.Generate().(*gui.ProcessPointMonitor)
	defer preview.Close()
	defer dst.Widget().(*gui.ProcessPointMonitor).Close()
	if len(preview.PointStates()) != 0 {
		t.Fatal("preview bound a runtime")
	}
	result := view.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "ui", TypeName: "PointUI"})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !strings.Contains(result.Code, "gui.NewProcessPointMonitor()") || strings.Contains(result.Code, "SetRuntime(") {
		t.Fatal("generated point monitor starts I/O or lacks constructor")
	}
	if !testing.Short() {
		vetGeneratedCode(t, result.Code)
	}
}
