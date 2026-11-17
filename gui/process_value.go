package gui

import (
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

// ProcessValue presents a live engineering value, unit, range and alarm state.
// It does not write to a PLC; thresholds affect display only. No idle timer.
type ProcessValue struct {
	Widget
	previewValue                float64
	value, min, max, alarmAbove float64
	integerValue                int64
	hasIntegerValue             bool
	title, unit, tagName        string
	precision                   int
	valueFont                   paint.Font
	valueText                   string
	transformJSON, rulesJSON    string
	pipeline                    *point.Pipeline
	rules                       []processColorRule
	rawSample                   core.Value
	hasSample                   bool
	sampleError, configError    error
	quality                     core.Quality
	ruleColor                   paint.Color
	hasRuleColor                bool
	ruleText                    string
}

func init()                          { core.RegisterFactory("gui.ProcessValue", core.TypeOf((*ProcessValue)(nil))) }
func NewProcessValue() *ProcessValue { w := new(ProcessValue); w.Init(w); return w }
func (w *ProcessValue) Init(self IWidget) {
	w.Widget.Init(self)
	w.max = 100
	w.alarmAbove = 80
	w.title = "Process"
	w.precision = 1
}
func finiteProcess(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Value is the legacy float projection used by visual geometry. For exact BI
// samples outside the float exact-integer range, use IntegerValue instead.
func (w *ProcessValue) Value() float64 { return w.value }

// IntegerValue returns the last accepted integer and whether the current sample
// is a valid good-quality integer. Bad samples preserve history but return false.
func (w *ProcessValue) IntegerValue() (int64, bool) {
	return w.integerValue, w.hasIntegerValue && w.quality == core.QualityGood && w.BindingError() == nil
}

// SetValue supplies a local good-quality sample; conversion rules apply exactly once.
func (w *ProcessValue) SetValue(v float64) {
	if finiteProcess(v) {
		w.SetPreviewValue(v)
	}
}
func (w *ProcessValue) Min() float64 { return w.min }
func (w *ProcessValue) SetMin(v float64) {
	if finiteProcess(v) && v != w.min {
		w.min = v
		w.Self().Update()
	}
}
func (w *ProcessValue) Max() float64 { return w.max }
func (w *ProcessValue) SetMax(v float64) {
	if finiteProcess(v) && v != w.max {
		w.max = v
		w.Self().Update()
	}
}
func (w *ProcessValue) AlarmAbove() float64 { return w.alarmAbove }
func (w *ProcessValue) SetAlarmAbove(v float64) {
	if finiteProcess(v) && v != w.alarmAbove {
		w.alarmAbove = v
		w.Self().Update()
	}
}
func (w *ProcessValue) IsAlarm() bool {
	if w.quality != core.QualityGood || w.BindingError() != nil {
		return false
	}
	if w.hasIntegerValue {
		cmp, err := point.CompareNumbers(w.integerValue, w.alarmAbove)
		return err == nil && cmp >= 0
	}
	return w.value >= w.alarmAbove
}
func (w *ProcessValue) Fraction() float64 {
	if w.max <= w.min {
		return 0
	}
	if w.hasIntegerValue {
		low, _ := point.CompareNumbers(w.integerValue, w.min)
		high, _ := point.CompareNumbers(w.integerValue, w.max)
		if low <= 0 {
			return 0
		}
		if high >= 0 {
			return 1
		}
		// Nearby large integer bounds must not collapse to the same float.
		// Split only outside the exact-float range: for small integers the
		// direct subtraction also preserves sub-unit fractional boundaries.
		n := w.value - w.min
		if w.integerValue > 1<<53 || w.integerValue < -(1<<53) {
			n = (float64(w.integerValue>>32)*0x1p32 - w.min) + float64(uint32(w.integerValue))
		}
		span := w.max - w.min
		if math.IsInf(span, 0) {
			return (w.value/2 - w.min/2) / (w.max/2 - w.min/2)
		}
		return max(0, min(1, n/span))
	}
	if w.value <= w.min {
		return 0
	}
	if w.value >= w.max {
		return 1
	}
	span := w.max - w.min
	if math.IsInf(span, 0) {
		return (w.value/2 - w.min/2) / (w.max/2 - w.min/2)
	}
	return (w.value - w.min) / span
}
func (w *ProcessValue) Title() string { return w.title }
func (w *ProcessValue) SetTitle(s string) {
	if w.title != s {
		w.title = s
		w.Self().Update()
	}
}
func (w *ProcessValue) Unit() string { return w.unit }
func (w *ProcessValue) SetUnit(s string) {
	if w.unit != s {
		w.unit = s
		w.valueText = ""
		w.Self().Update()
	}
}
func (w *ProcessValue) Precision() int { return w.precision }
func (w *ProcessValue) SetPrecision(v int) {
	if v < 0 {
		v = 0
	}
	if v > 6 {
		v = 6
	}
	if v != w.precision {
		w.precision = v
		w.valueText = ""
		w.Self().Update()
	}
}
func (w *ProcessValue) TagName() string     { return w.tagName }
func (w *ProcessValue) SetTagName(s string) { w.tagName = s }
func (w *ProcessValue) EnumProperties(p core.IPropertyList) {
	p.AddProperty("标题", w.Title, w.SetTitle)
	p.AddProperty("单位", w.Unit, w.SetUnit)
	p.AddProperty("数值", w.PreviewValue, w.SetPreviewValue)
	p.AddProperty("最小值", w.Min, w.SetMin)
	p.AddProperty("最大值", w.Max, w.SetMax)
	p.AddProperty("报警阈值", w.AlarmAbove, w.SetAlarmAbove)
	p.AddProperty("小数位", w.Precision, w.SetPrecision)
	p.AddProperty("tag", w.TagName, w.SetTagName)
	p.AddProperty("转换规则 JSON", w.TransformJSON, w.SetTransformJSON)
	p.AddProperty("颜色规则 JSON", w.ColorRulesJSON, w.SetColorRulesJSON)
}
func (w *ProcessValue) SizeHints() SizeHints {
	return SizeHints{Width: 220, Height: 108, Policy: GrowHorizontal | GrowVertical}
}
func (w *ProcessValue) Draw(g paint.Painter) {
	width, height := w.Size()
	if width <= 2 || height <= 2 {
		return
	}
	g.Save()
	defer g.Restore()
	g.Rectangle(0, 0, width, height)
	g.Clip()
	theme := Theme()
	accent := w.DisplayColor()
	roundedRect(g, 0.5, 0.5, width-1, height-1, 6)
	g.SetBrush1(theme.ViewBGColor)
	g.FillPreserve()
	g.SetPen1(theme.BorderColor, 1)
	g.Stroke()
	g.SetBrush1(accent)
	g.Rectangle(1, 7, 3, math.Max(0, height-14))
	g.Fill()
	g.SetFont(theme.Font)
	g.SetBrush1(theme.TextColor)
	g.DrawText1(12, 22, w.title)
	if w.valueFont == nil || w.valueFont.Family() != theme.Font.Family() {
		w.valueFont = paint.NewFont(theme.Font.Family(), 20, true, false)
	}
	w.valueText = w.DisplayText()
	g.SetFont(w.valueFont)
	g.SetBrush1(accent)
	g.DrawText1(12, math.Min(54, height-18), w.valueText)
	if height >= 80 {
		g.SetBrush1(theme.BorderColor)
		g.Rectangle(12, height-16, math.Max(0, width-24), 4)
		g.Fill()
		g.SetBrush1(accent)
		g.Rectangle(12, height-16, math.Max(0, width-24)*w.Fraction(), 4)
		g.Fill()
	}
}
