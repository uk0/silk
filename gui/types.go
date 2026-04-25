package gui

import (
	//	"silk/geom"
	"silk/geom"
	"silk/paint"
)

type titleProp struct {
	title string
}

func (this *titleProp) Title() string {
	return this.title
}

func (this *titleProp) SetTitle(s string) {
	this.title = s
}

type iconProp struct {
	icon paint.Icon
}

func (this *iconProp) Icon() paint.Icon {
	return this.icon
}

func (this *iconProp) SetIcon(icon paint.Icon) {
	this.icon = icon
}

type IconText struct {
	Ico paint.Icon
	Txt string
}

func (v IconText) Icon() paint.Icon {
	return v.Ico
}

func (v IconText) String() string {
	return v.Txt
}

func (v IconText) Text() string {
	return v.Txt
}

func LoadIcon(name string) paint.Icon {
	return paint.LoadIcon(name)
}

type HorzAlign int

const (
	HA_LEFT HorzAlign = iota
	HA_CENTER
	HA_RIGHT
)

type VertAlign int

const (
	VA_TOP VertAlign = iota
	VA_CENTER
	VA_BOTTOM
)

type SizePolicy int

const (
	ExpandHorizontal SizePolicy = 0x0001
	ExpandVertical   SizePolicy = 0x0002
	GrowHorizontal   SizePolicy = 0x0004
	GrowVertical     SizePolicy = 0x0008
	ShrinkHorizontal SizePolicy = 0x0010
	ShrinkVertical   SizePolicy = 0x0020

//	HeightForWidth              = 0x0040
//	WidthForHeight              = 0x0080
)

type SizeHints struct {
	Width, Height       float64
	MinWidth, MinHeight float64     // 最小尺寸约束
	MaxWidth, MaxHeight float64     // 最大尺寸约束 (0=无限制)
	Stretch             int         // 拉伸权重 (0=不拉伸/使用固定尺寸, >0=按权重分配剩余空间)
	Policy              SizePolicy
}

type Orientation int

const (
	East Orientation = iota
	West
	North
	South
)

func (o Orientation) IsVertical() bool {
	return o == North || o == South
}

func (o Orientation) IsHorizontal() bool {
	return o == West || o == East
}

type Conner int

const (
	TopLeft Conner = iota
	TopRight
	BottomLeft
	BottomRight
)

//type MouseButton int

//const (
//	LeftButton MouseButton = iota
//	RightButton
//	MiddleButton
//)

type AnchorFlag int

const (
	AnchorLef    AnchorFlag = 1
	AnchorRight  AnchorFlag = 2
	AnchorTop    AnchorFlag = 4
	AnchorBottom AnchorFlag = 8
)

type Anchor struct {
	Flags        AnchorFlag
	LeftOffset   float64
	RightOffset  float64
	TopOffset    float64
	BottomOffset float64
}

//type Place struct {
//	Mayjor, Minnor int
//}

type Vertical interface {
	IsVertical() bool
	SetVertical(b bool)
}

type VerticalT bool

func (v VerticalT) IsVertical() bool {
	return bool(v)
}

func (v *VerticalT) SetVertical(b bool) {
	*v = VerticalT(b)
}

type Margin struct {
	L, R, T, B float64
}

func (m Margin) Apply(x, y, w, h float64) (x1, y1, w1, h1 float64) {
	l, r, t, b := m.L, m.R, m.T, m.B
	x1 = x + l
	y1 = y + t
	w1 = w - l - r
	h1 = h - t - b
	if w1 < 0 {
		x1 += w1
		w1 = -w1
	}
	if h1 < 0 {
		y1 += h1
		h1 = -h1
	}
	return
}

// 边距
type Padding struct {
	L, R, T, B float64
}

func (m Padding) Apply(x, y, w, h float64) (x1, y1, w1, h1 float64) {
	l, r, t, b := m.L, m.R, m.T, m.B
	x1 = x + l
	y1 = y + t
	w1 = w - l - r
	h1 = h - t - b
	if w1 < 0 {
		x1 += w1
		w1 = -w1
	}
	if h1 < 0 {
		y1 += h1
		h1 = -h1
	}
	return
}

func (m Padding) Apply1(rc geom.Rect) geom.Rect {
	x, y, w, h := m.Apply(rc.X, rc.Y, rc.Width, rc.Height)
	return geom.Rect{x, y, w, h}
}
