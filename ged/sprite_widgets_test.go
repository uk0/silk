package ged

import (
	"reflect"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestSpriteComponentsPaletteDropRoundTripAndCodegen(t *testing.T) {
	palette := NewWidgetList()
	for _, name := range []string{"gui.SpriteView", "gui.ProcessValue", "gui.Gauge"} {
		found := false
		for i := 0; i < palette.Count(); i++ {
			if palette.Item(i).Data == name {
				found = true
			}
		}
		if !found {
			t.Fatal("missing palette", name)
		}
		view := NewGedView()
		view.GedScene().SetSize(240, 160)
		x, y := view.MapFromScene(10, 10)
		view.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: name})
		if len(view.Scene().Children()) != 1 {
			t.Fatal("drop failed", name)
		}
		src := view.Scene().Children()[0].(*FakeWidget)
		src.SetWidgetName("Process")
		switch w := src.Widget().(type) {
		case *gui.SpriteView:
			w.SetIconName("satin:tank")
			w.SetActiveIconName("satin:valve")
			w.SetActive(true)
			w.SetTagName("motor.running")
		case *gui.ProcessValue:
			w.SetTitle("压力")
			w.SetUnit("bar")
			w.SetValue(42.5)
			w.SetMax(200)
			w.SetAlarmAbove(120)
			w.SetPrecision(2)
			w.SetTagName("pressure")
		case *gui.Gauge:
			w.SetTagName("pressure")
		}
		want := captureWidgetProperties(src.Widget().(core.IEnumProperties))
		doc, err := core.LoadTDocStr(src.SaveDesign().String())
		if err != nil {
			t.Fatal(err)
		}
		dst, err := NewFakeWidgetFromFactory(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := dst.LoadDesign(doc); err != nil {
			t.Fatal(err)
		}
		for _, w := range []gui.IWidget{dst.Widget(), dst.Generate()} {
			if got := captureWidgetProperties(w.(core.IEnumProperties)); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s roundtrip lost fields %v / %v", name, got, want)
			}
		}
		result := view.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "ui", TypeName: "ProcessUI"})
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		if !strings.Contains(result.Code, "gui.New"+strings.TrimPrefix(name, "gui.")+"()") || !strings.Contains(result.Code, "scada.BindScreen") {
			t.Fatal("missing concrete construction/tag", result.Code)
		}
		if !testing.Short() {
			vetGeneratedCode(t, result.Code)
		}
	}
}
