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
