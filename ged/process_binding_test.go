package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"strings"
	"testing"
)

func TestProcessBindingDesignerPersistencePreviewAndGeneratedCode(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(480, 260)
	x, y := view.MapFromScene(10, 10)
	view.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.ProcessValue"})
	fake := view.Scene().Children()[0].(*FakeWidget)
	fake.SetWidgetName("Temperature")
	w := fake.Widget().(*gui.ProcessValue)
	w.SetTagName("AI101")
	w.SetUnit("°C")
	transform := `[{"type":"float"},{"expr":"x*0.1"}]`
	rules := `[{"when":"x>=80","color":"#ffaa00"}]`
	w.SetTransformJSON(transform)
	w.SetColorRulesJSON(rules)
	w.SetPreviewValue(928)
	w.SetSample(core.Value{Raw: "1234", Quality: core.QualityGood}) // must not persist acquired runtime value
	doc, e := core.LoadTDocStr(fake.SaveDesign().String())
	if e != nil {
		t.Fatal(e)
	}
	dst, e := NewFakeWidgetFromFactory("gui.ProcessValue")
	if e != nil {
		t.Fatal(e)
	}
	if e = dst.LoadDesign(doc); e != nil {
		t.Fatal(e)
	}
	for _, item := range []gui.IWidget{dst.Widget(), dst.Generate()} {
		p := item.(*gui.ProcessValue)
		if p.TransformJSON() != transform || p.ColorRulesJSON() != rules || p.PreviewValue() != 928 || p.DisplayText() != "92.8 °C" {
			t.Fatal("double converted/lost config", p.PreviewValue(), p.DisplayText())
		}
	}
	result := view.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "ui", TypeName: "ReactorUI"})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	for _, want := range []string{"SetTransformJSON(", "SetColorRulesJSON(", "SetPreviewValue(928)", "SetTagName(\"AI101\")", "scada.BindScreen"} {
		if !strings.Contains(result.Code, want) {
			t.Fatal("missing generated binding", want)
		}
	}
	if !testing.Short() {
		vetGeneratedCode(t, result.Code)
	}
}

func TestProcessWorkbenchDesignerFactoriesRoundtripAndCodegen(t *testing.T) {
	for _, factory := range []string{"gui.ProcessDiagram", "gui.ProcessEditor"} {
		t.Run(factory, func(t *testing.T) {
			view := NewGedView()
			view.GedScene().SetSize(1100, 800)
			x, y := view.MapFromScene(10, 10)
			view.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: factory})
			fake := view.Scene().Children()[0].(*FakeWidget)
			type config interface {
				ProjectJSON() string
				SetProjectJSON(string)
			}
			w := fake.Widget().(config)
			project := hmi.Template()
			project.Title = "Designer authored process"
			data, _ := project.JSON()
			w.SetProjectJSON(string(data))
			before := w.ProjectJSON()
			doc, err := core.LoadTDocStr(fake.SaveDesign().String())
			if err != nil {
				t.Fatal(err)
			}
			dst, err := NewFakeWidgetFromFactory(factory)
			if err != nil {
				t.Fatal(err)
			}
			if err = dst.LoadDesign(doc); err != nil {
				t.Fatal(err)
			}
			if dst.Widget().(config).ProjectJSON() != before || dst.Generate().(config).ProjectJSON() != before {
				t.Fatal("document did not roundtrip")
			}
			result := view.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "ui", TypeName: "ProcessUI"})
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			if !strings.Contains(result.Code, "SetProjectJSON(") {
				t.Fatal("missing project setter")
			}
			if !testing.Short() {
				vetGeneratedCode(t, result.Code)
			}
		})
	}
}
