package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

// SearchBox 搜索框控件，带搜索图标和清除按钮
type SearchBox struct {
	Widget
	text          string
	placeholder   string
	focused       bool
	cursorPos     int
	cbSearch      func(string)
	cbTextChanged func(string)
	hoverClear    bool
}

func init() {
	core.RegisterFactory("gui.SearchBox", core.TypeOf((*SearchBox)(nil)))
}

// Compile-time checks: SearchBox must satisfy the keyboard and text-input
// event interfaces the window dispatches (window_glfw.go onKey/onChar), or
// keystrokes and typed characters silently never reach it.
var (
	_ IEventKeyDown   = (*SearchBox)(nil)
	_ IEventTextInput = (*SearchBox)(nil)
)

func NewSearchBox() *SearchBox {
	p := new(SearchBox)
	p.Init(p)
	p.placeholder = "搜索..."
	return p
}

func (this *SearchBox) Text() string {
	return this.text
}

func (this *SearchBox) SetText(s string) {
	if this.text != s {
		this.text = s
		this.cursorPos = len([]rune(s))
		this.fireTextChanged()
		this.Self().Update()
	}
}

func (this *SearchBox) Placeholder() string {
	return this.placeholder
}

func (this *SearchBox) SetPlaceholder(s string) {
	this.placeholder = s
	this.Self().Update()
}

func (this *SearchBox) Clear() {
	this.SetText("")
}

func (this *SearchBox) SigSearch(fn func(string)) {
	this.cbSearch = fn
}

func (this *SearchBox) SigTextChanged(fn func(string)) {
	this.cbTextChanged = fn
}

func (this *SearchBox) fireTextChanged() {
	if this.cbTextChanged != nil {
		this.cbTextChanged(this.text)
	}
}

// --- Events ---

func (this *SearchBox) OnMouseEnter() {
	this.Self().Update()
}

func (this *SearchBox) OnMouseLeave() {
	this.hoverClear = false
	this.Self().Update()
}

func (this *SearchBox) OnMouseMove(x, y float64) {
	w, _ := this.Size()
	clearZone := w - 24
	wasHover := this.hoverClear
	this.hoverClear = x >= clearZone && this.text != ""
	if wasHover != this.hoverClear {
		this.Self().Update()
	}
}

func (this *SearchBox) OnLeftDown(x, y float64) {
	w, _ := this.Size()
	clearZone := w - 24

	// click clear button
	if x >= clearZone && this.text != "" {
		this.Clear()
		return
	}

	this.SetFocus()
	this.focused = true
	this.Self().Update()
}

func (this *SearchBox) OnFocusIn() {
	this.focused = true
	this.Self().Update()
}

func (this *SearchBox) OnFocusOut() {
	this.focused = false
	this.Self().Update()
}

func (this *SearchBox) OnKeyDown(key int, repeat bool) {
	runes := []rune(this.text)
	switch key {
	case KeyBackSpace:
		if this.cursorPos > 0 && len(runes) > 0 {
			runes = append(runes[:this.cursorPos-1], runes[this.cursorPos:]...)
			this.cursorPos--
			this.text = string(runes)
			this.fireTextChanged()
			this.Self().Update()
		}
	case KeyDelete:
		if this.cursorPos < len(runes) {
			runes = append(runes[:this.cursorPos], runes[this.cursorPos+1:]...)
			this.text = string(runes)
			this.fireTextChanged()
			this.Self().Update()
		}
	case KeyLeft:
		if this.cursorPos > 0 {
			this.cursorPos--
			this.Self().Update()
		}
	case KeyRight:
		if this.cursorPos < len(runes) {
			this.cursorPos++
			this.Self().Update()
		}
	case KeyEnter:
		if this.cbSearch != nil {
			this.cbSearch(this.text)
		}
	case KeyHome:
		this.cursorPos = 0
		this.Self().Update()
	case KeyEnd:
		this.cursorPos = len(runes)
		this.Self().Update()
	}
}

// OnTextInput implements IEventTextInput: the window routes committed text
// here (already stripped of control chars by onChar). Each rune is inserted
// at the caret and advances it, matching the old per-char handler.
func (this *SearchBox) OnTextInput(s string) {
	if s == "" {
		return
	}
	runes := []rune(this.text)
	if this.cursorPos > len(runes) {
		this.cursorPos = len(runes)
	}
	ins := []rune(s)
	runes = append(runes[:this.cursorPos], append(ins, runes[this.cursorPos:]...)...)
	this.cursorPos += len(ins)
	this.text = string(runes)
	this.fireTextChanged()
	this.Self().Update()
}

// --- Drawing ---

func (this *SearchBox) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	iconSize := 16.0
	pad := 8.0
	textStartX := pad + iconSize + 6

	// background with rounded corners
	r := h / 2
	if r > 12 {
		r = 12
	}

	g.Save()

	// rounded rect path
	g.MoveTo(r, 0)
	g.LineTo(w-r, 0)
	g.Arc(w-r, r, r, -math.Pi/2, 0)
	g.LineTo(w, h-r)
	g.Arc(w-r, h-r, r, 0, math.Pi/2)
	g.LineTo(r, h)
	g.Arc(r, h-r, r, math.Pi/2, math.Pi)
	g.LineTo(0, r)
	g.Arc(r, r, r, math.Pi, 3*math.Pi/2)
	g.LineTo(r, 0)

	// fill background
	if this.focused {
		g.SetBrush1(t.ViewBGColor)
	} else {
		g.SetBrush1(t.FormColor)
	}
	g.FillPreserve()

	// border
	if this.focused {
		g.SetPen1(t.HighLightColor, 1.5)
	} else if this.IsHover() {
		g.SetPen1(t.FormDarkColor, 1)
	} else {
		g.SetPen1(t.BorderColor, 1)
	}
	g.Stroke()

	// search icon (magnifying glass)
	iconX := pad + iconSize/2
	iconY := h / 2
	glassR := 5.0
	g.Arc(iconX-1, iconY-1, glassR, 0, 2*math.Pi)
	g.SetPen1(t.MenuGrayTextColor, 1.5)
	g.Stroke()
	// handle
	hx := iconX - 1 + glassR*math.Cos(math.Pi/4)
	hy := iconY - 1 + glassR*math.Sin(math.Pi/4)
	g.MoveTo(hx, hy)
	g.LineTo(hx+4, hy+4)
	g.Stroke()

	// text or placeholder
	g.SetFont(t.Font)
	fe := t.Font.FontExtents()
	textY := 0.5 * (h + fe.Ascent - fe.Descent)

	if this.text == "" && !this.focused {
		// placeholder
		g.SetBrush1(t.MenuGrayTextColor)
		g.Translate(textStartX, textY)
		g.DrawText(this.placeholder)
		g.Translate(-textStartX, -textY)
	} else {
		// actual text
		g.SetBrush1(t.TextColor)
		g.Translate(textStartX, textY)
		g.DrawText(this.text)
		g.Translate(-textStartX, -textY)

		// cursor
		if this.focused {
			prefix := string([]rune(this.text)[:this.cursorPos])
			ext := t.Font.TextExtents(prefix)
			cx := textStartX + ext.Width + ext.XBearing
			g.MoveTo(cx, (h-fe.Height)/2)
			g.LineTo(cx, (h+fe.Height)/2)
			g.SetPen1(t.TextColor, 1)
			g.Stroke()
		}
	}

	// clear button (X) when text is not empty
	if this.text != "" {
		clearX := w - 16
		clearY := h / 2
		clearR := 4.0
		if this.hoverClear {
			g.SetPen1(t.FormDarkColor, 1.5)
		} else {
			g.SetPen1(t.MenuGrayTextColor, 1.5)
		}
		g.MoveTo(clearX-clearR, clearY-clearR)
		g.LineTo(clearX+clearR, clearY+clearR)
		g.Stroke()
		g.MoveTo(clearX+clearR, clearY-clearR)
		g.LineTo(clearX-clearR, clearY+clearR)
		g.Stroke()
	}

	g.Restore()
}

func (this *SearchBox) SizeHints() SizeHints {
	t := Theme()
	fe := t.Font.FontExtents()
	h := fe.Height + 16
	return SizeHints{Width: 180, Height: h, Policy: GrowHorizontal | GrowVertical}
}

func (this *SearchBox) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("占位文本", this.Placeholder, this.SetPlaceholder)
}
