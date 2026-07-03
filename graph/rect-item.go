package graph

import (
	"github.com/uk0/silk/paint"
)

func NewRectItem() *RectItem {
	p := new(RectItem)
	p.Init(p)
	return p
}

type RectItem struct {
	Item
}

func (this *RectItem) DrawSelf(g paint.Painter) {
	//g.Save()
	//defer g.Restore()
	//panic("12")
	//drawErrCross(g, this.X(), this.Y(), this.Width(), this.Height())
	g.Rectangle(this.X(), this.Y(), this.Width(), this.Height())
	//g.SetLineWidth(1)
	//g.SetSourceRGB(0, 0, 0)
	g.SetPen1(paint.Color{0, 0, 0, 255}, 1)

	g.Stroke()
}
