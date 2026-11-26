package gui

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// SearchBox 搜索框控件，带搜索图标和清除按钮
type SearchBox struct {
	Widget
	text            string
	placeholder     string
	focused         bool
	cursorPos       int
	selectionAnchor int
	scrollX         float64
	cbSearch        func(string)
	cbTextChanged   func(string)
	hoverClear      bool
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

// Geometry shared by Draw and the mouse hit tests. The text origin has to be
// the same number in both or a click lands on the wrong character, so it lives
// here rather than as a local in Draw.
const (
	searchBoxPad      = 8.0
	searchBoxIconSize = 16.0
	searchBoxTextX    = searchBoxPad + searchBoxIconSize + 6
	// searchBoxClearW is the width of the right-hand strip holding the clear
	// button; a click inside it clears instead of moving the caret.
	searchBoxClearW = 24.0
)

// searchBoxShapingText preserves the old rune-prefix replacement semantics
// for programmatically supplied invalid UTF-8 without copying valid text.
func searchBoxShapingText(text string) string {
	if utf8.ValidString(text) {
		return text
	}
	return string([]rune(text))
}

// searchBoxCaretX returns the x offset, measured from the text origin, of the
// caret sitting after the first n runes: the pen advance of that prefix. The
// ink box (Width+XBearing) is deliberately not used — a prefix ending in a
// space has no ink past the previous glyph.
func searchBoxCaretX(text string, font paint.Font, n int) float64 {
	return searchBoxPrefixAdvance(searchBoxShapingText(text), font, n)
}

// The caller has normalized UTF-8 once. Iterating rune boundaries keeps this
// slice on the original text bytes rather than rebuilding a string each time.
func searchBoxPrefixAdvance(text string, font paint.Font, n int) float64 {
	if n <= 0 || text == "" {
		return 0
	}
	for i := range text {
		if n == 0 {
			return font.TextExtents(text[:i]).XAdvance
		}
		n--
	}
	return font.TextExtents(text).XAdvance
}

// searchBoxCaretIndex maps an x offset, measured from the text origin, to the
// rune index whose caret offset is nearest. Preserve prefix shaping and exact
// midpoint ties, including zero-width and combining glyphs.
func searchBoxCaretIndex(text string, font paint.Font, x float64) int {
	if x <= 0 || text == "" {
		return 0
	}
	text = searchBoxShapingText(text)
	prev := 0.0
	n := 0
	for end := 0; end < len(text); n++ {
		_, size := utf8.DecodeRuneInString(text[end:])
		end += size
		cur := font.TextExtents(text[:end]).XAdvance
		if x < (prev+cur)*0.5 {
			return n
		}
		prev = cur
	}
	return n
}

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
		this.selectionAnchor = this.cursorPos
		this.scrollX = 0
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

// SelectAll uses rune positions, like the editing fields, so replacing a
// query works identically for Latin text, CJK and supplementary characters.
func (this *SearchBox) SelectAll() {
	this.selectionAnchor = 0
	this.cursorPos = len([]rune(this.text))
	this.Self().Update()
}

func (this *SearchBox) selection() (int, int) {
	n := len([]rune(this.text))
	a := max(0, min(this.selectionAnchor, n))
	b := max(0, min(this.cursorPos, n))
	return min(a, b), max(a, b)
}

func (this *SearchBox) SelectionText() string {
	a, b := this.selection()
	return string([]rune(this.text)[a:b])
}

func (this *SearchBox) replaceSelection(text string) {
	a, b := this.selection()
	runes := []rune(this.text)
	ins := []rune(text)
	result := append(append(append([]rune{}, runes[:a]...), ins...), runes[b:]...)
	this.cursorPos = a + len(ins)
	this.selectionAnchor = this.cursorPos
	if next := string(result); next != this.text {
		this.text = next
		this.fireTextChanged()
	}
	this.Self().Update()
}

func (this *SearchBox) moveCaret(pos int, extend bool) {
	this.cursorPos = max(0, min(pos, len([]rune(this.text))))
	if !extend {
		this.selectionAnchor = this.cursorPos
	}
	this.Self().Update()
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
	clearZone := w - searchBoxClearW
	wasHover := this.hoverClear
	this.hoverClear = x >= clearZone && this.text != ""
	if wasHover != this.hoverClear {
		this.Self().Update()
	}
}

func (this *SearchBox) OnLeftDown(x, y float64) {
	if !this.IsEnabled() {
		return
	}
	w, _ := this.Size()
	clearZone := w - searchBoxClearW

	// click clear button
	if x >= clearZone && this.text != "" {
		this.Clear()
		return
	}

	this.SetFocus()
	this.focused = true
	this.moveCaret(searchBoxCaretIndex(this.text, Theme().Font, x-searchBoxTextX+this.scrollX), IsKeyDown(KeyShift))
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
	if !this.IsEnabled() {
		return
	}
	runes := []rune(this.text)
	if IsKeyDown(KeyCtrl) || isActionModifier() {
		switch key {
		case 'A':
			this.SelectAll()
			return
		case 'C', 'X':
			if text := this.SelectionText(); text != "" {
				Clipboard.Clear()
				Clipboard.SetData(text)
				if key == 'X' {
					this.replaceSelection("")
				}
			}
			return
		case 'V':
			this.pasteClipboard()
			return
		}
	}
	a, b := this.selection()
	shift := IsKeyDown(KeyShift)
	switch key {
	case KeyBackSpace:
		if a == b && this.cursorPos > 0 {
			this.selectionAnchor = this.cursorPos - 1
		}
		this.replaceSelection("")
	case KeyDelete:
		if a == b && this.cursorPos < len(runes) {
			this.selectionAnchor = this.cursorPos + 1
		}
		this.replaceSelection("")
	case KeyLeft:
		if a != b && !shift {
			this.moveCaret(a, false)
		} else {
			this.moveCaret(this.cursorPos-1, shift)
		}
	case KeyRight:
		if a != b && !shift {
			this.moveCaret(b, false)
		} else {
			this.moveCaret(this.cursorPos+1, shift)
		}
	case KeyEnter:
		if this.cbSearch != nil {
			this.cbSearch(this.text)
		}
	case KeyHome:
		this.moveCaret(0, shift)
	case KeyEnd:
		this.moveCaret(len(runes), shift)
	}
}

// OnTextInput implements IEventTextInput: the window routes committed text
// here (already stripped of control chars by onChar). Each rune is inserted
// at the caret and advances it, matching the old per-char handler.
func (this *SearchBox) OnTextInput(s string) {
	if !this.IsEnabled() || s == "" {
		return
	}
	// Clipboard paste is still a single-line query, not a multiline editor.
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if s == "" {
		return
	}
	this.replaceSelection(s)
}

// --- Drawing ---

func (this *SearchBox) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()
	iconSize := searchBoxIconSize
	pad := searchBoxPad
	textStartX := searchBoxTextX

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
	// Text/caret stay in their own strip; long queries must not cover the
	// search or clear icons, or float beyond the palette's parent panel.
	textWidth := math.Max(0, w-searchBoxTextX-searchBoxClearW)
	shaped := searchBoxShapingText(this.text)
	caretX := searchBoxPrefixAdvance(shaped, t.Font, this.cursorPos)
	this.scrollX = math.Max(0, math.Min(this.scrollX, caretX))
	if caretX-this.scrollX > math.Max(0, textWidth-2) {
		this.scrollX = caretX - math.Max(0, textWidth-2)
	}
	g.Save()
	g.Rectangle(textStartX, 0, textWidth, h)
	g.Clip()
	textStartX -= this.scrollX
	a, b := this.selection()
	selected := this.focused && a != b
	var x0, x1 float64
	if selected {
		// A selection endpoint is always the caret. Shape the other
		// endpoint just once for both the background and highlighted ink.
		if this.cursorPos <= a {
			x0, x1 = caretX, searchBoxPrefixAdvance(shaped, t.Font, b)
		} else {
			x0, x1 = searchBoxPrefixAdvance(shaped, t.Font, a), caretX
		}
		x0, x1 = textStartX+x0, textStartX+x1
		g.SetBrush1(t.MenuActiveBGColor)
		g.Rectangle(x0, (h-fe.Height)/2, x1-x0, fe.Height)
		g.Fill()
	}

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
		if selected {
			g.Save()
			g.Rectangle(x0, 0, x1-x0, h)
			g.Clip()
			g.SetBrush1(t.MenuActiveTextColor)
			g.Translate(textStartX, textY)
			g.DrawText(this.text)
			g.Restore()
		}

		// cursor
		if this.focused {
			cx := textStartX + caretX
			g.MoveTo(cx, (h-fe.Height)/2)
			g.LineTo(cx, (h+fe.Height)/2)
			g.SetPen1(t.TextColor, 1)
			g.Stroke()
		}
	}
	g.Restore()

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

func (this *SearchBox) pasteClipboard() {
	text, pos, anchor := this.text, this.cursorPos, this.selectionAnchor
	if this.tryAsyncPaste(func() bool { return this.text == text && this.cursorPos == pos && this.selectionAnchor == anchor }, this.OnTextInput) {
		return
	}
	if data, err := Clipboard.Data("text/plain"); err == nil {
		if text, ok := data.(string); ok {
			this.OnTextInput(text)
		}
	}
}
