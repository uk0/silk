package gui

import (
	"math"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.ComboBox", core.TypeOf((*ComboBox)(nil)))
}

type comboPopup struct {
	ListWidget
	combo *ComboBox
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
	if this.combo != nil && (!this.combo.IsEnabled() || !this.combo.IsVisible()) {
		return
	}
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

// 组合下拉框
type ComboBox struct {
	Widget
	sub             *comboPopup
	edit            IEdit
	btnSize         float64
	pushed          bool
	labelCache      comboLabelCache
	maxVisibleItems int

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

	edit := newComboEdit(this)
	this.SetEditWidget(edit)

	list := new(comboPopup)
	list.Init(list)
	list.combo = this
	list.SetParent(iw)
	list.SetVisible(false)
	core.Connect(list.SigSubmit, this.onListSubmit)
	list.SetSelectionVisible(true)
	list.SetHoverVisible(false)
	list.SetIconVisible(true)
	this.sub = list

}

func (this *ComboBox) SetEditWidget(edit IEdit) {
	if this.edit != nil && this.edit != edit && this.edit.Parent() == this.Self() {
		// Replacing the editor must detach the old child, otherwise both
		// editors keep rendering/hit-testing over the same field.
		this.edit.SetParent(nil)
	}
	this.edit = edit
	if edit != nil {
		edit.SetParent(this.Self())
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
	w = math.Max(0, w-22)
	h = math.Max(0, h)
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
	theme := Theme()
	g.Rectangle(0, 0, this.w, this.h)
	g.SetBrush1(theme.ViewBGColor)
	g.Fill()

	theme.DrawEditFrame(g, 0, 0, this.w, this.h, this.HasFocus(), this.IsHover(), false)
	if this.edit == nil {
		this.drawSelectionLabel(g)
	}
	// ClientRect reserves 22px for the dropdown affordance. Draw in that
	// region rather than relying on an external bitmap that may be absent or
	// unreadable in dark mode; the editable child never covers this path.
	right := this.w - theme.EditPadding.R
	left := right - 22
	if left < theme.EditPadding.L {
		left = theme.EditPadding.L
	}
	if right-left < 10 || this.h < 8 {
		return
	}
	x, y := (left+right)*0.5, this.h*0.5
	ink := theme.TextColor
	if !this.IsEnabled() {
		ink = paint.CurrentIconPalette().Disabled
	}
	g.MoveTo(x-4, y-2)
	g.LineTo(x, y+2)
	g.LineTo(x+4, y-2)
	g.SetPen1(ink, 1.5)
	g.Stroke()
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
	if this.sub == nil || this.IsSubPopupVisible() || !this.IsEnabled() || !this.IsVisible() || this.Count() == 0 {
		return
	}

	this.preparePopup()
	this.sub.SetParent(this.Self())
	x, y := this.MapToGlobal(0, 0)
	w, h := this.Size()
	// Bound the actual list before attaching/resizing a native surface. Never
	// allocate a backing buffer at the unbounded natural height of all items.
	area := comboPopupWorkArea(popupWorkAreaAt(x, y))
	this.sub.fitPopupWorkArea(area, geom.Rect{X: x, Y: y, Width: w, Height: h})
	this.sub.AttachWindow(WtPopup)
	LayoutPopup(this.sub, x, y, w, h, true, 1)
	this.sub.scrollRowIntoView(this.sub.activeIndex)

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
	if this.activeIndex >= idx {
		this.SetActiveIndex(this.activeIndex + 1)
	}
}

func (this *ComboBox) Remove(idx int) ListItem {
	item := this.sub.Remove(idx)
	if idx == this.activeIndex {
		this.SetActiveIndex(-1)
	} else if idx < this.activeIndex {
		this.SetActiveIndex(this.activeIndex - 1)
	}
	return item
}

func (this *ComboBox) RemoveLast() ListItem {
	if this.Count() == 0 {
		return ListItem{}
	}
	return this.Remove(this.Count() - 1)
}

func (this *ComboBox) Clear() {
	this.sub.Clear()
	this.SetActiveIndex(-1)
}

func (this *ComboBox) ItemList() (ret []ListItem) {
	return this.sub.ItemList()
}

func (this *ComboBox) Item(idx int) ListItem {
	return this.sub.Item(idx)
}

func (this *ComboBox) SetItem(idx int, item ListItem) {
	this.sub.SetItem(idx, item)
	if idx == this.activeIndex {
		this.SetActiveIndex(idx)
	}
}

func (this *ComboBox) Count() int {
	return this.sub.Count()
}

func (this *ComboBox) ActiveIndex() int {
	return this.activeIndex
}

func (this *ComboBox) ActiveItem() ListItem {
	if this.activeIndex < 0 || this.activeIndex >= this.Count() {
		return ListItem{}
	}
	return this.Item(this.activeIndex)
}

func (this *ComboBox) onListSubmit() {
	idx := this.sub.ActiveIndex()
	if !this.IsEnabled() || !this.IsVisible() || idx < 0 || idx >= this.Count() {
		return
	}
	// Retire this popup before callbacks; a callback may open a new popup/dialog.
	this.HideSubPopup()
	this.SetActiveIndex(idx)
	if this.edit != nil && this.edit.HasFocus() {
		this.edit.SelectAll()
	}
	if this.sigSubmit != nil {
		this.sigSubmit(this.Self())
	}
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
	this.SetActiveIndex(idx)
}

// SetActiveIndex commits a selection and updates the editor. -1 (or an
// out-of-range index) clears it. SelectionChanged fires once on an index
// change; programmatic selection never emits Submit.
func (this *ComboBox) SetActiveIndex(idx int) {
	if idx < -1 || idx >= this.Count() {
		idx = -1
	}
	this.sub.activeIndex = idx
	changed := idx != this.activeIndex
	this.activeIndex = idx
	if this.edit != nil {
		this.edit.SetText(this.ActiveItem().Text)
	}
	if changed && this.sigSelectionChanged != nil {
		this.sigSelectionChanged(this.Self(), this.activeIndex)
	}
	this.sub.Update()
	this.Self().Update()
}

// Items returns a copy of the display strings. For icons or application data,
// use ItemList/Append with ListItem instead.
func (this *ComboBox) Items() []string { return this.itemTexts() }

// SetItems replaces plain-text items and clears the current selection.
func (this *ComboBox) SetItems(items []string) {
	this.sub.Clear()
	for _, text := range items {
		this.sub.Append(ListItem{Text: text})
	}
	this.SetActiveIndex(-1)
}

func (this *ComboBox) Text() string {
	if this.edit != nil {
		return this.edit.Text()
	}
	return this.ActiveItem().Text
}

func (this *ComboBox) SetText(text string) {
	if this.edit != nil {
		this.edit.SetText(text)
	} else {
		for i := 0; i < this.Count(); i++ {
			if this.Item(i).Text == text {
				this.SetActiveIndex(i)
				return
			}
		}
	}
}

// OnKeyDown owns navigation, not native text. Selection-only controls and
// popups search via OnTextInput, preserving Unicode and avoiding double jumps
// from a key-down followed by its character event.
func (this *ComboBox) OnKeyDown(key int, repeat bool) {
	if this.handleComboKey(key, repeat) {
		return
	}
	// Preserve the legacy direct-key path when the editable combo frame itself
	// (rather than its editor) owns focus. Its text-input path is deliberately inert.
	if this.IsEnabled() && this.IsEditable() && currentMods() == 0 && (key >= 'A' && key <= 'Z' || key >= '0' && key <= '9') {
		this.searchPrefix(string(rune(key)))
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
	list.AddProperty("editable", this.IsEditable, this.SetEditable)
	list.AddProperty("max_visible_items", this.MaxVisibleItems, this.SetMaxVisibleItems)
	list.AddProperty("items", this.Items, this.SetItems)
	list.AddProperty("selectedIndex", this.ActiveIndex, this.SetActiveIndex)
	list.AddProperty("text", this.Text, this.SetText)
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
