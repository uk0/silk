package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
	"strings"
)

func init() {
	core.RegisterFactory("gui.ToolBar", core.TypeOf((*ToolBar)(nil)))
}

// ToolBar is a horizontal or vertical bar for tool buttons, separators,
// and arbitrary widgets. Similar to QToolBar.
type ToolBar struct {
	Widget
	items                []IWidget
	orientation          int     // 0=horizontal, 1=vertical
	iconSize             int     // icon display size in pixels
	spacing              float64 // gap between items
	compact              bool    // opt-in dense desktop tool chrome; ordinary forms unchanged
	preserveContentFocus bool
	overflow             []IWidget
	overflowBtn          *Button
}

func NewToolBar() *ToolBar {
	p := new(ToolBar)
	p.Init(p)
	return p
}

// Init carries the icon size and spacing, not NewToolBar: the designer and
// the .tdoc loader build widgets through the core factory, which reflects on
// Init and never sees the constructor. Icon buttons are sized from iconSize,
// so a factory toolbar shrinks them to the button margins alone.
func (this *ToolBar) Init(self IWidget) {
	this.Widget.Init(self)
	this.iconSize = int(Theme().IconSize)
	this.spacing = Theme().Spacing
}

func (this *ToolBar) EnumProperties(list core.IPropertyList) {
	list.AddProperty("图标大小", this.IconSize, this.SetIconSize)
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
	list.AddProperty("保留内容焦点", this.PreserveContentFocus, this.SetPreserveContentFocus)
}

// AddAction creates a tool button and appends it to the toolbar.
func (this *ToolBar) AddAction(text string, icon paint.Icon, callback func()) *Button {
	btn := NewButton1(text, icon)
	btn.Action().BindFunc0(callback)
	this.AddWidget(btn)
	return btn
}

// AddActionButton adds an existing action as a tool button.
func (this *ToolBar) AddActionButton(a IAction) *Button {
	btn := NewActionButton(a)
	this.AddWidget(btn)
	return btn
}

// AddSeparator inserts a visual separator into the toolbar.
func (this *ToolBar) AddSeparator() *Separator {
	sep := NewSeparator()
	this.AddWidget(sep)
	return sep
}

// AddWidget appends any widget (e.g. ComboBox, Edit) to the toolbar.
func (this *ToolBar) AddWidget(iw IWidget) {
	iw.SetParent(this)
	this.items = append(this.items, iw)
}

// RemoveWidget removes a widget from the toolbar.
func (this *ToolBar) RemoveWidget(iw IWidget) {
	iw.SetParent(nil)
	for i, v := range this.items {
		if v == iw {
			copy(this.items[i:], this.items[i+1:])
			this.items[len(this.items)-1] = nil
			this.items = this.items[:len(this.items)-1]
			break
		}
	}
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
}

// Items returns all items in the toolbar.
func (this *ToolBar) Items() []IWidget {
	return this.items
}

func (this *ToolBar) IconSize() int {
	return this.iconSize
}

func (this *ToolBar) SetIconSize(size int) {
	size = max(1, min(size, 256))
	if this.iconSize == size {
		return
	}
	this.iconSize = size
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *ToolBar) Spacing() float64 {
	return this.spacing
}

// SetCompact uses square icon targets and flat button faces, with font-aware
// label widths. Metrics are logical units; the window's DPI transform is applied
// once by the painter. It does not shrink arbitrary embedded widgets.
func (this *ToolBar) SetCompact(on bool) {
	if this.compact == on {
		return
	}
	this.compact = on
	this.Layout()
	this.Update()
}

func (this *ToolBar) IsCompact() bool { return this.compact }

// SetPreserveContentFocus keeps mouse-clicked tool buttons from taking focus
// away from the document or field they act on. Keyboard focus and embedded
// input widgets are unchanged; actions may still explicitly focus a new view.
// Opt-in so existing SDK toolbars retain their ordinary button behavior.
func (this *ToolBar) SetPreserveContentFocus(on bool) { this.preserveContentFocus = on }
func (this *ToolBar) PreserveContentFocus() bool      { return this.preserveContentFocus }

func (this *ToolBar) chromeMargin() Margin {
	if this.compact {
		return Margin{6, 6, 3, 3}
	}
	return Theme().ButtonMargin
}

func (this *ToolBar) compactButtonHeight() float64 {
	return math.Ceil(math.Max(28, math.Max(float64(this.iconSize), Theme().Font.FontExtents().Height)+8))
}

func (this *ToolBar) SetSpacing(s float64) {
	if math.IsNaN(s) || math.IsInf(s, 0) || s < 0 {
		s = 0
	}
	if this.spacing == s {
		return
	}
	this.spacing = s
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *ToolBar) Orientation() int {
	return this.orientation
}

func (this *ToolBar) SetOrientation(o int) {
	if this.orientation == o {
		return
	}
	this.orientation = o
	if i, ok := this.Self().(ILayout); ok {
		i.Layout()
	}
	this.Self().Update()
}

func (this *ToolBar) IsVertical() bool {
	return this.orientation == 1
}

// Layout arranges children horizontally or vertically.
func (this *ToolBar) Layout() {
	if this.IsVertical() {
		this.layoutVertical()
	} else {
		this.layoutHorizontal()
	}
}

// itemSizeHints is the single measurement used by placement and the bar's
// preferred size. A button's action text can be deliberately hidden; measuring
// it anyway both wastes horizontal space and under-reports text-only heights.
func (this *ToolBar) itemSizeHints(item IWidget) SizeHints {
	t := Theme()
	m := this.chromeMargin()
	icon := float64(this.iconSize)
	if this.compact {
		h := this.compactButtonHeight()
		if _, ok := item.(*Separator); ok {
			if this.IsVertical() {
				return SizeHints{Width: h, Height: 8}
			}
			return SizeHints{Width: 8, Height: h}
		}
		if btn, ok := item.(*Button); ok {
			if !btn.IsTextVisible() {
				return SizeHints{Width: h, Height: h}
			}
			ext := t.Font.TextExtents(btn.Text())
			w := math.Max(ext.Width, ext.XAdvance) + 16
			if btn.Icon() != nil && !btn.Icon().IsAir() {
				w += icon + 6
			}
			return SizeHints{Width: math.Ceil(math.Max(w, h)), Height: h}
		}
	}
	if _, ok := item.(*Separator); ok {
		if this.IsVertical() {
			return SizeHints{Width: icon + m.L + m.R + 4, Height: t.SeparatorSize}
		}
		return SizeHints{Width: t.SeparatorSize + 6, Height: icon + m.T + m.B + 4}
	}
	if btn, ok := item.(*Button); ok {
		hasIcon := btn.Icon() != nil && !btn.Icon().IsAir()
		if !btn.IsTextVisible() {
			return SizeHints{Width: icon + m.L + m.R + 4, Height: icon + m.T + m.B + 4}
		}
		text := t.Font.TextExtents(btn.Text())
		textW := math.Max(text.Width, text.XAdvance)
		textH := t.Font.FontExtents().Height
		if hasIcon {
			// Theme.DrawButton starts the label at 2*left margin + icon.
			return SizeHints{Width: icon + textW + m.L*2 + m.R + 4,
				Height: math.Max(icon, textH) + m.T + m.B + 4}
		}
		return SizeHints{Width: textW + m.L + m.R + 12, Height: textH + m.T + m.B + 6}
	}
	return item.SizeHints()
}

func toolbarDimension(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	return value
}

func (this *ToolBar) layoutHorizontal() {
	m := this.chromeMargin()
	w, h := this.Self().Size()
	w, h = toolbarDimension(w), toolbarDimension(h)
	type itemGeom struct{ y, w, h float64 }
	geom := make([]itemGeom, len(this.items))
	for i, item := range this.items {
		hints := this.itemSizeHints(item)
		iw, ih := toolbarDimension(hints.Width), toolbarDimension(hints.Height)
		if _, ok := item.(*Separator); ok {
			inset := math.Min(4, h/2)
			geom[i] = itemGeom{inset, iw, h - 2*inset}
			continue
		}
		if _, button := item.(*Button); !button && hints.Policy&(GrowVertical|ExpandVertical) != 0 {
			ih = h
		}
		ih = math.Min(ih, h)
		geom[i] = itemGeom{(h - ih) / 2, iw, ih}
	}

	ovW := float64(this.iconSize) + m.L + m.R + 4
	if this.compact {
		ovW = this.compactButtonHeight()
	}
	avail := math.Max(0, w-m.R)
	total := m.L
	for i, g := range geom {
		if i > 0 {
			total += this.spacing
		}
		total += g.w
	}
	shown := len(geom)
	if total > avail+1e-9 {
		limit := avail - ovW - this.spacing
		x := m.L
		shown = 0
		for i, g := range geom {
			if i > 0 {
				x += this.spacing
			}
			if x+g.w > limit+1e-9 {
				break
			}
			x += g.w
			shown = i + 1
		}
	}
	this.overflow = nil
	x := m.L
	for i, item := range this.items {
		if i >= shown {
			item.SetVisible(false)
			this.overflow = append(this.overflow, item)
			continue
		}
		item.SetVisible(true)
		item.SetBounds(x, geom[i].y, geom[i].w, geom[i].h)
		x += geom[i].w + this.spacing
	}
	if len(this.overflow) == 0 {
		if this.overflowBtn != nil {
			this.overflowBtn.SetVisible(false)
		}
		return
	}
	btn := this.overflowButton()
	btnW := math.Min(ovW, w)
	btnH := math.Min(float64(this.iconSize)+m.T+m.B+4, h)
	if this.compact {
		btnH = math.Min(this.compactButtonHeight(), h)
	}
	btn.SetVisible(true)
	btn.SetBounds(math.Max(0, avail-btnW), (h-btnH)/2, btnW, btnH)
}

// OverflowItems returns the items the last horizontal layout could not fit,
// in their original order. They are hidden and reachable through the chevron.
func (this *ToolBar) OverflowItems() []IWidget {
	return this.overflow
}

// overflowButton returns the chevron, creating it on first use. It is a child
// of the toolbar so it draws and takes clicks like any other widget, but it is
// deliberately kept out of this.items: it is not part of the run it cuts.
func (this *ToolBar) overflowButton() *Button {
	if this.overflowBtn == nil {
		btn := NewButton1("More tools", paint.LoadIcon("toolbar-overflow"))
		btn.SetTextVisible(false)
		btn.SetParent(this)
		SetToolTip(btn, "更多工具")
		btn.Action().BindFunc0(func() {
			this.showOverflow()
		})
		this.overflowBtn = btn
	}
	return this.overflowBtn
}

// showToolBarPopup pops the overflow menu up at a global position. It is a
// variable rather than a direct ShowAsPopup call because ShowAsPopup creates an
// OS window, which AppKit allows on the main thread alone; tests replace it to
// drive the chevron's real action without one.
var showToolBarPopup = func(menu *Menu, xg, yg float64) {
	menu.ShowAsPopup(xg, yg, true)
}

// showOverflow builds a menu mirroring the hidden items and drops it under the
// chevron. The menu entries trigger the original items' actions, so an action
// does the same thing whether the window was wide enough for it or not.
func (this *ToolBar) showOverflow() {
	menu := NewPopupMenu()
	menu.SetParent(this)
	for _, item := range this.overflow {
		switch v := item.(type) {
		case *Button:
			if v.Action() != nil {
				menu.AddActionButton(&toolBarOverflowAction{IAction: v.Action(), source: v})
			}
		case *Separator:
			menu.AddSeparator()
		}
	}
	if len(menu.Items()) == 0 {
		return
	}
	btn := this.overflowButton()
	xg, yg := btn.MapToGlobal(0, 0)
	bw, bh := btn.Size()
	// Anchor the menu's trailing edge to the chevron, not its leading edge.
	// Otherwise a right-edge toolbar opens almost the entire menu outside its
	// owner even when it would fit inside. LayoutPopup still clamps to screen.
	showToolBarPopup(menu, xg+bw-menu.SizeHints().Width, yg+bh)
}

// toolBarOverflowAction preserves dynamic checked/enabled state while giving
// icon-only actions a readable menu name. Trigger checks the original button
// again, since its availability can change while a popup is already open.
type toolBarOverflowAction struct {
	IAction
	source *Button
}

func (a *toolBarOverflowAction) Text() string {
	if text := strings.TrimSpace(a.source.Text()); text != "" {
		return text
	}
	if tip := strings.TrimSpace(GetToolTip(a.source)); tip != "" {
		return tip
	}
	if name := strings.TrimSpace(a.source.Action().ObjName()); name != "" {
		return name
	}
	return "未命名操作"
}
func (a *toolBarOverflowAction) IsEnabled() bool { return a.source.IsEnabled() }
func (a *toolBarOverflowAction) IsChecked() bool {
	return a.source.Action() != nil && a.source.Action().IsChecked()
}
func (a *toolBarOverflowAction) Rev() uint64 {
	var rev uint64
	if a.source.Action() != nil {
		rev = a.source.Action().Rev() << 2
	}
	if a.IsEnabled() {
		rev |= 1
	}
	if a.IsChecked() {
		rev |= 2
	}
	return rev
}
func (a *toolBarOverflowAction) Trigger(interface{}) {
	if a.source.IsEnabled() {
		a.source.Action().Trigger(a.source)
	}
}

func (this *ToolBar) layoutVertical() {
	m := this.chromeMargin()
	w, _ := this.Self().Size()
	w = toolbarDimension(w)
	// Horizontal overflow is a previous orientation's state, not an explicit
	// user hide. Restore it before placing the vertical run.
	this.overflow = nil
	if this.overflowBtn != nil {
		this.overflowBtn.SetVisible(false)
	}
	y := m.T
	for _, item := range this.items {
		item.SetVisible(true)
		hints := this.itemSizeHints(item)
		iw, ih := toolbarDimension(hints.Width), toolbarDimension(hints.Height)
		if _, ok := item.(*Separator); ok {
			item.SetBounds(0, y, w, ih)
			y += ih + this.spacing
			continue
		}
		if _, button := item.(*Button); !button && hints.Policy&(GrowHorizontal|ExpandHorizontal) != 0 {
			iw = w
		}
		iw = math.Min(iw, w)
		item.SetBounds((w-iw)/2, y, iw, ih)
		y += ih + this.spacing
	}
}

func (this *ToolBar) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Self().Size()

	// background: use MenuBGColor for subtle differentiation from form
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(t.MenuBGColor)
	g.Fill()

	// bottom border (horizontal) or right border (vertical)
	if this.IsVertical() {
		g.Line(w-0.5, 0, w-0.5, h)
	} else {
		g.Line(0, h-0.5, w, h-0.5)
	}
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()
}

func (this *ToolBar) SizeHints() SizeHints {
	m := this.chromeMargin()
	var main, cross float64
	for i, item := range this.items {
		hints := this.itemSizeHints(item)
		w, h := toolbarDimension(hints.Width), toolbarDimension(hints.Height)
		if this.IsVertical() {
			w, h = h, w
		}
		main += w
		cross = math.Max(cross, h)
		if i > 0 {
			main += this.spacing
		}
	}
	if this.IsVertical() {
		if this.compact {
			cross += m.L + m.R
		}
		return SizeHints{Width: cross, Height: main + m.T + m.B, Policy: GrowVertical}
	}
	if this.compact {
		cross += m.T + m.B
	}
	return SizeHints{Width: main + m.L + m.R, Height: cross, Policy: GrowHorizontal}
}

func (this *ToolBar) OnIdle() {
	for _, v := range this.items {
		if v.IsVisible() {
			if im, ok := v.(interface {
				OnIdle()
			}); ok {
				im.OnIdle()
			}
		}
	}
}
