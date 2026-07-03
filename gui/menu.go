package gui

import (
	//	"github.com/uk0/silk/diag"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
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
	// 键盘导航高亮项的下标, -1 表示没有键盘高亮.
	// 鼠标悬停仍走全局 mouseHoverWidget; 键盘高亮通过把高亮项设为
	// 悬停项来复用既有的菜单项高亮绘制(见 setHighlight).
	highlight int
}

// 新建菜单
func NewMenu(popup bool) *Menu {
	m := new(Menu)
	m.Init(m)
	m.highlight = -1
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
	// 让弹出菜单成为键盘焦点, 这样方向键/回车等按键会被投递到本菜单的
	// OnKeyDown. WtPopup 窗口不抢操作系统焦点(FocusOnShow=false), 但
	// 按键派发以全局 focusWidget 为目标(见 window_glfw.go onKey), 因此
	// 这一步足以让键盘导航生效, 无需改动窗口/焦点层.
	this.highlight = -1
	this.SetFocus()
}

// menuItemKind 描述一个菜单项在键盘导航中的可选性, 用于把"下一个可选项"
// 的查找逻辑抽成纯函数以便单元测试. selectable 为 false 表示分隔线或被
// 禁用的项, 方向键会跳过它们.
type menuItemKind struct {
	selectable bool
}

// nextSelectableIndex 返回从 from 出发、沿 dir(+1 向下 / -1 向上)方向的下
// 一个可选项下标. wrap 为 true 时到达边界后回绕(Qt QMenu 的行为). 当没有
// 任何可选项时返回 -1. from 可以是 -1 表示"尚未高亮", 此时向下取第一个、
// 向上取最后一个可选项.
func nextSelectableIndex(items []menuItemKind, from, dir int, wrap bool) int {
	n := len(items)
	if n == 0 || dir == 0 {
		return -1
	}
	// from 越界视为"尚未高亮"的哨兵, 此时按方向把起点放到对应的边界外侧,
	// 使第一步恰好进入环的前缘: 向下(dir>0)从 -1 起 -> 第 0 项, 向上(dir<0)
	// 从 n 起 -> 第 n-1 项. 这与 Home/End 传入 -1 / n 的用法一致.
	if from < 0 || from >= n {
		if dir > 0 {
			from = -1
		} else {
			from = n
		}
	}
	i := from
	for k := 0; k < n; k++ {
		i += dir
		if i < 0 || i >= n {
			if !wrap {
				return -1 // 夹紧: 该方向上没有更多可选项
			}
			i = (i%n + n) % n
		}
		if items[i].selectable {
			return i
		}
	}
	return -1
}

// itemKinds 把当前菜单项映射为可选性切片, 供 nextSelectableIndex 使用.
// 仅 IButton 且 IsEnabled() 的项可选, 分隔线与禁用项不可选.
func (this *Menu) itemKinds() []menuItemKind {
	kinds := make([]menuItemKind, len(this.items))
	for i, c := range this.items {
		if _, ok := c.(IButton); ok {
			kinds[i].selectable = c.IsEnabled()
		}
	}
	return kinds
}

// highlightedButton 返回当前键盘高亮的菜单项按钮, 没有则返回 nil.
func (this *Menu) highlightedButton() *Button {
	if this.highlight < 0 || this.highlight >= len(this.items) {
		return nil
	}
	b, _ := this.items[this.highlight].(*Button)
	return b
}

// setHighlight 设置键盘高亮项下标, 并复用菜单项既有的悬停高亮绘制:
// 把高亮项设为全局悬停项即可让 DrawButton 渲染出与鼠标悬停一致的外观
// (高亮背景 + 反白文字), 无需改动 theme. idx 为 -1 时清除高亮.
func (this *Menu) setHighlight(idx int) {
	this.highlight = idx
	if b := this.highlightedButton(); b != nil {
		setMouseHoverWidget(b.Self())
	}
	this.Update()
}

// OnKeyDown 实现弹出菜单的键盘导航(对标 Qt QMenu):
//   - 上/下: 在可选项之间移动高亮, 跳过分隔线与禁用项, 越界回绕;
//   - Home/End: 跳到第一个/最后一个可选项;
//   - 回车/空格: 触发高亮项, 走与鼠标点击相同的 emit 路径;
//   - Esc: 关闭整条弹出菜单链;
//   - 右: 若高亮项有子菜单则打开并高亮其首个可选项;
//   - 左: 关闭当前子菜单, 返回父菜单.
//
// 仅弹出菜单处理按键; 菜单栏(非 popup)不参与键盘导航.
func (this *Menu) OnKeyDown(key int, repeat bool) {
	if !this.IsPopup() {
		return
	}
	kinds := this.itemKinds()
	switch key {
	case KeyDown:
		if idx := nextSelectableIndex(kinds, this.highlight, +1, true); idx >= 0 {
			this.setHighlight(idx)
		}
	case KeyUp:
		if idx := nextSelectableIndex(kinds, this.highlight, -1, true); idx >= 0 {
			this.setHighlight(idx)
		}
	case KeyHome:
		if idx := nextSelectableIndex(kinds, -1, +1, false); idx >= 0 {
			this.setHighlight(idx)
		}
	case KeyEnd:
		if idx := nextSelectableIndex(kinds, len(kinds), -1, false); idx >= 0 {
			this.setHighlight(idx)
		}
	case KeyEnter, KeySpace:
		if b := this.highlightedButton(); b != nil && b.IsEnabled() {
			// 与点击一致: 切换/关闭弹出链并触发 Action.
			b.emit()
		}
	case KeyEsc:
		if root := findRootPopup(this.Self()); root != nil {
			root.Hide()
		} else {
			this.Hide()
		}
	case KeyRight:
		if b := this.highlightedButton(); b != nil && b.IsEnabled() && b.SubPopup() != nil {
			b.ShowSubPopup()
			if sub, ok := b.SubPopup().(*Menu); ok {
				sub.SetFocus()
				if idx := nextSelectableIndex(sub.itemKinds(), -1, +1, false); idx >= 0 {
					sub.setHighlight(idx)
				}
			}
		}
	case KeyLeft:
		// 关闭当前子菜单, 把焦点交还父菜单(若存在).
		if parent, ok := this.Parent().(*Button); ok {
			if pm, ok := parent.Parent().(*Menu); ok {
				parent.HideSubPopup()
				pm.SetFocus()
				return
			}
		}
	}
}
