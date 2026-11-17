package ged

import (
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func TestProcessIntegerDesignerDropPersistencePreviewAndCodegen(t *testing.T) {
	view := NewGedView()
	view.GedScene().SetSize(480, 260)
	x, y := view.MapFromScene(10, 10)
	view.OnDrop(x, y, &dropCtx{pa: gui.DndCopy, payload: "gui.ProcessValue"})
	fake := view.Scene().Children()[0].(*FakeWidget)
	fake.SetWidgetName("Counter")
	w := fake.Widget().(*gui.ProcessValue)
	w.SetTagName("BI_COUNTER")
	w.SetUnit("pcs")
	w.SetPrecision(0)
	transform := `[{"type":"int"},{"expr":"x &^ 0x2"}]`
	rules := `[{"when":"x == 9223372036854775805","color":"#2196f3"}]`
	w.SetTransformJSON(transform)
	w.SetColorRulesJSON(rules)
	w.SetPreviewValue(15)
	w.SetSample(core.Value{Raw: int64(math.MaxInt64), Quality: core.QualityGood})
	if n, ok := w.IntegerValue(); !ok || n != math.MaxInt64-2 || w.DisplayText() != "9223372036854775805 pcs" {
		t.Fatal("designer sample lost exact integer", n, ok, w.DisplayText(), w.BindingError())
	}
	doc, err := core.LoadTDocStr(fake.SaveDesign().String())
	if err != nil {
		t.Fatal(err)
	}
	dst, err := NewFakeWidgetFromFactory("gui.ProcessValue")
	if err != nil {
		t.Fatal(err)
	}
	if err := dst.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	for _, item := range []gui.IWidget{dst.Widget(), dst.Generate()} {
		card := item.(*gui.ProcessValue)
		if card.TransformJSON() != transform || card.ColorRulesJSON() != rules || card.PreviewValue() != 15 || card.DisplayText() != "13 pcs" {
			t.Fatal("integer preview double-applied or acquired value persisted", card.PreviewValue(), card.DisplayText())
		}
		card.SetSample(core.Value{Raw: int64(math.MaxInt64), Quality: core.QualityGood})
		if n, ok := card.IntegerValue(); !ok || n != math.MaxInt64-2 || card.DisplayColor() != (paint.Color{R: 33, G: 150, B: 243, A: 255}) {
			t.Fatal("integer runtime/color lost on reopen/preview", n, ok, card.DisplayColor(), card.BindingError())
		}
	}
	code := view.GedScene().GenerateCodeIndexed(CodeGenOptions{PackageName: "ui", TypeName: "CounterUI"})
	if code.Err != nil {
		t.Fatal(code.Err)
	}
	if strings.Contains(code.Code, "SetValue(9223372036854775807)") || !strings.Contains(code.Code, "SetPreviewValue(15)") || !strings.Contains(code.Code, "SetTransformJSON(") {
		t.Fatal("generated source persisted runtime state/lost integer pipeline")
	}
	if !testing.Short() {
		vetGeneratedCode(t, code.Code)
	}
}
