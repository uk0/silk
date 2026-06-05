package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.StatusBar", core.TypeOf((*StatusBar)(nil)))
}

// StatusBar is a horizontal bar at the bottom of a frame showing a message
// and optional permanent widgets on the right. Similar to QStatusBar.
type StatusBar struct {
	Widget
	message   string
	permanent []IWidget // permanent widgets anchored to the right
	spacing   float64
	msgTimer  Timer // auto-clears a timed transient message
}

func NewStatusBar() *StatusBar {
	p := new(StatusBar)
	p.Init(p)
	p.spacing = Theme().Spacing
	return p
}

func (this *StatusBar) EnumProperties(list core.IPropertyList) {
	list.AddProperty("消息", this.Message, this.SetMessage)
}

// ShowMessage displays a temporary status message (no timeout for now).
func (this *StatusBar) ShowMessage(text string) {
	this.msgTimer.Stop()
	this.message = text
	this.Self().Update()
}

// ShowMessageFor displays a transient status message that is automatically
// cleared after timeoutMs milliseconds (like QStatusBar.showMessage with a
// timeout). A pending timer from an earlier timed message is replaced.
func (this *StatusBar) ShowMessageFor(text string, timeoutMs uint32) {
	this.msgTimer.Stop()
	this.message = text
	this.Self().Update()
	this.msgTimer.Start(timeoutMs, func() {
		this.msgTimer.Stop()
		this.ClearMessage()
	})
}

// SetMessage sets the permanent status message.
func (this *StatusBar) SetMessage(text string) {
	if this.message == text {
		return
	}
	this.message = text
	this.Self().Update()
}

// Message returns the current status message.
func (this *StatusBar) Message() string {
	return this.message
}

// ClearMessage clears the status message.
func (this *StatusBar) ClearMessage() {
	this.message = ""
	this.Self().Update()
}

// AddPermanentWidget adds a widget to the right side of the status bar
// (e.g. a progress bar, label, or indicator).
func (this *StatusBar) AddPermanentWidget(iw IWidget) {
	iw.SetParent(this)
	this.permanent = append(this.permanent, iw)
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
}

// RemovePermanentWidget removes a permanent widget.
func (this *StatusBar) RemovePermanentWidget(iw IWidget) {
	iw.SetParent(nil)
	for i, v := range this.permanent {
		if v == iw {
			copy(this.permanent[i:], this.permanent[i+1:])
			this.permanent[len(this.permanent)-1] = nil
			this.permanent = this.permanent[:len(this.permanent)-1]
			break
		}
	}
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
}

// PermanentWidgets returns the list of permanent widgets.
func (this *StatusBar) PermanentWidgets() []IWidget {
	return this.permanent
}

// Layout positions the permanent widgets on the right side.
func (this *StatusBar) Layout() {
	w, h := this.Self().Size()
	t := Theme()
	m := t.ButtonMargin

	// lay out permanent widgets from the right
	x := w - m.R
	for i := len(this.permanent) - 1; i >= 0; i-- {
		pw := this.permanent[i]
		if !pw.IsVisible() {
			continue // hidden permanent widgets reserve no space
		}
		hints := pw.SizeHints()
		iw := hints.Width
		ih := hints.Height
		if (hints.Policy & (GrowVertical | ExpandVertical)) != 0 {
			ih = h - 2 // leave room for top border
		}
		y := 1 + (h-1-ih)*0.5 // 1px top border offset
		if y < 1 {
			y = 1
		}
		x -= iw
		pw.SetBounds(x, y, iw, ih)
		x -= this.spacing
	}
}

func (this *StatusBar) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Self().Size()

	// background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(t.FormColor)
	g.Fill()

	// thin top border line
	g.Line(0, 0.5, w, 0.5)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// draw message text on the left
	if this.message != "" {
		m := t.ButtonMargin
		g.SetFont(t.Font)
		g.SetBrush1(t.TextColor)
		ext := t.Font.TextExtents(this.message)
		fe := t.Font.FontExtents()
		_ = fe
		xt := m.L - ext.XBearing
		yt := 1 + (h-1+ext.YBearing)*0.5 - ext.YBearing
		g.Translate(xt, yt)
		g.DrawText(this.message)
		g.Translate(-xt, -yt)
	}
}

func (this *StatusBar) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	m := t.ButtonMargin
	h := fe.Height + m.T + m.B
	if h < 22 {
		h = 22
	}
	var pw float64
	for _, w := range this.permanent {
		if !w.IsVisible() {
			continue // hidden permanent widgets are excluded from the width sum
		}
		hints := w.SizeHints()
		pw += hints.Width + this.spacing
	}
	w := math.Max(pw+m.L+m.R, 100)
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal}
}

func (this *StatusBar) OnIdle() {
	for _, v := range this.permanent {
		if v.IsVisible() {
			if im, ok := v.(interface {
				OnIdle()
			}); ok {
				im.OnIdle()
			}
		}
	}
}
