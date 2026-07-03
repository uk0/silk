package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("ged.KeymapPanel", gui.TypeOf(KeymapPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.KeymapPanel",
		Name: "快捷键设置",
		Icon: "edit",
		Desc: "自定义键盘快捷键",
	})
}

// KeymapPanel is a simple, scrollable tool-view for browsing and editing
// keyboard shortcuts. Rows show the command identifier, its current key
// chord, and the context (global / editor). Clicking a row prompts the user
// for a new chord via ShowInputDialog and persists the change to disk.
type KeymapPanel struct {
	gui.Widget
	keymap     *KeyMap
	hoverIdx   int
	scrollY    float64
	editingIdx int
	rowHeight  float64
}

// NewKeymapPanel creates a new keymap panel bound to the shared global
// keymap (LoadKeymap()).
func NewKeymapPanel() *KeymapPanel {
	p := new(KeymapPanel)
	p.Init(p)
	return p
}

// Init is called by the factory and by NewKeymapPanel. Fields are
// initialized to sensible defaults and the shared keymap is loaded.
func (this *KeymapPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.hoverIdx = -1
	this.editingIdx = -1
	this.scrollY = 0
	this.rowHeight = 26
	this.keymap = LoadKeymap()
}

// Keymap returns the KeyMap instance this panel displays and edits.
func (this *KeymapPanel) Keymap() *KeyMap {
	return this.keymap
}

// Draw renders the panel. Layout:
//
//	Header (title + reset hint) | rowHeight high
//	For each binding: [command] ................... [Key chord] [Context]
func (this *KeymapPanel) Draw(g paint.Painter) {
	t := gui.Theme()
	w, h := this.Size()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	titleFont := paint.NewFont(t.Font.Family(), 13, true, false)
	normalFont := paint.NewFont(t.Font.Family(), 11, false, false)
	keyFont := paint.NewFont(t.Font.Family(), 11, true, false)

	// Header
	headerH := 30.0
	g.SetBrush1(paint.Color{R: t.FormColor.R, G: t.FormColor.G, B: t.FormColor.B, A: 255})
	g.Rectangle(0, 0, w, headerH)
	g.Fill()

	g.SetFont(titleFont)
	g.SetBrush1(t.HighLightColor)
	g.Translate(12, 20)
	g.DrawText("快捷键设置")
	g.Translate(-12, -20)

	// Hint (right-aligned)
	g.SetFont(normalFont)
	g.SetBrush1(t.TextColor)
	hint := "点击快捷键进行修改"
	hintExt := normalFont.TextExtents(hint)
	g.Translate(w-hintExt.XAdvance-12, 20)
	g.DrawText(hint)
	g.Translate(-(w - hintExt.XAdvance - 12), -20)

	// Bottom border of header
	g.SetPen1(t.BorderColor, 1)
	g.Line(0, headerH, w, headerH)
	g.Stroke()

	// Rows
	bindings := this.keymap.Bindings()
	y := headerH - this.scrollY
	const (
		leftPad   = 12.0
		keyColPad = 140.0 // right-edge pad for the key badge column
		ctxColPad = 50.0  // right-edge pad for context column
	)

	for i, b := range bindings {
		if y+this.rowHeight < headerH {
			y += this.rowHeight
			continue
		}
		if y > h {
			break
		}

		// Row background (hover highlight)
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: t.HighLightColor.R, G: t.HighLightColor.G, B: t.HighLightColor.B, A: 28})
			g.Rectangle(0, y, w, this.rowHeight)
			g.Fill()
		}

		// Alternating row separator
		g.SetPen1(paint.Color{R: t.BorderColor.R, G: t.BorderColor.G, B: t.BorderColor.B, A: 120}, 1)
		g.Line(leftPad, y+this.rowHeight, w-leftPad, y+this.rowHeight)
		g.Stroke()

		// Command label
		g.SetFont(normalFont)
		g.SetBrush1(t.TextColor)
		cmdY := y + this.rowHeight*0.5 + 4
		g.Translate(leftPad, cmdY)
		g.DrawText(b.Command)
		g.Translate(-leftPad, -cmdY)

		// Key chord badge (right side)
		keyX := w - keyColPad
		keyExt := keyFont.TextExtents(b.Key)
		badgeW := keyExt.Width + 14
		badgeH := this.rowHeight - 10
		badgeX := keyX
		badgeY := y + (this.rowHeight-badgeH)/2

		g.SetBrush1(paint.Color{R: t.FormColor.R, G: t.FormColor.G, B: t.FormColor.B, A: 255})
		g.Rectangle(badgeX, badgeY, badgeW, badgeH)
		g.FillPreserve()
		g.SetPen1(t.BorderColor, 1)
		g.Stroke()

		g.SetFont(keyFont)
		g.SetBrush1(t.TextColor)
		kx := badgeX + 7
		ky := badgeY + badgeH*0.5 + keyExt.Height*0.5 - 1
		g.Translate(kx, ky)
		g.DrawText(b.Key)
		g.Translate(-kx, -ky)

		// Context label
		g.SetFont(normalFont)
		ctxColor := paint.Color{R: 128, G: 128, B: 128, A: 255}
		g.SetBrush1(ctxColor)
		ctxExt := normalFont.TextExtents(b.Context)
		cxX := w - ctxColPad + (40-ctxExt.XAdvance)/2
		g.Translate(cxX, cmdY)
		g.DrawText(b.Context)
		g.Translate(-cxX, -cmdY)

		y += this.rowHeight
	}

	// Footer hint line
	if len(bindings) == 0 {
		g.SetFont(normalFont)
		g.SetBrush1(t.TextColor)
		empty := "（无快捷键）"
		emptyExt := normalFont.TextExtents(empty)
		g.Translate((w-emptyExt.XAdvance)/2, h/2)
		g.DrawText(empty)
		g.Translate(-(w-emptyExt.XAdvance)/2, -h/2)
	}
}

// rowAt returns the binding index at pointer y, or -1 if y is over the
// header or past the end of the list.
func (this *KeymapPanel) rowAt(y float64) int {
	const headerH = 30.0
	if y < headerH {
		return -1
	}
	idx := int((y - headerH + this.scrollY) / this.rowHeight)
	if idx < 0 || idx >= this.keymap.Len() {
		return -1
	}
	return idx
}

// OnMouseMove updates the hover highlight.
func (this *KeymapPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *KeymapPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnLeftDown opens an input dialog to edit the clicked binding's chord.
func (this *KeymapPanel) OnLeftDown(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 {
		return
	}
	bindings := this.keymap.Bindings()
	if idx >= len(bindings) {
		return
	}
	b := bindings[idx]
	this.editingIdx = idx

	newKey, ok := gui.ShowInputDialog(this.Self(), "修改快捷键", b.Command, b.Key)
	if ok && newKey != "" && newKey != b.Key {
		this.keymap.Set(b.Command, newKey)
		// Persist best-effort; errors are logged but don't block the UI.
		if err := SaveKeymap(this.keymap); err != nil {
			core.Warn("SaveKeymap: ", err)
		}
	}
	this.editingIdx = -1
	this.Self().Update()
}

// OnMouseWheel vertically scrolls the binding list.
func (this *KeymapPanel) OnMouseWheel(x, y, delta float64) {
	this.scrollY -= delta * 24
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	max := this.contentHeight() - this.Height() + 30
	if max < 0 {
		max = 0
	}
	this.scrollY = math.Min(this.scrollY, max)
	this.Self().Update()
}

// contentHeight is the total pixel height of the binding list (excluding
// the header).
func (this *KeymapPanel) contentHeight() float64 {
	return float64(this.keymap.Len()) * this.rowHeight
}

// SizeHints advertises a reasonable default panel size.
func (this *KeymapPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 360, Height: 480}
}
