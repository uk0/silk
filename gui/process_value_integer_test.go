package gui

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func TestProcessValueExactIntegerDisplayAndColors(t *testing.T) {
	w := NewProcessValue()
	w.SetPrecision(0)
	w.SetUnit("pcs")
	w.SetAlarmAbove(0x1p63)
	if err := w.TrySetColorRulesJSON(`[{"when":"x == 9223372036854775807","color":"#ff0000"},{"when":"true","color":"#00aa00"}]`); err != nil {
		t.Fatal(err)
	}
	for _, v := range []int64{math.MaxInt64 - 1, math.MaxInt64, math.MinInt64, 9007199254740993} {
		w.SetSample(core.Value{Raw: v, Quality: core.QualityGood})
		want := strconv.FormatInt(v, 10) + " pcs"
		color := paint.Color{G: 170, A: 255}
		if v == math.MaxInt64 {
			color = paint.Color{R: 255, A: 255}
		}
		if w.BindingError() != nil || w.DisplayText() != want || w.DisplayColor() != color || w.IsAlarm() {
			t.Fatal("exact BI display/logic missing", v, w.DisplayText(), w.DisplayColor(), w.BindingError(), w.IsAlarm())
		}
	}
}

func TestProcessValueIntegerProjectionFractionQualityAndRecovery(t *testing.T) {
	w := NewProcessValue()
	w.SetPrecision(0)
	w.SetUnit("pcs")
	w.SetMin(0x1p53)
	w.SetMax(0x1p53 + 4)
	w.SetAlarmAbove(0x1p53 + 2)
	i := int64(1<<53 + 1)
	w.SetSample(core.Value{Raw: i, Quality: core.QualityGood})
	if n, ok := w.IntegerValue(); !ok || n != i || w.Value() != float64(i) || w.Fraction() != .25 || w.IsAlarm() {
		t.Fatal("integer projected before geometry/alarm", n, ok, w.Value(), w.Fraction(), w.IsAlarm())
	}
	w.SetSample(core.Value{Raw: i + 2, Quality: core.QualityGood})
	if w.Fraction() != .75 || !w.IsAlarm() {
		t.Fatal("adjacent integer collapsed", w.Fraction(), w.IsAlarm())
	}
	w.SetSample(core.Value{Raw: i, Quality: core.QualityBad})
	if _, ok := w.IntegerValue(); ok || w.DisplayText() != "—" || w.IsAlarm() {
		t.Fatal("bad integer presented as current")
	}
	w.SetSample(core.Value{Raw: json.Number("9007199254740993"), Quality: core.QualityGood})
	if n, ok := w.IntegerValue(); !ok || n != i || w.BindingError() != nil || w.DisplayText() != "9007199254740993 pcs" {
		t.Fatal("integer recovery lost precision", n, ok, w.DisplayText(), w.BindingError())
	}
	w.SetPrecision(2)
	if w.DisplayText() != "9007199254740993.00 pcs" {
		t.Fatal("integer precision formatting changed", w.DisplayText())
	}
	w.SetSample(core.Value{Raw: 12.25, Quality: core.QualityGood})
	if _, ok := w.IntegerValue(); ok || w.DisplayText() != "12.25 pcs" {
		t.Fatal("float did not retire integer mode", w.DisplayText())
	}
	w.SetMin(-1.000000001)
	w.SetMax(1)
	w.SetSample(core.Value{Raw: int64(-1), Quality: core.QualityGood})
	if math.Abs(w.Fraction()-((float64(-1)-w.Min())/(w.Max()-w.Min()))) > 1e-20 {
		t.Fatal("small integer lost fractional range precision", w.Fraction())
	}
}
