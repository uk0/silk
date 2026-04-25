package gui

import (
	"silk/core"
	"silk/geom"
	"silk/paint"
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

	// Draw shadow behind the popup
	roundedRect(g, 2, 2, w, h, radius)
	g.SetBrush1(paint.Color{0, 0, 0, 30})
	g.Fill()

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
