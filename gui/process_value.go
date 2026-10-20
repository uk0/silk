package gui

import (
	"fmt"
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// ProcessValue presents a live engineering value, unit, range and alarm state.
// It does not write to a PLC; thresholds affect display only. No idle timer.
type ProcessValue struct {
	Widget
	value, min, max, alarmAbove float64
	title, unit, tagName        string
	precision                   int
	valueFont                   paint.Font
	valueText                   string
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
func finiteProcess(v float64) bool     { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (w *ProcessValue) Value() float64 { return w.value }
func (w *ProcessValue) SetValue(v float64) {
	if finiteProcess(v) && v != w.value {
		w.value = v
		w.valueText = ""
		w.Self().Update()
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
func (w *ProcessValue) IsAlarm() bool { return w.value >= w.alarmAbove }
func (w *ProcessValue) Fraction() float64 {
	if w.max <= w.min {
		return 0
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
	p.AddProperty("数值", w.Value, w.SetValue)
	p.AddProperty("最小值", w.Min, w.SetMin)
	p.AddProperty("最大值", w.Max, w.SetMax)
	p.AddProperty("报警阈值", w.AlarmAbove, w.SetAlarmAbove)
	p.AddProperty("小数位", w.Precision, w.SetPrecision)
	p.AddProperty("tag", w.TagName, w.SetTagName)
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
	accent := paint.CurrentIconPalette().Accent
	if w.IsAlarm() {
		accent = paint.CurrentIconPalette().Danger
	}
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
	if w.valueText == "" {
		w.valueText = fmt.Sprintf("%.*f %s", w.precision, w.value, w.unit)
	}
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
