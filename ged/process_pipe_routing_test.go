package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
)

func TestProcessPipeRoutingDesignerPersistencePreviewAndCodegen(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(600, 360)
	x, y := view.MapFromScene(10, 10)
	view.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.ProcessDiagram"})
	fake := view.Scene().Children()[0].(*FakeWidget)
	fake.SetWidgetName("Process")
	w := fake.Widget().(*gui.ProcessDiagram)
	defer w.Close()
	p := hmi.Template()
	p.Links[0].Routing = hmi.RoutingAvoid
	if e := w.SetProject(p); e != nil {
		t.Fatal(e)
	}
	r, e := w.PipeRoute("feed")
	if e != nil || r.Status != hmi.PipeRouteOK {
		t.Fatal(r, e)
	}
	authored := w.ProjectJSON()
	doc, e := core.LoadTDocStr(fake.SaveDesign().String())
	if e != nil {
		t.Fatal(e)
	}
	dst, e := NewFakeWidgetFromFactory("gui.ProcessDiagram")
	if e != nil {
		t.Fatal(e)
	}
	if e = dst.LoadDesign(doc); e != nil {
		t.Fatal(e)
	}
	for _, item := range []gui.IWidget{dst.Widget(), dst.Generate()} {
		v := item.(*gui.ProcessDiagram)
		defer v.Close()
		if v.ProjectJSON() != authored || v.Project().Links[0].Routing != hmi.RoutingAvoid {
			t.Fatal("authored routing lost")
		}
		if r, e := v.PipeRoute("feed"); e != nil || r.Status != hmi.PipeRouteOK {
			t.Fatal("preview route failed", r, e)
		}
	}
	code := view.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "ui", TypeName: "ProcessUI"})
	if code.Err != nil || !strings.Contains(code.Code, "SetProjectJSON(") || strings.Contains(code.Code, "PipeRouteResult") {
		t.Fatal("generated scene lost config or persisted cache", code.Err)
	}
	if !testing.Short() {
		vetGeneratedCode(t, code.Code)
	}
}
