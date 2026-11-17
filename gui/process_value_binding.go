package gui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

// ProcessColorRule evaluates against the converted value. First match wins.
// Colors are explicit #RRGGBB, not theme-dependent names; Text is an optional
// state label (e.g. "运行"). Empty rules retain the existing alarm threshold UI.
type ProcessColorRule struct {
	When  string `json:"when"`
	Color string `json:"color"`
	Text  string `json:"text,omitempty"`
}
type processColorRule struct {
	expr  *point.Expression
	color paint.Color
	text  string
}

func (w *ProcessValue) TransformJSON() string     { return w.transformJSON }
func (w *ProcessValue) ColorRulesJSON() string    { return w.rulesJSON }
func (w *ProcessValue) SetTransformJSON(s string) { _ = w.TrySetTransformJSON(s) }
func (w *ProcessValue) TrySetTransformJSON(s string) error {
	p, e := point.ParsePipeline(s)
	if e != nil {
		w.configError = e
		w.Update()
		return e
	}
	w.transformJSON = s
	w.pipeline = p
	w.configError = nil
	w.reapplyProcessSample()
	return nil
}
func (w *ProcessValue) SetColorRulesJSON(s string) { _ = w.TrySetColorRulesJSON(s) }
func (w *ProcessValue) TrySetColorRulesJSON(s string) error {
	var specs []ProcessColorRule
	if s != "" {
		if e := point.DecodeConfig(s, &specs); e != nil {
			w.configError = e
			w.Update()
			return e
		}
	}
	if len(specs) > 32 {
		e := fmt.Errorf("ProcessValue: at most 32 color rules")
		w.configError = e
		w.Update()
		return e
	}
	rules := make([]processColorRule, 0, len(specs))
	for _, r := range specs {
		expr, e := point.Compile(r.When)
		if e != nil {
			w.configError = e
			w.Update()
			return e
		}
		if len(r.Color) != 7 || !strings.HasPrefix(r.Color, "#") {
			e = fmt.Errorf("ProcessValue: color must be #RRGGBB")
		} else {
			var n uint64
			n, e = strconv.ParseUint(r.Color[1:], 16, 24)
			if e == nil {
				rules = append(rules, processColorRule{expr, paint.Color{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, r.Text})
			}
		}
		if e != nil {
			w.configError = e
			w.Update()
			return e
		}
	}
	w.rulesJSON = s
	w.rules = rules
	w.configError = nil
	w.reapplyProcessSample()
	return nil
}
func (w *ProcessValue) reapplyProcessSample() {
	if w.hasSample {
		w.SetSample(w.rawSample)
	} else {
		w.Update()
	}
}

// SetSample consumes the complete sample on the UI thread. Bad/uncertain or
// conversion errors never replace the last engineering value with a fake zero.
// Invalid display is visibly grey and "—"; runtime data is not persisted.
func (w *ProcessValue) SetSample(s core.Value) {
	oldText, oldColor, oldValue := w.DisplayText(), w.DisplayColor(), w.value
	defer func() {
		if oldText != w.DisplayText() || oldColor != w.DisplayColor() || oldValue != w.value {
			w.Update()
		}
	}()
	w.rawSample = s
	w.hasSample = true
	w.quality = s.Quality
	w.sampleError = nil
	w.hasRuleColor = false
	w.ruleText = ""
	if s.Quality != core.QualityGood {
		w.valueText = ""
		return
	}
	v, e := w.pipeline.Apply(s.Raw)
	nextValue := w.value
	nextInteger, integer := int64(0), false
	if e == nil {
		switch n := v.(type) {
		case int:
			nextInteger, integer = int64(n), true
		case int64:
			nextInteger, integer = n, true
		case json.Number:
			if i, err := n.Int64(); err == nil {
				nextInteger, integer = i, true
			}
		}
		if integer {
			// Only visual geometry/legacy Value use this approximate projection;
			// formatting, condition rules and alarm comparisons retain int64.
			nextValue = float64(nextInteger)
		} else {
			// Boolean pipelines drive numeric/state cards as 0 or 1.
			var n any
			n, e = point.Convert(v, point.Float)
			if e == nil {
				nextValue = n.(float64)
			}
		}
	}
	if e == nil {
		for _, r := range w.rules {
			var out any
			out, e = r.expr.Evaluate(v)
			if e != nil {
				break
			}
			matched, ok := out.(bool)
			if !ok {
				e = fmt.Errorf("ProcessValue: color condition must return bool")
				break
			}
			if matched {
				w.ruleColor = r.color
				w.hasRuleColor = true
				w.ruleText = r.text
				break
			}
		}
	}
	if e == nil {
		w.value = nextValue
		w.integerValue, w.hasIntegerValue = nextInteger, integer
	}
	w.sampleError = e
	w.valueText = ""
}
func (w *ProcessValue) BindingError() error {
	if w.configError != nil {
		return w.configError
	}
	return w.sampleError
}
func (w *ProcessValue) SampleQuality() core.Quality { return w.quality }
func (w *ProcessValue) DisplayColor() paint.Color {
	if w.BindingError() != nil || w.hasSample && w.quality != core.QualityGood {
		return paint.Color{R: 140, G: 148, B: 160, A: 255}
	}
	if w.hasRuleColor {
		return w.ruleColor
	}
	if w.IsAlarm() {
		return paint.CurrentIconPalette().Danger
	}
	return paint.CurrentIconPalette().Accent
}
func (w *ProcessValue) DisplayText() string {
	if w.BindingError() != nil || w.hasSample && w.quality != core.QualityGood {
		return "—"
	}
	if w.ruleText != "" {
		return w.ruleText
	}
	if w.valueText == "" {
		if w.hasIntegerValue {
			w.valueText = strconv.FormatInt(w.integerValue, 10)
			if w.precision > 0 {
				w.valueText += "." + strings.Repeat("0", w.precision)
			}
			w.valueText += " " + w.unit
		} else {
			w.valueText = fmt.Sprintf("%.*f %s", w.precision, w.value, w.unit)
		}
	}
	return w.valueText
}

// PreviewValue is the authored raw numeric preview, separate from live samples.
// Saving a running screen never serializes acquired data or double-converts it.
func (w *ProcessValue) PreviewValue() float64 { return w.previewValue }
func (w *ProcessValue) SetPreviewValue(v float64) {
	if finiteProcess(v) {
		w.previewValue = v
		w.SetSample(core.Value{Raw: v, Quality: core.QualityGood})
	}
}
