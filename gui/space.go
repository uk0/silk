package gui

import (
	"github.com/uk0/silk/core"
	//	"github.com/uk0/silk/factory"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.Space", core.TypeOf((*Space)(nil))) //((*Space)(nil)))
}

// 空格
// 此控件除占用空白区域以外, 没有其他作用
// 可以设置为带有弹性, 如果布局器支持, 有弹性表现为看不见的弹簧, 占用尽可能多的空间
type Space struct {
	Widget
	vertical bool
	expand   bool
	min      float64
}

func NewSpace(vertical, expand bool) *Space {
	p := new(Space)
	p.Init(p)
	p.vertical = vertical
	p.expand = expand
	return p
}

func (this *Space) Draw(cc paint.Painter) {
	if core.IsDebugOn() {
		const n = 20
		da := 3.14 / 4.0
		cc.SetBrush1(paint.Color{0, 0, 255, 64})
		//cc.SetLineWidth(0.5)
		if this.vertical {
			h := this.h - 1
			if h < 1 {
				return
			}
			dy := h / n / 8.0
			hw := this.w * 0.5
			cc.MoveTo(hw, 0)
			r := hw * 0.7
			a := 0.0
			for y := 0.0; y <= this.h; y += dy {
				cc.LineTo(hw+math.Sin(a)*r, y)
				a += da
			}
			cc.Stroke()
		} else {
			w := this.w - 1
			if w < 1 {
				return
			}
			dx := w / n / 8.0
			hh := this.h * 0.5
			cc.MoveTo(0, hh)
			r := hh * 0.7
			a := 0.0
			for x := 0.0; x <= this.w; x += dx {
				cc.LineTo(x, hh+math.Sin(a)*r)
				a += da
			}
			cc.Stroke()
		}
	}
}

func (this *Space) SizeHints() SizeHints {
	p := GrowVertical | GrowHorizontal
	if this.vertical {
		if this.expand {
			p |= ExpandVertical
		}
		return SizeHints{Width: 0, Height: this.min, Policy: p}
	} else {
		if this.expand {
			p |= ExpandHorizontal
		}
		return SizeHints{Width: this.min, Height: 0, Policy: p}
	}
}

func (this *Space) SetMinSize(min float64) {
	this.min = min
}

func (this *Space) MinSize() float64 {
	return this.min
}
