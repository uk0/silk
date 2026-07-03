package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
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

// statusIconLabelIconSize is the compact glyph size used by StatusIconLabel
// cells — a touch smaller than Theme().IconSize so the icon reads as an inline
// prefix to the text rather than a full-size button glyph.
const statusIconLabelIconSize = 14.0

// StatusIconLabel is a compact status-bar cell that draws a small icon followed
// by a short text, both vertically centered. It is a permanent widget like any
// other (add it with AddPermanentWidget or the AddIconLabel helper), letting
// callers show IDE-style indicators — a branch glyph + branch name, an error or
// warning glyph + count — in place of a plain text label. An empty icon name
// draws text only; empty text draws the icon only.
type StatusIconLabel struct {
	Widget
	icon string // icon resource name for LoadIcon; "" means text-only
	text string
}

// NewStatusIconLabel creates an icon+text status cell. icon is an icon resource
// name (e.g. "git-branch", "error", "warning"); text is the label after it.
func NewStatusIconLabel(icon string, text string) *StatusIconLabel {
	p := new(StatusIconLabel)
	p.Init(p)
	p.icon = icon
	p.text = text
	return p
}

// Icon returns the cell's icon resource name.
func (this *StatusIconLabel) Icon() string {
	return this.icon
}

// Text returns the cell's text.
func (this *StatusIconLabel) Text() string {
	return this.text
}

// SetIcon changes the icon resource name and re-lays out the parent bar (the
// cell's width depends on whether an icon is present).
func (this *StatusIconLabel) SetIcon(name string) {
	if this.icon == name {
		return
	}
	this.icon = name
	this.InvalidateParentLayout()
}

// SetText changes the label text and re-lays out the parent bar, since the
// cell's width tracks the text width.
func (this *StatusIconLabel) SetText(text string) {
	if this.text == text {
		return
	}
	this.text = text
	this.InvalidateParentLayout()
}

// statusIconLabelWidth computes a StatusIconLabel's content width from its
// parts: the icon box, the icon→text gap (counted only when both are present),
// and the measured text width. Kept free of painter/Theme access so the layout
// math stays unit-testable.
func statusIconLabelWidth(iconW, gap, textW float64, hasIcon, hasText bool) float64 {
	var w float64
	if hasIcon {
		w += iconW
	}
	if hasIcon && hasText {
		w += gap
	}
	if hasText {
		w += textW
	}
	return w
}

// SizeHints reports the cell's natural size: icon + gap + text wide, and tall
// enough for the taller of the font line and the icon.
func (this *StatusIconLabel) SizeHints() SizeHints {
	t := Theme()
	var textW float64
	if this.text != "" {
		textW = t.Font.TextExtents(this.text).Width
	}
	w := statusIconLabelWidth(statusIconLabelIconSize, t.Spacing, textW, this.icon != "", this.text != "")
	fe := t.Font.FontExtents()
	h := math.Max(fe.Height, statusIconLabelIconSize)
	return SizeHints{Width: w, Height: h, Policy: 0}
}

// Draw renders the icon on the left and the text after it, both vertically
// centered. Colors come from the theme so the cell tracks light/dark mode.
func (this *StatusIconLabel) Draw(g paint.Painter) {
	t := Theme()
	_, h := this.Self().Size()

	x := 0.0
	if this.icon != "" {
		yi := (h - statusIconLabelIconSize) * 0.5
		g.DrawIcon1(LoadIcon(this.icon), x, yi, statusIconLabelIconSize, false)
		x += statusIconLabelIconSize
		if this.text != "" {
			x += t.Spacing
		}
	}
	if this.text != "" {
		g.SetFont(t.Font)
		g.SetBrush1(t.TextColor)
		ext := t.Font.TextExtents(this.text)
		xt := x - ext.XBearing
		yt := 0.5*(h+ext.YBearing) - ext.YBearing
		g.Translate(xt, yt)
		g.DrawText(this.text)
		g.Translate(-xt, -yt)
	}
}

// AddIconLabel creates a StatusIconLabel (icon + text) and adds it as a
// permanent widget on the right, returning it so the caller can update it later
// via SetText / SetIcon. icon is an icon resource name (e.g. "git-branch").
func (this *StatusBar) AddIconLabel(icon string, text string) *StatusIconLabel {
	cell := NewStatusIconLabel(icon, text)
	this.AddPermanentWidget(cell)
	return cell
}
