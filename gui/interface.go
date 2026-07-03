package gui

import (
	"github.com/uk0/silk/paint"
)

//type IIconText interface {
//	Text() string
//	SetText(text string)
//	Icon() paint.Icon
//	SetIcon(icon paint.Icon)
//}

type IEventMouseMove interface {
	OnMouseMove(x, y float64)
}

type IEventMouseEnter interface {
	OnMouseEnter()
}

type IEventMouseLeave interface {
	OnMouseLeave()
}

type IEventLeftDown interface {
	OnLeftDown(x, y float64)
}

type IEventLeftUp interface {
	OnLeftUp(x, y float64)
}

type IEventRightDown interface {
	OnRightDown(x, y float64)
}

type IEventRightUp interface {
	OnRightUp(x, y float64)
}

type IEventMouseStop interface {
	OnMouseStop(x, y float64)
}

type IEventMouseWheel interface {
	OnMouseWheel(x, y, z float64)
}

//type IOnPrepare interface {
//	OnPrepare()
//}

type IEventFocusChanged interface {
	OnFocusChanged(newFocusWidget, oldFocusWidget IWidget)
}

type IEventTextInput interface {
	OnTextInput(s string)
}

type IWidgetEvent interface {
	OnMove()
	OnResize()
}

type IEventShow interface {
	OnShow()
}

type IEventHide interface {
	OnHide()
}

type IEventKeyDown interface {
	OnKeyDown(key int, repeat bool)
}

type IEventKeyUp interface {
	OnKeyUp(key int)
}

//type IHideSub interface {
//	HideSub()
//}

type ITitle interface {
	// 用来显示的标题
	Title() string
}

type IString interface {
	// 表示对象的内容的字符串
	String() string
}

type IText interface {
	// 表示对象的内容的字符串
	Text() string
}

type IIcon interface {
	// 表示对象的内容的字符串
	Icon() paint.Icon
}

type IDrawOverlay interface {
	DrawOverlay(cc paint.Painter)
}

//type ILayout interface {
//	SizeHints(owner IWidget) SizeHints
//	Arrange(owner IWidget)
//}

type IHeightForWidth interface {
	HeightForWidth(float64) float64
}

type IWidthForHeight interface {
	WidthForHeight(float64) float64
}

type ILayout interface {
	Layout()
}

type iWidget interface {
	setVisible(b bool)
	setSize(width, height float64)
	setPos(x, y float64)
}
