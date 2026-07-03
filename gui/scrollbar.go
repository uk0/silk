package gui

import (
	"github.com/uk0/silk/core"
	//	"github.com/uk0/silk/factory"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.ScrollBar", core.TypeOf((*ScrollBar)(nil)))
}

func NewScrollBar() *ScrollBar {
	p := new(ScrollBar)
	p.Init(p)
	return p
}

type ScrollPart int

const (
	SCROLL_SMALL_DEC ScrollPart = iota
	SCROLL_SMALL_INC
	SCROLL_LARGE_DEC
	SCROLL_LARGE_INC
	SCROLL_TRACK_BAR
)

// scrollPartKind 把滚动条轴向上的一个坐标归类到 Qt 风格的交互"部件".
// 抽成纯函数(不依赖 GL/主题/控件状态)便于单元测试; 整型 PointToPart(1..5) 由它派生.
type scrollPartKind int

const (
	partArrowDec    scrollPartKind = iota // 起始箭头按钮(上/左)
	partBeforeThumb                       // 滑块前方的空白槽 -> 向后翻页
	partOnThumb                           // 可拖动的滑块本体
	partAfterThumb                        // 滑块后方的空白槽 -> 向前翻页
	partArrowInc                          // 末端箭头按钮(下/右)
)

// scrollArrowSize is the length reserved at each rail end for the legacy
// step-arrow buttons. The modern bar draws no arrows, so the zone is 0: the
// whole rail is trough + thumb, end clicks page like any trough click, and
// the arrow parts (1/5) are unreachable from hit-testing. Wheel/keyboard
// line-stepping still goes through SmallBakward/SmallForward directly.
const scrollArrowSize = 0.0

// scrollMinThumb is the minimum thumb length along the scroll axis, so huge
// documents still get a grabbable pill instead of a sliver.
const scrollMinThumb = 24.0

// scrollPart 把轴向坐标 pos(竖直取 y, 水平取 x)映射为 scrollPartKind.
// thumbStart/thumbLen 为滑块在该轴上的绝对偏移与长度(thumbStart 已含起始箭头内缩);
// trackLen 为控件在该轴上的总长度, arrowSize 为单个端部箭头按钮长度(若不绘制则传 0).
// 优先级与原 PointToPart 一致: 滑块优先于箭头, 箭头优先于两侧空白槽.
func scrollPart(pos, thumbStart, thumbLen, trackLen, arrowSize float64) scrollPartKind {
	if pos >= thumbStart && pos < thumbStart+thumbLen {
		return partOnThumb
	}
	if arrowSize > 0 {
		if pos < arrowSize {
			return partArrowDec
		}
		if pos >= trackLen-arrowSize {
			return partArrowInc
		}
	}
	if pos < thumbStart {
		return partBeforeThumb
	}
	return partAfterThumb
}

type IOnHorzScroll interface {
	OnHorzScroll(sender IWidget)
}

type IOnVertScroll interface {
	OnVertScroll(sender IWidget)
}

// 滚动条
type ScrollBar struct {
	Widget
	vert        bool
	pushed      bool
	autoHide    bool
	part        int
	min         float64
	max         float64
	small       float64
	large       float64
	value       float64
	timer       Timer
	tc          int
	mouseValue  float64
	mouseOffset float64
	changed     func(IWidget)
}

func (this *ScrollBar) IsAutoHide() bool {
	return this.autoHide
}

func (this *ScrollBar) SetAutoHide(b bool) {
	this.autoHide = b
}

func (this *ScrollBar) Draw(g paint.Painter) {
	Theme().DrawScroll(g, this)
}

func (this *ScrollBar) OnMouseEnter() {
	this.Self().Update()
}

func (this *ScrollBar) OnMouseLeave() {
	if !this.pushed {
		this.part = 0
	}
	this.Self().Update()
}

func (this *ScrollBar) onTimer() {
	this.tc++
	if this.tc > 1 && this.tc < 4 {
		return
	}
	switch this.part {
	case 1:
		this.SmallBakward()
	case 2:
		this.largeBakward(true)
	//case 3:
	//	this.drag()
	case 4:
		this.largeForward(true)
	case 5:
		this.SmallForward()
	}
}

func (this *ScrollBar) OnLeftDown(x, y float64) {
	this.pushed = true
	this.part = this.PointToPart(x, y)
	if this.part == 3 {
		tx, ty, tw, th := this.TrackRect()
		if this.IsVertical() {
			this.mouseOffset = ty + th*0.5 - y
		} else {
			this.mouseOffset = tx + tw*0.5 - x
		}
		this.mouseValue = this.PointToValue(x+this.mouseOffset, y+this.mouseOffset)
		this.onTimer()
	} else {
		this.mouseOffset = 0
		this.timer.Start(90, this.onTimer)
		this.tc = 0
		this.mouseValue = this.PointToValue(x, y)
		this.onTimer()
	}

	//core.Debug(this.part)
	this.Self().Update()
}

func (this *ScrollBar) OnLeftUp(x, y float64) {
	this.timer.Stop()
	this.pushed = false
	if x < 0 || y < 0 || x >= this.w || y >= this.h {
		this.part = 0
	} else {
		this.part = this.PointToPart(x, y)
	}
	this.Self().Update()
}

func (this *ScrollBar) OnMouseMove(x, y float64) {
	if this.pushed {
		this.mouseValue = this.PointToValue(x+this.mouseOffset, y+this.mouseOffset)
		if this.part == 3 {
			this.drag()
		}
	} else {
		part := this.PointToPart(x, y)
		if part != this.part {
			this.part = part
			this.Self().Update()
		}
	}
}

func (this *ScrollBar) SetRange(min, max float64) {
	if max < min {
		core.Warn("illegal range [", min, ", ", max, "], treat as [", min, ", ", min, "]")
		max = min
	}
	this.min = min
	this.max = max
	if this.large == 0 {
		this.large = 0.1 * (max - min)
	}
	if this.small == 0 {
		this.small = 0.1 * (max - min)
	}
	this.SetValue(this.value)
}
func (this *ScrollBar) Range() (min, max float64) {
	min = this.min
	max = this.max
	return
}

func (this *ScrollBar) SetDelta(small, large float64) {
	this.small = small
	this.large = large
}

func (this *ScrollBar) Delta() (small, large float64) {
	small = this.small
	large = this.large
	return
}

func (this *ScrollBar) SetValue(v float64) {
	if v < this.min {
		v = this.min
	} else if v > this.max {
		v = this.max
	}
	if v != this.value {
		this.value = v
		this.Self().Update()
		this.emitChanged()
	}
}

func (this *ScrollBar) Value() (value float64) {
	value = this.value
	return
}

func (this *ScrollBar) SetVertical(vert bool) {
	this.vert = vert
	this.Self().Update()
}

func (this *ScrollBar) IsVertical() bool {
	return this.vert
}

func (this *ScrollBar) TrackRect() (x, y, w, h float64) {
	if this.IsVertical() {
		h1 := this.h - scrollArrowSize*2
		if !this.IsValid() {
			x, y, w, h = 0, scrollArrowSize, this.w, h1
			return
		}
		ts := h1 * this.large / (this.large + this.max - this.min)
		if ts < scrollMinThumb {
			ts = scrollMinThumb
		}
		if ts > h1 {
			ts = h1
		}
		y = scrollArrowSize + (h1-ts)*this.value/(this.max-this.min)
		x = 0
		w = this.w
		h = ts
	} else {
		w1 := this.w - scrollArrowSize*2
		if !this.IsValid() {
			x, y, w, h = scrollArrowSize, 0, w1, this.h
			return
		}
		ts := w1 * this.large / (this.large + this.max - this.min)
		if ts < scrollMinThumb {
			ts = scrollMinThumb
		}
		if ts > w1 {
			ts = w1
		}
		x = scrollArrowSize + (w1-ts)*this.value/(this.max-this.min)
		y = 0
		w = ts
		h = this.h
	}
	return
}

func (this *ScrollBar) PointToValue(x, y float64) float64 {
	if !this.IsValid() {
		return 0
	}
	var v float64
	if this.IsVertical() {
		h1 := this.h - scrollArrowSize*2
		v = this.min + (this.large+this.max-this.min)*(y-scrollArrowSize)/h1

	} else {
		w1 := this.w - scrollArrowSize*2
		v = this.min + (this.large+this.max-this.min)*(x-scrollArrowSize)/w1
	}
	if v < this.min {
		v = this.min
	}
	if v > this.max+this.large {
		v = this.max + this.large
	}
	return v
}

func (this *ScrollBar) ActivePart() (part int, pushed bool) {
	return this.part, this.pushed
}

func (this *ScrollBar) PointToPart(x, y float64) int {
	if !this.IsValid() {
		return 0
	}
	tx, ty, tw, th := this.TrackRect()
	// 取轴向上的坐标/滑块/轨道长度, 复用纯函数分类(端部不再绘制箭头按钮, 故
	// arrowSize = 0: 整条轨道即滑槽, 部件 1/5 不可达, 端部点击同空槽翻页).
	var kind scrollPartKind
	if this.IsVertical() {
		kind = scrollPart(y, ty, th, this.h, scrollArrowSize)
	} else {
		kind = scrollPart(x, tx, tw, this.w, scrollArrowSize)
	}
	// 映射回既有的 1..5 部件编号(onTimer 依赖之).
	switch kind {
	case partArrowDec:
		return 1
	case partBeforeThumb:
		return 2
	case partOnThumb:
		return 3
	case partAfterThumb:
		return 4
	default: // partArrowInc
		return 5
	}
}

func (this *ScrollBar) SmallBakward() {
	if !this.IsValid() {
		return
	}
	v := this.value - this.small
	this.SetValue(v)
}

func (this *ScrollBar) SmallForward() {
	if !this.IsValid() {
		return
	}
	v := this.value + this.small
	this.SetValue(v)

}

func (this *ScrollBar) LargeBakward() {
	this.largeBakward(false)
}

func (this *ScrollBar) largeBakward(mouseLimit bool) {
	if !this.IsValid() {
		return
	}
	limit := this.min
	if mouseLimit {
		limit = this.mouseValue - this.large*0.5
		if limit < this.min {
			limit = this.min
		}
	}
	if this.value <= limit {
		return
	}
	v := this.value - this.large
	if v <= limit {
		v = limit
	}
	if v != this.value {
		this.value = v
		this.Self().Update()
		this.emitChanged()
	}
}
func (this *ScrollBar) LargeForward() {
	this.largeForward(false)
}

func (this *ScrollBar) largeForward(mouseLimit bool) {
	if !this.IsValid() {
		return
	}

	limit := this.max
	if mouseLimit {
		limit = this.mouseValue - this.large*0.5
		if limit > this.max {
			limit = this.max
		}
	}
	if this.value >= limit {
		return
	}

	v := this.value + this.large
	if v >= limit {
		v = limit
	}
	if v != this.value {
		this.value = v
		this.Self().Update()
		this.emitChanged()
	}
}

func (this *ScrollBar) drag() {
	if !this.IsValid() {
		return
	}
	v := this.mouseValue - this.large*0.5
	this.SetValue(v)
}

func (this *ScrollBar) SizeHints() (hints SizeHints) {
	hints.Height = Theme().ScrollWidth
	hints.Width = hints.Height
	if this.IsVertical() {
		hints.Policy = ExpandHorizontal
	} else {
		hints.Policy = ExpandVertical
	}
	return
}

func (this *ScrollBar) IsValid() bool {
	return this.max > this.min && this.large > 0 && this.small > 0
}

func (this *ScrollBar) emitChanged() {
	if this.changed != nil {
		this.changed(this.Self())
	}
}

func (this *ScrollBar) SetChangedCallback(fn func(IWidget)) {
	this.changed = fn
}
