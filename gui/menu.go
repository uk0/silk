package gui

import (
	//	"silk/diag"
	"silk/core"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.Menu", core.TypeOf((*Menu)(nil))) //((*Menu)(nil)))
}

type IMenu interface {
	HideAllSubs()
}

// 菜单, 用来装载菜单按钮
// 可用作弹出菜单和传统的菜单栏
// 注: Menu是菜单项的容器, 不是菜单项
type Menu struct {
	Widget
	items []IWidget
	VerticalT
}

// 新建菜单
func NewMenu(popup bool) *Menu {
	m := new(Menu)
	m.Init(m)
	if popup {
		m.SetVertical(true)
		m.SetVisible(false)
		m.LazyAttachWindow(WtPopup)
	}
	return m
}

// 新建传统菜单栏形式的菜单
func NewMenuBar() *Menu {
	return NewMenu(false)
}

// 新建弹出菜单
func NewPopupMenu() *Menu {
	return NewMenu(true)
}

func (this *Menu) Draw(g paint.Painter) {
	Theme().DrawMenu(g, this)
}

func shrinkRect(x, y, w, h, margin float64) (x1, y1, w1, h1 float64) {
	x1 = x + margin
	y1 = y + margin
	w1 = w - margin*2
	h1 = h - margin*2
	if w1 < 0 {
		w1 = 0
	}
	if h1 < 0 {
		h1 = 0
	}
	return
}

func sepCount(objCount int) int {
	if objCount < 1 {
		return 0
	}
	return objCount - 1
}

func (this *Menu) layoutVertical() {
	count := len(this.items)
	//if count == 0 {
	//	return
	//}
	iw := this.Self()
	t := Theme()
	//ml, mr, mt, mb := t.MenuMargin.Margin()
	spacing := float64(0)
	w, h := iw.Size()
	var margin *Margin
	if this.IsPopup() {
		margin = &t.MenuMargin
	} else {
		margin = &t.MenuBarMargin
	}
	x, y, w, h := margin.Apply(0, 0, w, h)
	n := count
	hints := make([]SizeHints, n, n)
	var total float64
	var expandBase, shrinkBase float64

	for i, c := range this.items {

		hi := c.SizeHints()
		hints[i] = hi
		total += hi.Height

		if (hi.Policy & ExpandVertical) != 0 {
			expandBase += hi.Height
		}
		if (hi.Policy & ShrinkVertical) != 0 {
			shrinkBase += hi.Height
		}

		if (hi.Policy & (ExpandHorizontal | GrowHorizontal)) != 0 {
			hints[i].Width = w
		}
	}
	extra := w - total - float64(sepCount(n))*spacing
	if extra > 0 && expandBase > 0 {
		dh := extra / expandBase
		y1 := y
		for i, c := range this.items {
			hi := hints[i]
			var h1 float64
			if (hi.Policy & ExpandVertical) != 0 {
				h1 = hi.Height + hi.Height*dh
			} else {
				h1 = hi.Height
			}
			c.SetBounds(x, y1, hi.Width, h1)
			y1 += h1 + spacing
		}
	} else if extra < 0 && shrinkBase > 0 {
		dh := extra / shrinkBase
		y1 := y
		for i, c := range this.items {
			hi := hints[i]
			var h1 float64
			if (hi.Policy & ShrinkVertical) != 0 {
				h1 = hi.Height + hi.Height*dh
			} else {
				h1 = hi.Height
			}
			c.SetBounds(x, y1, hi.Width, h1)
			y1 += h1 + spacing
		}
	} else {
		y1 := y
		for i, c := range this.items {
			hi := hints[i]
			var h1 float64 = hi.Height
			c.SetBounds(x, y1, hi.Width, h1)
			y1 += h1 + spacing
		}

	}
}

func (this *Menu) layoutHorizontal() {
	count := len(this.items)
	//if count == 0 {
	//	return
	//}
	iw := this.Self()
	t := Theme()
	//	margin := float64(0)
	spacing := float64(0)
	w, h := iw.Size()
	var margin *Margin
	if this.IsPopup() {
		margin = &t.MenuMargin
	} else {
		margin = &t.MenuBarMargin
	}
	x, y, w, h := margin.Apply(0, 0, w, h)
	n := count
	hints := make([]SizeHints, n, n)
	var total float64
	var expandBase, shrinkBase float64

	for i, c := range this.items {
		hi := c.SizeHints()
		hints[i] = hi
		total += hi.Width

		if (hi.Policy & ExpandHorizontal) != 0 {
			expandBase += hi.Width
		}
		if (hi.Policy & ShrinkHorizontal) != 0 {
			shrinkBase += hi.Width
		}

		if (hi.Policy & (ExpandVertical | GrowVertical)) != 0 {
			hints[i].Height = h
		}
	}
	extra := w - total - float64(sepCount(n))*spacing
	if extra > 0 && expandBase > 0 {
		dw := extra / expandBase
		x1 := x
		for i, c := range this.items {
			hi := hints[i]
			var w1 float64
			if (hi.Policy & ExpandHorizontal) != 0 {
				w1 = hi.Width + hi.Width*dw
			} else {
				w1 = hi.Width
			}
			c.SetBounds(x1, y, w1, hi.Height)
			x1 += w1 + spacing
		}
	} else if extra < 0 && shrinkBase > 0 {
		dw := extra / shrinkBase
		x1 := x
		for i, c := range this.items {
			hi := hints[i]
			var w1 float64
			if (hi.Policy & ShrinkHorizontal) != 0 {
				w1 = hi.Width + hi.Width*dw
			} else {
				w1 = hi.Width
			}
			c.SetBounds(x1, y, w1, hi.Height)
			x1 += w1 + spacing
		}
	} else {
		x1 := x
		for i, c := range this.items {
			hi := hints[i]
			var w1 float64 = hi.Width
			c.SetBounds(x1, y, w1, hi.Height)
			x1 += w1 + spacing
		}
	}
}

func (this *Menu) Layout() {
	if this.IsVertical() {
		this.layoutVertical()
	} else {
		this.layoutHorizontal()
	}
}

func (this *Menu) SizeHints() SizeHints {
	t := Theme()

	if this.IsPopup() {
		//ml, mr, mt, mb := t.MenuMargin.Margin()
		m := t.MenuMargin
		var w, h float64
		for _, c := range this.items {
			hints := c.SizeHints()
			w = math.Max(w, hints.Width)
			h += hints.Height
		}
		w += m.L + m.R
		h += m.T + m.B
		return SizeHints{Width: w, Height: h, Policy: 0}
	} else {
		m := t.MenuBarMargin
		var w, h float64
		if this.IsVertical() {
			//h += float64(sep) * t.Spacing
			for _, c := range this.items {
				hints := c.SizeHints()
				w = math.Max(w, hints.Width)
				h += hints.Height
			}
		} else {
			//w += float64(sep) * t.Spacing
			for _, c := range this.items {
				hints := c.SizeHints()
				w += hints.Width
				h = math.Max(h, hints.Height)
			}
		}
		w += m.L + m.R
		h += m.T + m.B
		return SizeHints{Width: w, Height: h, Policy: 0}
	}

}

func (this *Menu) AddWidget(iw IWidget) {
	iw.SetParent(this)
	this.items = append(this.items, iw)
}

func (this *Menu) RemoveWidget(iw IWidget) {
	iw.SetParent(nil)
	for i, v := range this.items {
		if v == iw {
			for j := i; j < len(this.items)-1; j++ {
				this.items[j] = this.items[j+1]
			}
			this.items[len(this.items)-1] = nil
			this.items = this.items[:len(this.items)-1]
			break
		}
	}
	this.Layout()
}

func (this *Menu) Clear() {
	for _, iw := range this.Items() {
		iw.Detach()
	}
	this.items = nil
}

func (this *Menu) AddSubMenu(text string, icon paint.Icon, sub *Menu) (*Menu, *Button) {
	btn := this.AddButton1(text, icon)
	if sub == nil {
		sub = NewMenu(true)
	}
	btn.SetSubPopup(sub)
	btn.SetTextVisible(true)
	return sub, btn
}

func (this *Menu) AddButton() *Button {
	return this.AddButton1("", nil)
}

func (this *Menu) AddButton1(text string, icon paint.Icon) *Button {
	btn := NewButton1(text, icon)
	this.AddWidget(btn)
	return btn
}

func (this *Menu) AddActionButton(a IAction) *Button {
	btn := NewActionButton(a)
	this.AddWidget(btn)
	return btn
}

func (this *Menu) AddSeparator() *Separator {
	sep := NewSeparator()
	this.AddWidget(sep)
	return sep
}

func (this *Menu) Items() []IWidget {
	return this.items
}

func (this *Menu) OnShow() {

}

func (this *Menu) OnHide() {
	this.HideAllSubs()
	if this.parent != nil {
		this.parent.Self().Update()
	}
	if this.Window() != nil && this.Window().inModal {
		this.Window().EndModal(nil)
	}
}

func findAscendantsByPos(w IWidget, x, y float64, pass, match func(IWidget) bool) IWidget {
	if x >= 0 && y >= 0 && x < w.Width() && y < w.Height() {
		return w
	}
	xg, yg := w.MapToGlobal(x, y)
	for w = w.Parent(); w != nil && (pass == nil || pass(w)); w = w.Parent() {
		if match != nil && !match(w) {
			continue
		}
		x, y = w.MapFromGlobal(xg, yg)
		if x >= 0 && y >= 0 && x < w.Width() && y < w.Height() {
			return w
		}
	}
	return nil
}

func findRootPopup(w IWidget) IWidget {

	if p := w.Parent(); p != nil {
		ret := findRootPopup(p)
		if ret != nil {
			return ret
		}
	}

	if w.IsPopup() {
		return w
	}
	return nil
}

func (this *Menu) OnLeftDown(x, y float64) {
	if x >= 0 && y >= 0 && x < this.w && y < this.h {
		return
	}
	p := findAscendantsByPos(this.Self(), x, y,
		func(w IWidget) bool {
			if _, ok := w.(IButton); ok {
				return true
			}
			if _, ok := w.(IMenu); ok {
				return true
			}
			return false
		},
		func(w IWidget) bool {
			if _, ok := w.(IButton); ok {
				return true
			}
			if _, ok := w.(IMenu); ok {
				return true
			}
			return false
		})
	if btn, ok := p.(IButton); ok {
		if !btn.IsInPopupMenu() {
			btn.HideSubPopup()
		}
		return
	}
	if menu, ok := p.(IMenu); ok {
		menu.HideAllSubs()
		return
	}

	// Click is outside all menus — close the popup chain first,
	// then re-deliver the click. We MUST hide and release capture
	// BEFORE calling emulateMouseDown, otherwise the emulated event
	// gets delivered back to this same Menu.OnLeftDown → infinite recursion.
	root := findRootPopup(this.Self())
	if root != nil {
		root.Hide()
	}
	emulateMouseDown(true)
}

func (this *Menu) OnMouseStop(x, y float64) {
	if x >= 0 && y >= 0 && x < this.w && y < this.h {
		return
	}
	p := findAscendantsByPos(this.Self(), x, y,
		func(w IWidget) bool {
			if _, ok := w.(IButton); ok {
				return true
			}
			if _, ok := w.(IMenu); ok {
				return true
			}
			return false
		},
		func(w IWidget) bool {
			if _, ok := w.(IButton); ok {
				return true
			}
			if _, ok := w.(IMenu); ok {
				return true
			}
			return false
		})

	if menu, ok := p.(IMenu); ok {
		menu.HideAllSubs()
		w1 := FindWidgetUnderMouse()
		//core.Debug(w1)
		if btn, ok := w1.(IButton); ok {
			btn.ShowSubPopup()
		}
		return
	}

}

func (this *Menu) OnMouseMove(x, y float64) {

}

func (this *Menu) HideAllSubs() {
	for _, v := range this.Children() {
		if i, ok := v.(interface {
			HideSubPopup()
		}); ok {
			i.HideSubPopup()
		}
	}
}

func (this *Menu) OnIdle() {
	for _, v := range this.Children() {
		if v.IsVisible() {
			if im, ok := v.(interface {
				OnIdle()
			}); ok {
				im.OnIdle()
			}
		}
	}
}

// 在全局坐标(xg, yg)显示为弹出菜单
// 注: 无论菜单的Parent()是否为nil, 都是全局坐标
func (this *Menu) ShowAsPopup(xg, yg float64, autoClose bool) {
	this.AttachWindow(WtPopup)
	if autoClose {
		this.Window().SetCloseOnHide(true)
	}
	hints := this.SizeHints()
	this.SetSize(0, 0)
	this.SetSize(hints.Width, hints.Height)
	LayoutPopup1(this, xg, yg)
	this.SetVisible(true)
	this.PushCapture()
}
