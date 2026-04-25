package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.ToolBar", core.TypeOf((*ToolBar)(nil)))
}

// ToolBar is a horizontal or vertical bar for tool buttons, separators,
// and arbitrary widgets. Similar to QToolBar.
type ToolBar struct {
	Widget
	items       []IWidget
	orientation int     // 0=horizontal, 1=vertical
	iconSize    int     // icon display size in pixels
	spacing     float64 // gap between items
}

func NewToolBar() *ToolBar {
	p := new(ToolBar)
	p.Init(p)
	p.iconSize = int(Theme().IconSize)
	p.spacing = Theme().Spacing
	return p
}

func (this *ToolBar) EnumProperties(list core.IPropertyList) {
	list.AddProperty("图标大小", this.IconSize, this.SetIconSize)
	list.AddProperty("间距", this.Spacing, this.SetSpacing)
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

func (this *ToolBar) SetSpacing(s float64) {
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

func (this *ToolBar) layoutHorizontal() {
	t := Theme()
	m := t.ButtonMargin
	w, h := this.Self().Size()
	_ = w

	x := m.L
	for _, item := range this.items {
		hints := item.SizeHints()
		// separators get a small fixed width
		if _, ok := item.(*Separator); ok {
			sz := t.SeparatorSize + 6
			item.SetBounds(x, 4, sz, h-8)
			x += sz + this.spacing
			continue
		}
		// buttons: size depends on whether they have an icon, text, or both
		if btn, ok := item.(*Button); ok {
			hasIcon := btn.Icon() != nil && !btn.Icon().IsAir()
			text := btn.Text()

			var btnW, btnH float64
			if hasIcon && text == "" {
				// Icon-only button (standard Qt Creator toolbar style)
				btnW = float64(this.iconSize) + m.L + m.R + 4
				btnH = float64(this.iconSize) + m.T + m.B + 4
			} else if hasIcon && text != "" {
				// Icon + text button
				textW := t.Font.TextExtents(text).Width
				btnW = float64(this.iconSize) + textW + m.L + m.R + 10
				btnH = float64(this.iconSize) + m.T + m.B + 4
			} else {
				// Text-only button — size by text content
				textW := t.Font.TextExtents(text).Width
				fe := t.Font.FontExtents()
				btnW = textW + m.L + m.R + 12
				btnH = fe.Height + m.T + m.B + 6
			}
			y := (h - btnH) * 0.5
			if y < 0 {
				y = 0
			}
			item.SetBounds(x, y, btnW, btnH)
			x += btnW + this.spacing
			continue
		}
		// custom widgets use their SizeHints
		iw := hints.Width
		ih := hints.Height
		if (hints.Policy & (GrowVertical | ExpandVertical)) != 0 {
			ih = h
		}
		y := (h - ih) * 0.5
		if y < 0 {
			y = 0
		}
		item.SetBounds(x, y, iw, ih)
		x += iw + this.spacing
	}
}

func (this *ToolBar) layoutVertical() {
	t := Theme()
	m := t.ButtonMargin
	w, h := this.Self().Size()
	_ = h

	y := m.T
	for _, item := range this.items {
		hints := item.SizeHints()
		if _, ok := item.(*Separator); ok {
			sz := t.SeparatorSize
			item.SetBounds(0, y, w, sz)
			y += sz + this.spacing
			continue
		}
		if _, ok := item.(*Button); ok {
			btnW := float64(this.iconSize) + m.L + m.R
			btnH := float64(this.iconSize) + m.T + m.B
			x := (w - btnW) * 0.5
			if x < 0 {
				x = 0
			}
			item.SetBounds(x, y, btnW, btnH)
			y += btnH + this.spacing
			continue
		}
		iw := hints.Width
		ih := hints.Height
		if (hints.Policy & (GrowHorizontal | ExpandHorizontal)) != 0 {
			iw = w
		}
		x := (w - iw) * 0.5
		if x < 0 {
			x = 0
		}
		item.SetBounds(x, y, iw, ih)
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
	t := Theme()
	m := t.ButtonMargin
	if this.IsVertical() {
		var maxW, totalH float64
		for i, item := range this.items {
			if _, ok := item.(*Separator); ok {
				totalH += t.SeparatorSize
			} else if _, ok := item.(*Button); ok {
				bw := float64(this.iconSize) + m.L + m.R
				bh := float64(this.iconSize) + m.T + m.B
				maxW = math.Max(maxW, bw)
				totalH += bh
			} else {
				hints := item.SizeHints()
				maxW = math.Max(maxW, hints.Width)
				totalH += hints.Height
			}
			if i > 0 {
				totalH += this.spacing
			}
		}
		return SizeHints{Width: maxW, Height: totalH + m.T + m.B, Policy: GrowVertical}
	}

	// horizontal
	var totalW, maxH float64
	for i, item := range this.items {
		if _, ok := item.(*Separator); ok {
			totalW += t.SeparatorSize
		} else if _, ok := item.(*Button); ok {
			bw := float64(this.iconSize) + m.L + m.R
			bh := float64(this.iconSize) + m.T + m.B
			totalW += bw
			maxH = math.Max(maxH, bh)
		} else {
			hints := item.SizeHints()
			totalW += hints.Width
			maxH = math.Max(maxH, hints.Height)
		}
		if i > 0 {
			totalW += this.spacing
		}
	}
	return SizeHints{Width: totalW + m.L + m.R, Height: maxH, Policy: GrowHorizontal}
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
