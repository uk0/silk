package gui

import (
	"silk/core"
	"silk/paint"
	"math"
	"strings"
)

// AvatarShape 头像形状
type AvatarShape int

const (
	AvatarCircle AvatarShape = iota
	AvatarSquare
)

// Avatar 头像控件，显示图片或文字首字母
type Avatar struct {
	Widget
	pixmap  paint.Pixmap
	text    string
	bgColor paint.Color
	shape   AvatarShape
	size    float64
}

func init() {
	core.RegisterFactory("gui.Avatar", core.TypeOf((*Avatar)(nil)))
}

func NewAvatar() *Avatar {
	p := new(Avatar)
	p.Init(p)
	p.bgColor = paint.Color{66, 133, 244, 255}
	p.shape = AvatarCircle
	p.size = 40
	return p
}

func (this *Avatar) SetPixmap(pm paint.Pixmap) { this.pixmap = pm; this.Self().Update() }
func (this *Avatar) Pixmap() paint.Pixmap      { return this.pixmap }
func (this *Avatar) Text() string              { return this.text }
func (this *Avatar) AvatarSize() float64       { return this.size }
func (this *Avatar) Shape() AvatarShape        { return this.shape }

func (this *Avatar) SetText(s string) {
	this.text = s
	this.Self().Update()
}

func (this *Avatar) SetBgColor(c paint.Color) {
	this.bgColor = c
	this.Self().Update()
}

func (this *Avatar) SetShape(s AvatarShape) {
	this.shape = s
	this.Self().Update()
}

func (this *Avatar) SetAvatarSize(s float64) {
	this.size = s
	this.Self().Update()
}

func (this *Avatar) initials() string {
	if this.text == "" {
		return ""
	}
	parts := strings.Fields(this.text)
	if len(parts) >= 2 {
		r1 := []rune(parts[0])
		r2 := []rune(parts[1])
		return string(r1[0:1]) + string(r2[0:1])
	}
	r := []rune(this.text)
	if len(r) >= 2 {
		return string(r[0:2])
	}
	return string(r[0:1])
}

func (this *Avatar) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Self().Size()
	s := math.Min(w, h)
	if s <= 0 {
		s = this.size
	}
	cx, cy := s/2, s/2

	g.Save()

	if this.shape == AvatarCircle {
		g.Arc(cx, cy, s/2, 0, 2*math.Pi)
	} else {
		r := 4.0
		g.MoveTo(r, 0)
		g.LineTo(s-r, 0)
		g.Arc(s-r, r, r, -math.Pi/2, 0)
		g.LineTo(s, s-r)
		g.Arc(s-r, s-r, r, 0, math.Pi/2)
		g.LineTo(r, s)
		g.Arc(r, s-r, r, math.Pi/2, math.Pi)
		g.LineTo(0, r)
		g.Arc(r, r, r, math.Pi, 3*math.Pi/2)
		g.LineTo(r, 0)
	}

	if this.pixmap != nil {
		g.SetBrush1(paint.Color{240, 240, 240, 255})
		g.FillPreserve()
		g.Clip()
		imgW := float64(this.pixmap.Width())
		imgH := float64(this.pixmap.Height())
		scale := math.Max(s/imgW, s/imgH)
		dx := (s - imgW*scale) / 2
		dy := (s - imgH*scale) / 2
		g.Translate(dx, dy)
		g.Scale(scale, scale)
		g.DrawPixmap(this.pixmap)
	} else {
		g.SetBrush1(this.bgColor)
		g.Fill()

		// draw initials
		initials := strings.ToUpper(this.initials())
		if initials != "" {
			g.SetFont(t.Font)
			g.SetBrush1(paint.Color{255, 255, 255, 255})
			ext := t.Font.TextExtents(initials)
			tx := (s - ext.Width) / 2 - ext.XBearing
			ty := 0.5*(s+ext.YBearing) - ext.YBearing
			g.Translate(tx, ty)
			g.DrawText(initials)
			g.Translate(-tx, -ty)
		}
	}

	g.Restore()
}

func (this *Avatar) SizeHints() SizeHints {
	return SizeHints{Width: this.size, Height: this.size, Policy: 0}
}

func (this *Avatar) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("大小", this.AvatarSize, this.SetAvatarSize)
	list.AddProperty("形状", this.Shape, this.SetShape)
}
