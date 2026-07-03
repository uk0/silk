package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

// AlertLevel represents the severity level of an Alert banner. It mirrors
// the four-level scheme used by Toast (info / success / warning / error) so
// the visual language stays consistent across transient and inline messages.
type AlertLevel int

const (
	AlertInfo    AlertLevel = iota // Blue
	AlertSuccess                   // Green
	AlertWarning                   // Amber
	AlertError                     // Red
)

// Alert 内联提示横幅控件，在布局中常驻显示一条带级别和图标的状态信息。
// 与 Toast 不同：Toast 是浮层、定时自动消失；Alert 固定在布局里作为状态提示，
// 可选标题、可选关闭按钮。
type Alert struct {
	Widget
	level     AlertLevel
	message   string
	title     string
	closeable bool
	cbClose   func()
}

func init() {
	core.RegisterFactory("gui.Alert", core.TypeOf((*Alert)(nil)))
}

// NewAlert creates an inline message banner at the given level with the
// supplied message text. Title and the close button are off by default.
func NewAlert(level AlertLevel, message string) *Alert {
	p := new(Alert)
	p.Init(p)
	p.level = level
	p.message = message
	return p
}

func (this *Alert) Level() AlertLevel { return this.level }
func (this *Alert) Message() string   { return this.message }
func (this *Alert) Title() string     { return this.title }
func (this *Alert) IsCloseable() bool { return this.closeable }

func (this *Alert) SetLevel(l AlertLevel) {
	this.level = l
	this.Self().Update()
}

func (this *Alert) SetMessage(s string) {
	this.message = s
	this.Self().Update()
}

func (this *Alert) SetTitle(s string) {
	this.title = s
	this.Self().Update()
}

func (this *Alert) SetCloseable(b bool) {
	this.closeable = b
	this.Self().Update()
}

// SigClose sets the callback fired when the user clicks the × button. The
// alert hides itself before the callback runs.
func (this *Alert) SigClose(fn func()) {
	this.cbClose = fn
}

// alertColors returns the soft tint used for the banner fill and the full
// accent colour used for the left bar, icon and title. The tints are light
// versions of the matching Toast level colours.
func alertColors(level AlertLevel) (fill paint.Color, accent paint.Color) {
	switch level {
	case AlertInfo:
		fill = paint.Color{232, 244, 253, 255}  // light blue
		accent = paint.Color{33, 150, 243, 255} // blue
	case AlertSuccess:
		fill = paint.Color{232, 245, 233, 255} // light green
		accent = paint.Color{76, 175, 80, 255} // green
	case AlertWarning:
		fill = paint.Color{255, 243, 224, 255} // light amber
		accent = paint.Color{255, 152, 0, 255} // amber
	case AlertError:
		fill = paint.Color{253, 235, 234, 255} // light red
		accent = paint.Color{244, 67, 54, 255} // red
	default:
		fill = paint.Color{240, 240, 240, 255}
		accent = paint.Color{120, 120, 120, 255}
	}
	return
}

// --- Events ---

func (this *Alert) OnLeftDown(x, y float64) {
	if !this.closeable {
		return
	}
	w, _ := this.Size()
	// Close button sits in the top-right corner.
	closeX := w - alertPadding - alertCloseR
	closeY := alertPadding + alertCloseR
	if x >= closeX-alertCloseR && x <= closeX+alertCloseR &&
		y >= closeY-alertCloseR && y <= closeY+alertCloseR {
		this.Self().SetVisible(false)
		if this.cbClose != nil {
			this.cbClose()
		}
	}
}

// --- Drawing ---

const (
	alertPadding = 12.0 // inner margin around content
	alertBarW    = 4.0  // left accent bar width
	alertIconR   = 8.0  // level glyph radius
	alertCloseR  = 7.0  // close button hit radius
	alertGap     = 12.0 // gap between icon and text
)

func (this *Alert) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	fill, accent := alertColors(this.level)
	r := 6.0

	g.Save()

	// Rounded-rect background tinted by level.
	roundedRect(g, 0, 0, w, h, r)
	g.SetBrush1(fill)
	g.Fill()

	// Colored left accent bar (rounded left corners only).
	roundedRect(g, 0, 0, alertBarW+r, h, r)
	g.SetBrush1(accent)
	g.Fill()
	g.Rectangle(alertBarW, 0, r, h)
	g.SetBrush1(fill)
	g.Fill()

	// Level glyph centred vertically against the top text row.
	iconX := alertBarW + alertPadding + alertIconR
	iconY := alertPadding + alertIconR
	this.drawIcon(g, iconX, iconY, accent)

	// Text block to the right of the icon.
	textX := iconX + alertIconR + alertGap
	textY := alertPadding
	if this.title != "" {
		titleFont := paint.NewFont(t.Font.Family(), 13, true, false)
		g.SetFont(titleFont)
		g.SetBrush1(accent)
		fe := titleFont.FontExtents()
		g.DrawText1(textX, textY+fe.Ascent, this.title)
		textY += fe.Height + 2
	}

	msgFont := paint.NewFont(t.Font.Family(), 13, false, false)
	g.SetFont(msgFont)
	g.SetBrush1(t.TextColor)
	mfe := msgFont.FontExtents()
	g.DrawText1(textX, textY+mfe.Ascent, this.message)

	// Close button (×) in the top-right corner.
	if this.closeable {
		cx := w - alertPadding - alertCloseR
		cy := alertPadding + alertCloseR
		d := 3.5
		g.SetPen1(accent, 1.5)
		g.MoveTo(cx-d, cy-d)
		g.LineTo(cx+d, cy+d)
		g.MoveTo(cx+d, cy-d)
		g.LineTo(cx-d, cy+d)
		g.Stroke()
	}

	g.Restore()
}

// drawIcon paints a simple per-level glyph (i / ✓ / ! / ✕) using basic
// shapes, in white over a filled accent disc.
func (this *Alert) drawIcon(g paint.Painter, cx, cy float64, accent paint.Color) {
	white := paint.Color{255, 255, 255, 255}
	g.Arc(cx, cy, alertIconR, 0, 2*math.Pi)
	g.SetBrush1(accent)
	g.Fill()

	switch this.level {
	case AlertInfo:
		// "i": dot + stem
		g.Arc(cx, cy-3, 1.3, 0, 2*math.Pi)
		g.SetBrush1(white)
		g.Fill()
		g.MoveTo(cx, cy-1)
		g.LineTo(cx, cy+4)
		g.SetPen1(white, 2)
		g.Stroke()
	case AlertSuccess:
		// checkmark
		g.MoveTo(cx-4, cy)
		g.LineTo(cx-1, cy+3)
		g.LineTo(cx+4, cy-3)
		g.SetPen1(white, 2)
		g.Stroke()
	case AlertWarning:
		// "!": stem + dot
		g.MoveTo(cx, cy-4)
		g.LineTo(cx, cy+1)
		g.SetPen1(white, 2)
		g.Stroke()
		g.Arc(cx, cy+4, 1.2, 0, 2*math.Pi)
		g.SetBrush1(white)
		g.Fill()
	case AlertError:
		// "✕"
		g.MoveTo(cx-3.5, cy-3.5)
		g.LineTo(cx+3.5, cy+3.5)
		g.MoveTo(cx+3.5, cy-3.5)
		g.LineTo(cx-3.5, cy+3.5)
		g.SetPen1(white, 2)
		g.Stroke()
	}
}

// --- SizeHints ---

func (this *Alert) SizeHints() SizeHints {
	t := Theme()
	titleFont := paint.NewFont(t.Font.Family(), 13, true, false)
	msgFont := paint.NewFont(t.Font.Family(), 13, false, false)
	mfe := msgFont.FontExtents()

	// Text column starts after the accent bar, padding, icon and gap.
	textLeft := alertBarW + alertPadding + 2*alertIconR + alertGap
	msgExt := msgFont.TextExtents(this.message)
	textW := msgExt.Width

	contentH := mfe.Height
	if this.title != "" {
		tfe := titleFont.FontExtents()
		titleExt := titleFont.TextExtents(this.title)
		if titleExt.Width > textW {
			textW = titleExt.Width
		}
		contentH += tfe.Height + 2
	}

	w := textLeft + textW + alertPadding
	if this.closeable {
		w += 2*alertCloseR + alertGap
	}
	h := contentH + 2*alertPadding

	// Icon needs a minimum height regardless of a short single text row.
	if minH := 2*alertIconR + 2*alertPadding; h < minH {
		h = minH
	}
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal}
}

func (this *Alert) EnumProperties(list core.IPropertyList) {
	list.AddProperty("级别", this.levelIndex, this.setLevelIndex)
	list.AddProperty("标题", this.Title, this.SetTitle)
	list.AddProperty("内容", this.Message, this.SetMessage)
	list.AddProperty("可关闭", this.IsCloseable, this.SetCloseable)
}

// levelIndex / setLevelIndex adapt the AlertLevel enum to the int-based
// property editor, matching how other widgets expose enum fields.
func (this *Alert) levelIndex() int     { return int(this.level) }
func (this *Alert) setLevelIndex(i int) { this.SetLevel(AlertLevel(i)) }
