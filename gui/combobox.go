package gui

import (
	"silk/core"
	"silk/geom"
	"silk/paint"
	"strings"
	//"math"
	//"time"
)

func init() {
	core.RegisterFactory("gui.ComboBox", core.TypeOf((*ComboBox)(nil)))
}

type comboPopup struct {
	ListWidget
}

// Draw renders the combo popup with shadow and rounded border overlay.
func (this *comboPopup) Draw(g paint.Painter) {
	w, h := this.Size()
	radius := 4.0

	paint.DrawShadowRect(g, 0, 0, w, h, radius, 4, paint.Color{0, 0, 0, 90})

	// Draw the normal list content
	this.ListWidget.Draw(g)

	// Overlay a rounded border on top (replacing the rectangular frame)
	t := Theme()
	roundedRect(g, 0, 0, w, h, radius)
	g.SetPen1(t.MenuBorderColor, 1)
	g.Stroke()
}

func (this *comboPopup) OnLeftDown(x, y float64) {
	// 如果在列表框内部, 或者不是弹出模式, 则不需做特殊处理
	if x >= 0 && y >= 0 && x < this.w && y < this.h || !this.IsPopup() {
		this.ListWidget.OnLeftDown(x, y)
		return
	}

	this.PopCapture()
	cb := this.Parent()
	if cb != nil {
		xg, yg := this.MapToGlobal(x, y)
		x1, y1 := cb.MapFromGlobal(xg, yg)
		if x1 >= 0 && x1 < cb.Width() && y1 >= 0 && y1 < cb.Height() {
			// 在ComboBox范围内, 还要先排除子控件
			edit := cb.FindWidgetAt(x1, y1)
			if edit == nil || edit == cb {
				// 点击的是组合框的空白和按钮处, 只需模拟鼠标消息, 让ComboBox负责隐藏列表
				// 这里不能调用this.Hide(), 否则Combox会再把列表显示出来
				emulateMouseDown(true)
				return
			}
		}
	}
	// 点击其他位置均需隐藏下拉框
	this.Hide()
	emulateMouseDown(true)
}

//func (this *comboPopup) OnLeftDown(x, y float64) {
//	this.ListWidget.OnLeftUp(x, y)

//	this.PopCapture()
//	cb := this.Parent()
//	if cb != nil {
//		xg, yg := this.MapToGlobal(x, y)
//		x1, y1 := cb.MapFromGlobal(xg, yg)
//		if x1 >= 0 && x1 < cb.Width() && y1 >= 0 && y1 < cb.Height() {
//			// 在ComboBox范围内, 还要先排除子控件
//			edit := cb.FindWidgetAt(x1, y1)
//			if edit == nil || edit == cb {
//				// 点击的是组合框的空白和按钮处, 只需模拟鼠标消息, 让ComboBox负责隐藏列表
//				// 这里不能调用this.Hide(), 否则Combox会再把列表显示出来
//				emulateMouseDown(true)
//				return
//			}
//		}
//	}
//	// 点击其他位置均需隐藏下拉框
//	this.Hide()
//	emulateMouseDown(true)
//}

// 组合下拉框
type ComboBox struct {
	Widget
	sub     *comboPopup
	edit    IEdit
	btnSize float64
	pushed  bool

	activeIndex         int
	sigSelectionChanged func(o interface{}, idx int)
	sigSubmit           func(o interface{})
}

func NewComboBox() *ComboBox {
	p := new(ComboBox)
	p.Init(p)
	return p
}

func (this *ComboBox) Init(iw IWidget) {
	this.Widget.Init(iw)

	this.activeIndex = -1

	edit := NewEdit()
	edit.SetPadding(Padding{})
	edit.SetNoFrame(true)
	edit.SetText("中文测试")
	this.SetEditWidget(edit)

	list := new(comboPopup)
	list.Init(list)
	list.SetParent(iw)
	list.SetVisible(false)
	core.Connect(list.SigSubmit, this.onListSubmit)
	list.SetSelectionVisible(false)
	list.SetHoverVisible(true)
	list.SetIconVisible(true)
	this.sub = list

}

func (this *ComboBox) SetEditWidget(edit IEdit) {
	this.edit = edit
	if edit != nil {
		edit.SetParent(this)
		edit.SetRedrawParent(true)
	}
	this.Layout()
}

func (this *ComboBox) EditWidget() IEdit {
	return this.edit
}

func (this *ComboBox) SubPopup() *comboPopup {
	return this.sub
}

func (this *ComboBox) IsEditable() bool {
	return this.edit != nil
}

func (this *ComboBox) Layout() {

	clientRect := this.ClientRect()
	if this.edit != nil {
		this.edit.SetBounds1(clientRect)
	}
}

func (this *ComboBox) ClientRect() geom.Rect {
	x, y, w, h := 0.0, 0.0, this.w, this.h
	m := Theme().EditPadding
	x, y, w, h = m.Apply(x, y, w, h)
	w -= 22
	return geom.Rect{x, y, w, h}
}

func (this *ComboBox) HasFocus() bool {
	return this.Widget.HasFocus() ||
		this.edit != nil && this.edit.HasFocus()
}

func (this *ComboBox) IsHover() bool {
	return this.Widget.IsHover() ||
		this.edit != nil && this.edit.IsHover()
}

func (this *ComboBox) Draw(g paint.Painter) {
	g.Rectangle(0, 0, this.w, this.h)
	g.SetBrush1(Theme().ViewBGColor)
	g.Fill()

	Theme().DrawEditFrame(g, 0, 0, this.w, this.h, this.HasFocus(), this.IsHover(), false)
}

func (this *ComboBox) OnMouseEnter() {
	this.Self().Update()
	core.Debug("(this *ComboBox) OnMouseEnter()")
}

func (this *ComboBox) OnMouseLeave() {
	this.Self().Update()
	core.Debug("(this *ComboBox) OnMouseLeave()")
}

func (this *ComboBox) OnLeftDown(x, y float64) {
	//core.Debug("(this *Button) OnLeftDown()")
	if this.IsEnabled() {
		this.pushed = true
		this.SetFocus()
		this.Self().Update()
	}
}

func (this *ComboBox) OnLeftUp(x, y float64) {
	//core.Debug("(this *Button) OnLeftUp()")
	pushed := this.pushed
	this.pushed = false
	this.Self().Update()
	this.PopCapture()
	if pushed && this.IsHover() && this.IsEnabled() {
		this.ToggleSubPopup()
	}
}

func (this *ComboBox) IsSubPopupVisible() bool {
	return this.sub != nil && this.sub.IsVisible()
}

func (this *ComboBox) ShowSubPopup() {
	if this.sub == nil || this.IsSubPopupVisible() {
		return
	}

	this.sub.SetParent(this)
	this.sub.AttachWindow(WtPopup)
	hints := this.sub.SizeHints()
	this.sub.SetSize(hints.Width, hints.Height)
	x, y := this.MapToGlobal(0, 0)
	w, h := this.Size()
	this.sub.SetWidth(w)
	LayoutPopup(this.sub, x, y, w, h, true, 1)

	this.sub.Show()
	this.sub.PushCapture()
	this.Self().Update()

}

func (this *ComboBox) HideSubPopup() {
	if this.sub == nil || !this.sub.IsVisible() {
		return
	}

	this.sub.Hide()

	if this.edit != nil {
		this.edit.SelectAll()
		this.edit.SetFocus()
	} else {
		this.SetFocus()
	}
}

func (this *ComboBox) ToggleSubPopup() {
	if this.IsSubPopupVisible() {
		this.HideSubPopup()
	} else {
		this.ShowSubPopup()
	}
}

func (this *ComboBox) Append(a ListItem) {
	this.sub.Append(a)
}

func (this *ComboBox) Insert(idx int, a ListItem) {
	this.sub.Insert(idx, a)
}

func (this *ComboBox) Remove(idx int) ListItem {
	return this.sub.Remove(idx)
}

func (this *ComboBox) RemoveLast() ListItem {
	return this.sub.RemoveLast()
}

func (this *ComboBox) Clear() {
	this.sub.Clear()
}

func (this *ComboBox) ItemList() (ret []ListItem) {
	return this.sub.ItemList()
}

func (this *ComboBox) Item(idx int) ListItem {
	return this.sub.Item(idx)
}

func (this *ComboBox) SetItem(idx int, item ListItem) {
	this.sub.SetItem(idx, item)
}

func (this *ComboBox) Count() int {
	return this.sub.Count()
}

func (this *ComboBox) ActiveIndex() int {
	return this.activeIndex
}

func (this *ComboBox) ActiveItem() ListItem {
	return this.sub.ActiveItem()
}

func (this *ComboBox) onListSubmit() {
	if this.edit != nil {
		this.edit.SetText(this.ActiveItem().Text)
	}
	if this.sub.ActiveIndex() != this.activeIndex {
		this.activeIndex = this.sub.ActiveIndex()
		if this.sigSelectionChanged != nil {
			this.sigSelectionChanged(this.Self(), this.activeIndex)
		}
	}
	if this.sigSubmit != nil {
		this.sigSubmit(this.Self())
	}

	this.HideSubPopup()
}

// setActiveIndex commits idx as the current selection (the "closed dropdown"
// path). It clamps to a valid row, syncs the list's highlight, updates the
// edit text and fires sigSelectionChanged only when the index actually moves.
func (this *ComboBox) setActiveIndex(idx int) {
	n := this.Count()
	if n == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	this.sub.activeIndex = idx
	if idx == this.activeIndex {
		return
	}
	this.activeIndex = idx
	if this.edit != nil {
		this.edit.SetText(this.sub.Item(idx).Text)
	}
	if this.sigSelectionChanged != nil {
		this.sigSelectionChanged(this.Self(), this.activeIndex)
	}
	this.Self().Update()
}

// OnKeyDown implements Qt QComboBox keyboard navigation. When the dropdown is
// open Up/Down move the highlighted row and Enter commits it; when closed they
// change the current selection directly. Esc closes without changing selection,
// Home/End jump to the first/last item, and a printable character performs a
// single-character type-ahead jump to the next matching item.
func (this *ComboBox) OnKeyDown(key int, repeat bool) {
	n := this.Count()
	open := this.IsSubPopupVisible()

	switch key {
	case KeyDown:
		if open {
			this.sub.activeIndex = clampIndex(this.sub.activeIndex+1, n)
			this.sub.Update()
		} else {
			this.setActiveIndex(this.activeIndex + 1)
		}
		return
	case KeyUp:
		if open {
			this.sub.activeIndex = clampIndex(this.sub.activeIndex-1, n)
			this.sub.Update()
		} else {
			this.setActiveIndex(this.activeIndex - 1)
		}
		return
	case KeyHome:
		if open {
			this.sub.activeIndex = clampIndex(0, n)
			this.sub.Update()
		} else {
			this.setActiveIndex(0)
		}
		return
	case KeyEnd:
		if open {
			this.sub.activeIndex = clampIndex(n-1, n)
			this.sub.Update()
		} else {
			this.setActiveIndex(n - 1)
		}
		return
	case KeyEnter:
		if open {
			// Commit the highlighted row through the normal submit path,
			// which updates the edit text, fires the callbacks and hides.
			this.onListSubmit()
		}
		return
	case KeyEsc:
		if open {
			this.HideSubPopup()
		}
		return
	}

	// Type-ahead: printable A-Z / 0-9 arrive as their ASCII code (see
	// keyboard_glfw.go). Jump to the next item whose text starts with it.
	if key >= 0x20 && key <= 0x7E {
		start := this.activeIndex
		if open {
			start = this.sub.activeIndex
		}
		idx := nextMatchIndex(this.itemTexts(), start, rune(key))
		if idx >= 0 {
			if open {
				this.sub.activeIndex = idx
				this.sub.Update()
			} else {
				this.setActiveIndex(idx)
			}
		}
	}
}

// itemTexts returns the display text of every item, for type-ahead matching.
func (this *ComboBox) itemTexts() []string {
	n := this.Count()
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = this.sub.Item(i).Text
	}
	return out
}

func (this *ComboBox) SigSelectionChanged(fn func(o interface{}, idx int)) {
	this.sigSelectionChanged = fn
}

func (this *ComboBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("可用", this.IsEnabled, this.SetEnabled)
	list.AddProperty("可见", this.IsVisible, this.SetVisible)
}

func (this *ComboBox) SigSubmit(fn func(o interface{})) {
	this.sigSubmit = fn
}

// clampIndex clamps i to [0, n-1]; returns -1 for an empty list.
func clampIndex(i, n int) int {
	if n <= 0 {
		return -1
	}
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// nextMatchIndex returns the index of the next item (after start, cycling) whose
// text begins with ch, matched case-insensitively. The search starts at start+1
// and wraps around through start itself, so a repeated keystroke steps through
// successive matches. Returns -1 when no item matches.
func nextMatchIndex(items []string, start int, ch rune) int {
	n := len(items)
	if n == 0 {
		return -1
	}
	prefix := strings.ToLower(string(ch))
	for off := 1; off <= n; off++ {
		i := (start + off) % n
		if i < 0 {
			i += n
		}
		if strings.HasPrefix(strings.ToLower(items[i]), prefix) {
			return i
		}
	}
	return -1
}
