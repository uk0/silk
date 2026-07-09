package gui

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// ---------------------------------------------------------------------------
// Tank
// ---------------------------------------------------------------------------

func TestTankLevelRoundTrip(t *testing.T) {
	k := NewTank()
	if k.Level() != 0 {
		t.Errorf("initial Level = %v, want 0", k.Level())
	}
	k.SetLevel(0.42)
	if k.Level() != 0.42 {
		t.Errorf("Level = %v, want 0.42", k.Level())
	}
	// Clamp above 1 and below 0.
	k.SetLevel(1.5)
	if k.Level() != 1 {
		t.Errorf("Level clamp high = %v, want 1", k.Level())
	}
	k.SetLevel(-0.3)
	if k.Level() != 0 {
		t.Errorf("Level clamp low = %v, want 0", k.Level())
	}
}

func TestTankColorRoundTrip(t *testing.T) {
	k := NewTank()
	c := paint.Color{R: 10, G: 20, B: 30, A: 255}
	k.SetColor(c)
	if k.Color() != c {
		t.Errorf("Color = %v, want %v", k.Color(), c)
	}
}

func TestTankRangeAndEngValue(t *testing.T) {
	k := NewTank()
	k.SetRange(0, 200)
	if k.Min() != 0 || k.Max() != 200 {
		t.Errorf("range = [%v,%v], want [0,200]", k.Min(), k.Max())
	}
	k.SetLevel(0.5)
	if k.EngValue() != 100 {
		t.Errorf("EngValue = %v, want 100", k.EngValue())
	}
}

func TestTankShowLabel(t *testing.T) {
	k := NewTank()
	if !k.ShowLabel() {
		t.Error("ShowLabel should default true")
	}
	k.SetShowLabel(false)
	if k.ShowLabel() {
		t.Error("ShowLabel should be false after SetShowLabel(false)")
	}
}

// ---------------------------------------------------------------------------
// Indicator
// ---------------------------------------------------------------------------

func TestIndicatorOnOff(t *testing.T) {
	ind := NewIndicator()
	if ind.IsOn() {
		t.Error("Indicator should default off")
	}
	ind.SetOn(true)
	if !ind.IsOn() {
		t.Error("Indicator should be on after SetOn(true)")
	}
	ind.SetOn(false)
	if ind.IsOn() {
		t.Error("Indicator should be off after SetOn(false)")
	}
}

func TestIndicatorColors(t *testing.T) {
	ind := NewIndicator()
	on := paint.Color{R: 200, G: 0, B: 0, A: 255}
	off := paint.Color{R: 1, G: 2, B: 3, A: 255}
	ind.SetColor(on)
	ind.SetOffColor(off)
	if ind.Color() != on {
		t.Errorf("Color = %v, want %v", ind.Color(), on)
	}
	if ind.OffColor() != off {
		t.Errorf("OffColor = %v, want %v", ind.OffColor(), off)
	}
}

func TestIndicatorBlink(t *testing.T) {
	ind := NewIndicator()
	if ind.IsBlink() {
		t.Error("Indicator should not blink by default")
	}
	ind.SetBlink(true)
	if !ind.IsBlink() {
		t.Error("Indicator should blink after SetBlink(true)")
	}
}

// ---------------------------------------------------------------------------
// DigitalDisplay
// ---------------------------------------------------------------------------

func TestDigitalDisplayValueFormat(t *testing.T) {
	d := NewDigitalDisplay()
	d.SetValue(23.456)
	if d.Value() != 23.456 {
		t.Errorf("Value = %v, want 23.456", d.Value())
	}
	if d.Text() != "23.5" {
		t.Errorf("Text = %q, want %q", d.Text(), "23.5")
	}
	d.SetFormat("%.0f")
	if d.Format() != "%.0f" {
		t.Errorf("Format = %q, want %q", d.Format(), "%.0f")
	}
	if d.Text() != "23" {
		t.Errorf("Text = %q, want %q", d.Text(), "23")
	}
}

func TestDigitalDisplayUnitAndText(t *testing.T) {
	d := NewDigitalDisplay()
	d.SetUnit("°C")
	if d.Unit() != "°C" {
		t.Errorf("Unit = %q, want °C", d.Unit())
	}
	col := paint.Color{R: 5, G: 6, B: 7, A: 255}
	d.SetColor(col)
	if d.Color() != col {
		t.Errorf("Color = %v, want %v", d.Color(), col)
	}
}

func TestDigitalDisplayLimitsColor(t *testing.T) {
	d := NewDigitalDisplay()
	// No limits: always the normal color.
	if d.displayColor() != d.onColor {
		t.Error("without limits displayColor should be onColor")
	}
	d.SetLimits(10, 90)
	if d.Lo() != 10 || d.Hi() != 90 {
		t.Errorf("limits = [%v,%v], want [10,90]", d.Lo(), d.Hi())
	}
	d.SetValue(5)
	if d.displayColor() != d.loColor {
		t.Error("value below lo should use loColor")
	}
	d.SetValue(95)
	if d.displayColor() != d.hiColor {
		t.Error("value at/above hi should use hiColor")
	}
	d.SetValue(50)
	if d.displayColor() != d.onColor {
		t.Error("value inside band should use onColor")
	}
}

// ---------------------------------------------------------------------------
// Valve
// ---------------------------------------------------------------------------

func TestValveState(t *testing.T) {
	v := NewValve()
	if v.State() {
		t.Error("Valve should default closed")
	}
	v.SetState(true)
	if !v.State() {
		t.Error("Valve should be open after SetState(true)")
	}
}

func TestValveToggleAndColors(t *testing.T) {
	v := NewValve()
	v.Toggle()
	if !v.State() {
		t.Error("Toggle from closed should open")
	}
	v.Toggle()
	if v.State() {
		t.Error("Toggle from open should close")
	}
	oc := paint.Color{R: 1, G: 1, B: 1, A: 255}
	cc := paint.Color{R: 2, G: 2, B: 2, A: 255}
	v.SetOpenColor(oc)
	v.SetClosedColor(cc)
	if v.OpenColor() != oc || v.ClosedColor() != cc {
		t.Errorf("colors = %v/%v, want %v/%v", v.OpenColor(), v.ClosedColor(), oc, cc)
	}
}

// ---------------------------------------------------------------------------
// Pipe
// ---------------------------------------------------------------------------

func TestPipeActiveFlow(t *testing.T) {
	p := NewPipe()
	if p.IsActive() {
		t.Error("Pipe should default inactive")
	}
	p.SetActive(true)
	if !p.IsActive() {
		t.Error("Pipe should be active after SetActive(true)")
	}
	fc := paint.Color{R: 9, G: 8, B: 7, A: 255}
	p.SetFlowColor(fc)
	if p.FlowColor() != fc {
		t.Errorf("FlowColor = %v, want %v", p.FlowColor(), fc)
	}
}

func TestPipeOrientation(t *testing.T) {
	p := NewPipe()
	if p.IsVertical() {
		t.Error("Pipe should default horizontal")
	}
	p.SetVertical(true)
	if !p.IsVertical() {
		t.Error("Pipe should be vertical after SetVertical(true)")
	}
}

// ---------------------------------------------------------------------------
// Pump / Thermometer / ValueBar (extras)
// ---------------------------------------------------------------------------

func TestIndustrialPumpStates(t *testing.T) {
	p := NewPump()
	if p.IsRunning() || p.IsFault() {
		t.Error("Pump should default stopped and fault-free")
	}
	p.SetRunning(true)
	if !p.IsRunning() {
		t.Error("Pump should run after SetRunning(true)")
	}
	p.SetFault(true)
	if !p.IsFault() {
		t.Error("Pump should fault after SetFault(true)")
	}
}

func TestIndustrialThermometer(t *testing.T) {
	th := NewThermometer()
	th.SetRange(0, 200)
	if th.Min() != 0 || th.Max() != 200 {
		t.Errorf("range = [%v,%v], want [0,200]", th.Min(), th.Max())
	}
	th.SetValue(50)
	if th.Value() != 50 {
		t.Errorf("Value = %v, want 50", th.Value())
	}
	if th.Fraction() != 0.25 {
		t.Errorf("Fraction = %v, want 0.25", th.Fraction())
	}
	// Clamp to range.
	th.SetValue(500)
	if th.Value() != 200 {
		t.Errorf("Value clamp = %v, want 200", th.Value())
	}
}

func TestIndustrialValueBar(t *testing.T) {
	b := NewValueBar()
	// Without limits the bar uses the normal color.
	if b.barColor() != b.normalColor {
		t.Error("without limits barColor should be normalColor")
	}
	b.SetLimits(10, 20, 80, 90)
	if b.LoLo() != 10 || b.Lo() != 20 || b.Hi() != 80 || b.HiHi() != 90 {
		t.Errorf("limits = [%v,%v,%v,%v], want [10,20,80,90]",
			b.LoLo(), b.Lo(), b.Hi(), b.HiHi())
	}
	b.SetValue(5)
	if b.barColor() != b.alarmColor {
		t.Error("value at/below loLo should be alarmColor")
	}
	b.SetValue(15)
	if b.barColor() != b.warnColor {
		t.Error("value at/below lo should be warnColor")
	}
	b.SetValue(50)
	if b.barColor() != b.normalColor {
		t.Error("value inside band should be normalColor")
	}
	b.SetValue(85)
	if b.barColor() != b.warnColor {
		t.Error("value at/above hi should be warnColor")
	}
	b.SetValue(95)
	if b.barColor() != b.alarmColor {
		t.Error("value at/above hiHi should be alarmColor")
	}
}

// ---------------------------------------------------------------------------
// Factory registration
// ---------------------------------------------------------------------------

func TestIndustrialFactoryRegistration(t *testing.T) {
	names := []string{
		"gui.Tank", "gui.Indicator", "gui.DigitalDisplay", "gui.Valve",
		"gui.Pipe", "gui.Pump", "gui.Thermometer", "gui.ValueBar",
	}
	for _, n := range names {
		f := core.FindFactory(n)
		if f == nil {
			t.Errorf("factory %q not registered", n)
			continue
		}
		if f.New() == nil {
			t.Errorf("factory %q New() returned nil", n)
		}
	}
}

// ---------------------------------------------------------------------------
// TagName / design-time "tag" property
// ---------------------------------------------------------------------------

// tagged is the design-time tag-binding surface every industrial widget
// exposes: a plain TagName round-trip plus EnumProperties for the property
// sheet.
type tagged interface {
	core.IEnumProperties
	SetTagName(string)
	TagName() string
}

// industrialTaggedWidgets returns one fresh instance of every industrial
// widget, keyed by name for readable failures.
func industrialTaggedWidgets() map[string]tagged {
	return map[string]tagged{
		"Tank":           NewTank(),
		"Indicator":      NewIndicator(),
		"DigitalDisplay": NewDigitalDisplay(),
		"Valve":          NewValve(),
		"Pipe":           NewPipe(),
		"Pump":           NewPump(),
		"Thermometer":    NewThermometer(),
		"ValueBar":       NewValueBar(),
	}
}

// propCapture is a core.IPropertyList stand-in that records the get/set funcs a
// widget registers so a test can drive an individual property by id.
type propCapture struct {
	gets map[string]interface{}
	sets map[string]interface{}
}

func newPropCapture() *propCapture {
	return &propCapture{
		gets: map[string]interface{}{},
		sets: map[string]interface{}{},
	}
}

func (p *propCapture) AddProperty(id string, get, set interface{}) {
	p.gets[id] = get
	p.sets[id] = set
}

func TestIndustrialTagNameRoundTrip(t *testing.T) {
	for name, w := range industrialTaggedWidgets() {
		if got := w.TagName(); got != "" {
			t.Errorf("%s: initial TagName = %q, want empty", name, got)
		}
		w.SetTagName("level")
		if got := w.TagName(); got != "level" {
			t.Errorf("%s: TagName = %q, want %q", name, got, "level")
		}
	}
}

func TestIndustrialTagEnumProperty(t *testing.T) {
	for name, w := range industrialTaggedWidgets() {
		pc := newPropCapture()
		w.EnumProperties(pc)

		get, ok := pc.gets["tag"].(func() string)
		if !ok {
			t.Fatalf("%s: EnumProperties exposes no string getter for %q", name, "tag")
		}
		set, ok := pc.sets["tag"].(func(string))
		if !ok {
			t.Fatalf("%s: EnumProperties exposes no string setter for %q", name, "tag")
		}

		// The property's setter drives the field; its getter reflects it.
		set("flow")
		if got := get(); got != "flow" {
			t.Errorf("%s: tag getter after property set = %q, want %q", name, got, "flow")
		}
		// The property's getter also reflects writes made via SetTagName.
		w.SetTagName("direct")
		if got := get(); got != "direct" {
			t.Errorf("%s: tag getter after SetTagName = %q, want %q", name, got, "direct")
		}
	}
}

// ---------------------------------------------------------------------------
// Expanded design-time property sheets
// ---------------------------------------------------------------------------

// TestIndustrialEnumPropertyIDs pins the full property-sheet contract of every
// industrial widget: the pre-existing entries survive and the added ones are
// exposed.
func TestIndustrialEnumPropertyIDs(t *testing.T) {
	want := map[string][]string{
		"Tank":           {"液位", "显示标签", "颜色", "tag"},
		"Indicator":      {"点亮", "闪烁", "颜色", "熄灭颜色", "tag"},
		"DigitalDisplay": {"数值", "格式", "单位", "颜色", "tag"},
		"Valve":          {"打开", "打开颜色", "关闭颜色", "tag"},
		"Pipe":           {"有流量", "竖直", "流动颜色", "tag"},
		"Pump":           {"运行", "故障", "tag"},
		"Thermometer":    {"温度", "颜色", "tag"},
		"ValueBar":       {"数值", "tag"},
	}
	for name, w := range industrialTaggedWidgets() {
		pc := newPropCapture()
		w.EnumProperties(pc)
		for _, id := range want[name] {
			if _, ok := pc.gets[id]; !ok {
				t.Errorf("%s: property %q not exposed", name, id)
			}
		}
	}
}

// roundTripColorProp drives one captured paint.Color property end to end: the
// recorded setter stores the value and the recorded getter reflects it.
func roundTripColorProp(t *testing.T, name, id string, pc *propCapture, want paint.Color) {
	t.Helper()
	get, ok := pc.gets[id].(func() paint.Color)
	if !ok {
		t.Errorf("%s: EnumProperties exposes no color getter for %q", name, id)
		return
	}
	set, ok := pc.sets[id].(func(paint.Color))
	if !ok {
		t.Errorf("%s: EnumProperties exposes no color setter for %q", name, id)
		return
	}
	set(want)
	if got := get(); got != want {
		t.Errorf("%s: %q round-trip = %v, want %v", name, id, got, want)
	}
}

func TestIndustrialColorEnumProperties(t *testing.T) {
	cases := []struct {
		name string
		w    core.IEnumProperties
		ids  []string
	}{
		{"Tank", NewTank(), []string{"颜色"}},
		{"Indicator", NewIndicator(), []string{"颜色", "熄灭颜色"}},
		{"DigitalDisplay", NewDigitalDisplay(), []string{"颜色"}},
		{"Valve", NewValve(), []string{"打开颜色", "关闭颜色"}},
		{"Pipe", NewPipe(), []string{"流动颜色"}},
		{"Thermometer", NewThermometer(), []string{"颜色"}},
	}
	for _, c := range cases {
		pc := newPropCapture()
		c.w.EnumProperties(pc)
		for i, id := range c.ids {
			want := paint.Color{R: uint8(40 + 10*i), G: uint8(50 + 10*i), B: uint8(60 + 10*i), A: 255}
			roundTripColorProp(t, c.name, id, pc, want)
		}
	}
}

// TestGaugeEnumProperties covers the Gauge chart widget (chart_gauge.go): the
// pre-existing 标题/单位 entries survive and the added 数值 entry round-trips.
func TestGaugeEnumProperties(t *testing.T) {
	g := NewGauge()
	pc := newPropCapture()
	g.EnumProperties(pc)

	for _, id := range []string{"标题", "单位"} {
		if _, ok := pc.gets[id]; !ok {
			t.Errorf("Gauge: property %q not exposed", id)
		}
	}

	get, ok := pc.gets["数值"].(func() float64)
	if !ok {
		t.Fatalf("Gauge: EnumProperties exposes no float64 getter for %q", "数值")
	}
	set, ok := pc.sets["数值"].(func(float64))
	if !ok {
		t.Fatalf("Gauge: EnumProperties exposes no float64 setter for %q", "数值")
	}
	set(42.5)
	if got := get(); got != 42.5 {
		t.Errorf("Gauge: 数值 round-trip = %v, want 42.5", got)
	}
	// The property setter drives the same state as SetValue.
	g.SetValue(63)
	if got := get(); got != 63 {
		t.Errorf("Gauge: 数值 getter after SetValue = %v, want 63", got)
	}
}
