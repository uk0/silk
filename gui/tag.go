package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

// Tag 标签控件，用于显示标签/状态/分类
type Tag struct {
	Widget
	text       string
	color      paint.Color
	textColor  paint.Color
	closeable  bool
	hoverClose bool
	cbClose    func()
}

func init() {
	core.RegisterFactory("gui.Tag", core.TypeOf((*Tag)(nil)))
}

// tagTextPadding is the room the pill keeps either side of the label;
// tagCloseWidth is the slot on the right the close cross owns. SizeHints
// reserves both, Draw clips the label to what is left of them and
// OnMouseMove treats the slot as the close hit region — they have to stay
// one number or a squeezed tag runs its text through the cross.
const (
	tagTextPadding = 8.0
	tagCloseWidth  = 18.0
)

func NewTag(text string) *Tag {
	p := new(Tag)
	p.Init(p)
	p.text = text
	return p
}

// Init carries the visual defaults, not NewTag: the designer and the .tdoc
// loader build widgets through the core factory, which reflects on Init and
// never sees the constructor. Colours left in NewTag render a transparent tag.
func (this *Tag) Init(self IWidget) {
	this.Widget.Init(self)
	this.color = paint.Color{66, 133, 244, 255}      // blue
	this.textColor = paint.Color{255, 255, 255, 255} // white
}

func (this *Tag) Text() string       { return this.text }
func (this *Tag) Color() paint.Color { return this.color }

func (this *Tag) SetText(s string) {
	this.text = s
	this.Self().Update()
}

func (this *Tag) SetColor(c paint.Color) {
	this.color = c
	this.Self().Update()
}

func (this *Tag) SetTextColor(c paint.Color) {
	this.textColor = c
	this.Self().Update()
}

func (this *Tag) IsCloseable() bool { return this.closeable }

func (this *Tag) SetCloseable(b bool) {
	this.closeable = b
	this.Self().Update()
}

func (this *Tag) SigClose(fn func()) {
	this.cbClose = fn
}

// --- Events ---

func (this *Tag) OnMouseEnter() {
	this.Self().Update()
}

func (this *Tag) OnMouseLeave() {
	this.hoverClose = false
	this.Self().Update()
}

func (this *Tag) OnMouseMove(x, y float64) {
	if !this.closeable {
		return
	}
	w, _ := this.Size()
	was := this.hoverClose
	this.hoverClose = x >= w-tagCloseWidth
	if was != this.hoverClose {
		this.Self().Update()
	}
}

func (this *Tag) OnLeftDown(x, y float64) {
	if this.closeable && this.hoverClose {
		if this.cbClose != nil {
			this.cbClose()
		}
	}
}

// --- Drawing ---

func (this *Tag) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	r := h / 2
	if r > 10 {
		r = 10
	}

	// rounded rect background
	g.Save()
	g.MoveTo(r, 0)
	g.LineTo(w-r, 0)
	g.Arc(w-r, r, r, -math.Pi/2, 0)
	g.LineTo(w, h-r)
	g.Arc(w-r, h-r, r, 0, math.Pi/2)
	g.LineTo(r, h)
	g.Arc(r, h-r, r, math.Pi/2, math.Pi)
	g.LineTo(0, r)
	g.Arc(r, r, r, math.Pi, 3*math.Pi/2)
	g.LineTo(r, 0)

	g.SetBrush1(this.color)
	g.Fill()

	// text, bounded by the room SizeHints asked for so a tag laid out
	// below its hint loses the tail of the label instead of painting it
	// through the close cross
	f := t.Font
	g.SetFont(f)
	ext := f.TextExtents(this.text)
	tx := tagTextPadding
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	textW := w - tx - tagTextPadding
	if this.closeable {
		textW = w - tx - tagCloseWidth
	}
	if textW < 0 {
		textW = 0
	}
	g.Save()
	g.Rectangle(tx, 0, textW, h)
	g.Clip()
	g.SetBrush1(this.textColor)
	g.Translate(tx-ext.XBearing, ty)
	g.DrawText(this.text)
	g.Restore()

	// close button
	if this.closeable {
		cx := w - 12
		cy := h / 2
		cr := 3.0
		if this.hoverClose {
			g.SetPen1(paint.Color{255, 255, 255, 255}, 1.5)
		} else {
			g.SetPen1(paint.Color{255, 255, 255, 180}, 1)
		}
		g.MoveTo(cx-cr, cy-cr)
		g.LineTo(cx+cr, cy+cr)
		g.Stroke()
		g.MoveTo(cx+cr, cy-cr)
		g.LineTo(cx-cr, cy+cr)
		g.Stroke()
	}

	g.Restore()
}

func (this *Tag) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	ext := t.Font.TextExtents(this.text)
	w := ext.Width + tagTextPadding*2
	if this.closeable {
		w += tagCloseWidth
	}
	h := fe.Height + 8
	return SizeHints{Width: w, Height: h, Policy: 0}
}

func (this *Tag) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("可关闭", this.IsCloseable, this.SetCloseable)
}
