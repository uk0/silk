package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	//	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.Separator", core.TypeOf((*Separator)(nil))) //((*Separator)(nil)))
}

// 分隔线
// 通常用来分隔两个相邻菜单按钮
// 分隔线表现形式是一条线, 宽度大于高度时是横线, 反之是竖线
type Separator struct {
	Widget
}

func NewSeparator() *Separator {
	p := new(Separator)
	p.Init(p)
	return p
}

func (this *Separator) Draw(cc paint.Painter) {
	Theme().DrawSeperator(cc, this.w, this.h, this.Height() > this.Width())
}

func (this *Separator) SizeHints() SizeHints {
	sz := Theme().SeparatorSize
	return SizeHints{Height: sz, Width: sz, Policy: GrowVertical | GrowHorizontal}
}
