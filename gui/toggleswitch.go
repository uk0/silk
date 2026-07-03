package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

// ToggleSwitch 开关控件，iOS 风格的滑动开关
type ToggleSwitch struct {
	Widget
	checked    bool
	text       string
	pushed     bool
	cbToggle   func(bool)
	readonly   bool
	animOffset float64 // 0.0=off, 1.0=on
}

func init() {
	core.RegisterFactory("gui.ToggleSwitch", core.TypeOf((*ToggleSwitch)(nil)))
}

func NewToggleSwitch() *ToggleSwitch {
	p := new(ToggleSwitch)
	p.Init(p)
	return p
}

func (this *ToggleSwitch) IsChecked() bool {
	return this.checked
}

func (this *ToggleSwitch) SetChecked(b bool) {
	if this.checked != b {
		this.checked = b
		if b {
			this.animOffset = 1.0
		} else {
			this.animOffset = 0.0
		}
		this.Self().Update()
	}
}

func (this *ToggleSwitch) Toggle() {
	this.checked = !this.checked
	if this.checked {
		this.animOffset = 1.0
	} else {
		this.animOffset = 0.0
	}
	if this.cbToggle != nil {
		this.cbToggle(this.checked)
	}
	this.Self().Update()
}

func (this *ToggleSwitch) Text() string {
	return this.text
}

func (this *ToggleSwitch) SetText(s string) {
	this.text = s
	this.Self().Update()
}

func (this *ToggleSwitch) IsEnabled() bool {
	return !this.readonly
}

func (this *ToggleSwitch) SetEnabled(b bool) {
	this.readonly = !b
	this.Self().Update()
}

func (this *ToggleSwitch) SigToggle(fn func(bool)) {
	this.cbToggle = fn
}

// --- Events ---

func (this *ToggleSwitch) OnMouseEnter() {
	this.Self().Update()
}

func (this *ToggleSwitch) OnMouseLeave() {
	this.Self().Update()
}

func (this *ToggleSwitch) OnLeftDown(x, y float64) {
	if this.IsEnabled() {
		this.pushed = true
		this.SetFocus()
		this.Self().Update()
	}
}

func (this *ToggleSwitch) OnLeftUp(x, y float64) {
	pushed := this.pushed
	this.pushed = false
	this.PopCapture()
	if pushed && this.IsHover() && this.IsEnabled() {
		this.Toggle()
	}
	this.Self().Update()
}

// OnKeyDown implements IEventKeyDown, giving the switch Qt-style keyboard
// control while it holds focus. Space (and Enter, for convenience) flips the
// state like a click; Left forces it OFF and Right forces it ON (the Qt switch
// direction convention). All paths route through Toggle so the change callback
// fires exactly as a click does, and the explicit Left/Right cases only toggle
// when the state actually changes, so re-asserting the current state is a no-op
// with no spurious callback. Guarded on IsEnabled so a disabled switch ignores
// keys.
func (this *ToggleSwitch) OnKeyDown(key int, repeat bool) {
	if !this.IsEnabled() {
		return
	}
	switch key {
	case KeySpace, KeyEnter:
		this.Toggle()
	case KeyLeft:
		if this.checked {
			this.Toggle()
		}
	case KeyRight:
		if !this.checked {
			this.Toggle()
		}
	}
}

// --- Drawing ---

func (this *ToggleSwitch) Draw(g paint.Painter) {
	t := Theme()
	_, h := this.Size()

	trackW := 40.0
	trackH := 20.0
	radius := trackH / 2
	thumbRadius := 8.0
	thumbPadding := 2.0

	// vertical center
	ty := (h - trackH) / 2

	// track background
	onColor := t.HighLightColor
	offColor := t.BorderColor
	disabledColor := t.FormColor

	var trackColor paint.Color
	if !this.IsEnabled() {
		trackColor = disabledColor
	} else if this.checked {
		trackColor = onColor
	} else {
		trackColor = offColor
	}

	// draw rounded track
	g.Save()

	// left semicircle
	g.Arc(radius, ty+radius, radius, math.Pi/2, 3*math.Pi/2)
	// top edge
	g.LineTo(trackW-radius, ty)
	// right semicircle
	g.Arc(trackW-radius, ty+radius, radius, -math.Pi/2, math.Pi/2)
	// bottom edge
	g.LineTo(radius, ty+trackH)

	g.SetBrush1(trackColor)
	g.Fill()

	// thumb circle
	thumbX := thumbPadding + thumbRadius
	if this.checked {
		thumbX = trackW - thumbPadding - thumbRadius
	}
	thumbY := ty + trackH/2

	g.Arc(thumbX, thumbY, thumbRadius, 0, 2*math.Pi)
	if this.pushed && this.IsEnabled() {
		g.SetBrush1(t.FormColor)
	} else {
		g.SetBrush1(t.ViewBGColor)
	}
	g.FillPreserve()
	if this.IsHover() && this.IsEnabled() {
		g.SetPen1(t.MenuGrayTextColor, 1)
	} else {
		g.SetPen1(t.BorderColor, 0.5)
	}
	g.Stroke()

	g.Restore()

	// focus ring around the track while the switch holds keyboard focus
	if this.HasFocus() {
		this.drawFocusRing(g, trackW, trackH, ty)
	}

	// text label
	if this.text != "" {
		g.SetFont(t.Font)
		g.SetBrush1(t.TextColor)
		fe := t.Font.FontExtents()
		ext := t.Font.TextExtents(this.text)
		tx := trackW + 8
		tty := (h+fe.Ascent-fe.Descent)/2 - ext.YBearing - fe.Ascent + fe.Descent
		tty = 0.5*(h+ext.YBearing) - ext.YBearing
		g.Translate(tx-ext.XBearing, tty)
		g.DrawText(this.text)
		g.Translate(-(tx - ext.XBearing), -tty)
	}
}

// drawFocusRing paints a subtle accent outline around the track while the
// switch holds keyboard focus, so keyboard users can see where they are. It
// uses the theme highlight color at low alpha (the same accent the edit frame
// uses when focused) and follows the stadium track shape with a small inset.
func (this *ToggleSwitch) drawFocusRing(g paint.Painter, trackW, trackH, ty float64) {
	pad := 2.0
	c := Theme().HighLightColor
	c.A = 90 // low alpha keeps it subtle
	roundedRect(g, -pad, ty-pad, trackW+pad*2, trackH+pad*2, trackH/2+pad)
	g.SetPen1(c, 1.5)
	g.Stroke()
}

func (this *ToggleSwitch) SizeHints() SizeHints {
	t := Theme()
	w := 40.0
	h := 24.0
	if this.text != "" {
		ext := t.Font.TextExtents(this.text)
		fe := t.Font.FontExtents()
		w += 8 + ext.Width
		h = math.Max(h, fe.Height+4)
	}
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}

func (this *ToggleSwitch) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("选中", this.IsChecked, this.SetChecked)
	list.AddProperty("可用", this.IsEnabled, this.SetEnabled)
}
