package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

// DropdownItem represents an item in the dropdown menu.
type DropdownItem struct {
	Text string
	Icon paint.Icon
	Data interface{}
}

// DropdownButton is a button that opens a dropdown menu when clicked.
type DropdownButton struct {
	Widget
	text     string
	items    []DropdownItem
	selected int
	opened   bool
	cbSelect func(int, string)
}

func init() {
	core.RegisterFactory("gui.DropdownButton", core.TypeOf((*DropdownButton)(nil)))
}

func NewDropdownButton() *DropdownButton {
	p := new(DropdownButton)
	p.Init(p)
	p.text = ""
	p.selected = -1
	return p
}

func (this *DropdownButton) Text() string { return this.text }

func (this *DropdownButton) SetText(s string) {
	this.text = s
	this.Self().Update()
}

func (this *DropdownButton) Selected() int { return this.selected }

func (this *DropdownButton) SetSelected(idx int) {
	if idx >= 0 && idx < len(this.items) {
		this.selected = idx
		this.text = this.items[idx].Text
	} else {
		this.selected = -1
	}
	this.Self().Update()
}

func (this *DropdownButton) Items() []DropdownItem { return this.items }

func (this *DropdownButton) SetItems(items []DropdownItem) {
	this.items = items
	this.Self().Update()
}

func (this *DropdownButton) AddItem(text string, icon paint.Icon, data interface{}) {
	this.items = append(this.items, DropdownItem{Text: text, Icon: icon, Data: data})
	this.Self().Update()
}

func (this *DropdownButton) SigSelect(fn func(int, string)) {
	this.cbSelect = fn
}

// --- Events ---

func (this *DropdownButton) OnMouseEnter() {
	this.Self().Update()
}

func (this *DropdownButton) OnMouseLeave() {
	this.Self().Update()
}

func (this *DropdownButton) OnLeftDown(x, y float64) {
	this.SetFocus()
	this.Self().Update()
}

func (this *DropdownButton) OnLeftUp(x, y float64) {
	if this.IsHover() {
		this.showDropdown()
	}
}

func (this *DropdownButton) showDropdown() {
	if len(this.items) == 0 {
		return
	}

	menu := NewPopupMenu()
	for i, item := range this.items {
		idx := i
		txt := item.Text
		btn := menu.AddButton1(txt, item.Icon)
		btn.Action().BindFunc0(func() {
			this.selected = idx
			this.text = txt
			if this.cbSelect != nil {
				this.cbSelect(idx, txt)
			}
			this.Self().Update()
		})
	}

	xg, yg := this.MapToGlobal(0, 0)
	_, h := this.Size()
	menu.ShowAsPopup(xg, yg+h, true)
	this.opened = true
}

func (this *DropdownButton) drawRoundedRect(g paint.Painter, x, y, w, h, r float64) {
	g.MoveTo(x+r, y)
	g.LineTo(x+w-r, y)
	g.Arc(x+w-r, y+r, r, -math.Pi/2, 0)
	g.LineTo(x+w, y+h-r)
	g.Arc(x+w-r, y+h-r, r, 0, math.Pi/2)
	g.LineTo(x+r, y+h)
	g.Arc(x+r, y+h-r, r, math.Pi/2, math.Pi)
	g.LineTo(x, y+r)
	g.Arc(x+r, y+r, r, math.Pi, 3*math.Pi/2)
	g.LineTo(x+r, y)
}

// --- Drawing ---

func (this *DropdownButton) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	r := 4.0

	g.Save()

	// background
	this.drawRoundedRect(g, 0, 0, w, h, r)
	if this.IsHover() {
		g.SetBrush1(t.FormLightColor)
	} else {
		g.SetBrush1(t.FormColor)
	}
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// text
	displayText := this.text
	if displayText == "" {
		displayText = "Select"
	}
	f := t.Font
	g.SetFont(f)
	ext := f.TextExtents(displayText)
	tx := 8.0 - ext.XBearing
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	g.SetBrush1(t.TextColor)
	g.Translate(tx, ty)
	g.DrawText(displayText)
	g.Translate(-tx, -ty)

	// down arrow on right side
	arrowX := w - 16
	arrowY := h / 2
	arrowS := 4.0
	g.MoveTo(arrowX-arrowS, arrowY-arrowS/2)
	g.LineTo(arrowX, arrowY+arrowS/2)
	g.LineTo(arrowX+arrowS, arrowY-arrowS/2)
	g.SetPen1(t.FormDarkColor, 1.5)
	g.Stroke()

	g.Restore()
}

func (this *DropdownButton) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	w := 120.0
	h := fe.Height + 10
	if h < 28 {
		h = 28
	}
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}

func (this *DropdownButton) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("选中", this.Selected, this.SetSelected)
}
