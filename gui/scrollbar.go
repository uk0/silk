package gui

import (
	"silk/core"
	//	"silk/factory"
	"silk/paint"
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
	ss := Theme().ScrollWidth
	if this.IsVertical() {
		h1 := this.h - ss*2
		if !this.IsValid() {
			x, y, w, h = 0, ss, this.w, h1
			return
		}
		ts := h1 * this.large / (this.large + this.max - this.min)
		if ts < ss {
			ts = ss
		}
		y = ss + (h1-ts)*this.value/(this.max-this.min)
		x = 0
		w = this.w
		h = ts
	} else {
		w1 := this.w - ss*2
		if !this.IsValid() {
			x, y, w, h = ss, 0, w1, this.h
			return
		}
		ts := w1 * this.large / (this.large + this.max - this.min)
		if ts < ss {
			ts = ss
		}
		x = ss + (w1-ts)*this.value/(this.max-this.min)
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
	ss := Theme().ScrollWidth
	var v float64
	if this.IsVertical() {
		h1 := this.h - ss*2
		v = this.min + (this.large+this.max-this.min)*(y-ss)/h1

	} else {
		w1 := this.w - ss*2
		v = this.min + (this.large+this.max-this.min)*(x-ss)/w1
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
	ss := Theme().ScrollWidth
	tx, ty, tw, th := this.TrackRect()
	if this.IsVertical() {
		if y >= ty && y < ty+th {
			return 3
		}
		if y < ss {
			return 1
		}
		if y >= this.h-ss {
			return 5
		}
		if y < ty {
			return 2
		}
		return 4
	} else {
		if x >= tx && x < tx+tw {
			return 3
		}
		if x < ss {
			return 1
		}
		if x >= this.w-ss {
			return 5
		}
		if x < tx {
			return 2
		}
		return 4

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
