package gui

import (
	"math"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func TestProcessValueTransformsRulesQualityAndRecovery(t *testing.T) {
	w := NewProcessValue()
	w.SetUnit("°C")
	if e := w.TrySetTransformJSON(`[{"type":"float"},{"expr":"x*0.1"}]`); e != nil {
		t.Fatal(e)
	}
	rules := `[{"when":"x>=100","color":"#ff0000"},{"when":"x>=80","color":"#ffaa00"},{"when":"true","color":"#00aa00"}]`
	if e := w.TrySetColorRulesJSON(rules); e != nil {
		t.Fatal(e)
	}
	w.SetSample(core.Value{Raw: "928", Quality: core.QualityGood})
	if math.Abs(w.Value()-92.8) > 1e-10 || w.DisplayText() != "92.8 °C" || w.DisplayColor() != (paint.Color{R: 255, G: 170, A: 255}) {
		t.Fatal(w.Value(), w.DisplayText(), w.DisplayColor())
	}
	w.SetSample(core.Value{Raw: "broken", Quality: core.QualityGood})
	if w.BindingError() == nil || w.DisplayText() != "—" || math.Abs(w.Value()-92.8) > 1e-10 {
		t.Fatal("malformed input replaced valid value")
	}
	w.SetSample(core.Value{Raw: 928.0, Quality: core.QualityBad})
	if w.DisplayText() != "—" || w.IsAlarm() {
		t.Fatal("bad sample looks live")
	}
	w.SetSample(core.Value{Raw: "1020", Quality: core.QualityGood})
	if w.DisplayText() != "102.0 °C" || w.DisplayColor() != (paint.Color{R: 255, A: 255}) || w.BindingError() != nil {
		t.Fatal("recovery failed")
	}
	if e := w.TrySetColorRulesJSON(`[{"when":"true","color":"red"}]`); e == nil || w.ColorRulesJSON() != rules {
		t.Fatal("invalid design committed")
	}
	if e := w.TrySetColorRulesJSON(rules); e != nil {
		t.Fatal(e)
	}
	w.SetColorRulesJSON(`[{"when":"true","color":"#00ff00","text":"运行"}]`)
	w.SetTransformJSON(`[{"type":"bool"}]`)
	w.SetSample(core.Value{Raw: true, Quality: core.QualityGood})
	if w.DisplayText() != "运行" {
		t.Fatal(w.DisplayText())
	}
}
func TestProcessValueBindingCoalescesAndCancels(t *testing.T) {
	isolateTagBridge(t)
	w := NewProcessValue()
	tag := core.NewTagDB().GetOrCreate("AI101", core.Meta{})
	stop := BindTagUI(tag, w.SetSample)
	for i := 0; i < 1000; i++ {
		tag.SetValue(float64(i))
	}
	drainUITasks()
	if w.Value() != 999 {
		t.Fatal(w.Value())
	}
	tag.SetValue(123.)
	stop()
	drainUITasks()
	if w.Value() != 999 {
		t.Fatal("queued update survived cancel")
	}
}
