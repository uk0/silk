package gui

import (
	"fmt"
	"silk/core"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.ColorPicker", core.TypeOf((*ColorPicker)(nil)))
}

// ColorPicker is a color selection widget that displays the current color
// as a small swatch with its hex value. Clicking opens a popup palette.
type ColorPicker struct {
	Widget
	color           paint.Color
	pushed          bool
	cbColorChanged  func(paint.Color)
	popup           *colorPalettePopup
}

// NewColorPicker creates a new ColorPicker with a default blue color.
func NewColorPicker() *ColorPicker {
	p := new(ColorPicker)
	p.Init(p)
	p.color = paint.Color{66, 133, 244, 255}
	return p
}

// Color returns the currently selected color.
func (this *ColorPicker) Color() paint.Color { return this.color }

// SetColor sets the selected color.
func (this *ColorPicker) SetColor(c paint.Color) {
	changed := this.color != c
	this.color = c
	if changed && this.cbColorChanged != nil {
		this.cbColorChanged(c)
	}
	this.Self().Update()
}

// SigColorChanged sets the callback for when the color changes.
func (this *ColorPicker) SigColorChanged(fn func(paint.Color)) {
	this.cbColorChanged = fn
}

func (this *ColorPicker) hexText() string {
	return fmt.Sprintf("#%02X%02X%02X", this.color.R, this.color.G, this.color.B)
}

// --- Drawing ---

func (this *ColorPicker) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	// Background
	g.Rectangle(0, 0, w, h)
	if this.pushed {
		g.SetBrush1(paint.Color{220, 220, 220, 255})
	} else if this.IsHover() {
		g.SetBrush1(paint.Color{235, 235, 235, 255})
	} else {
		g.SetBrush1(paint.Color{245, 245, 245, 255})
	}
	g.Fill()

	// Border
	g.Rectangle(0, 0, w, h)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Color swatch
	swatchSize := h - 8
	swatchX := 4.0
	swatchY := 4.0
	g.Rectangle(swatchX, swatchY, swatchSize, swatchSize)
	g.SetBrush1(this.color)
	g.FillPreserve()
	g.SetPen1(paint.Color{180, 180, 180, 255}, 1)
	g.Stroke()

	// Hex text
	text := this.hexText()
	g.SetFont(t.Font)
	g.SetBrush1(t.TextColor)
	ext := t.Font.TextExtents(text)
	tx := swatchX + swatchSize + 6 - ext.XBearing
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	g.Translate(tx, ty)
	g.DrawText(text)
	g.Translate(-tx, -ty)
}

// --- Events ---

func (this *ColorPicker) OnMouseEnter() { this.Self().Update() }
func (this *ColorPicker) OnMouseLeave() { this.Self().Update() }

func (this *ColorPicker) OnLeftDown(x, y float64) {
	this.SetFocus()
	this.pushed = true
	this.Self().Update()
	this.showPopup()
}

func (this *ColorPicker) OnLeftUp(x, y float64) {
	this.pushed = false
	this.Self().Update()
}

func (this *ColorPicker) showPopup() {
	if this.popup != nil && this.popup.IsVisible() {
		this.popup.Hide()
		return
	}
	popup := newColorPalettePopup(this)
	this.popup = popup
	gx, gy := this.MapToGlobal(0, this.h)
	popup.ShowAsPopup(gx, gy)
}

func (this *ColorPicker) SizeHints() SizeHints {
	return SizeHints{Width: 120, Height: 26, Policy: GrowHorizontal | GrowVertical}
}

func (this *ColorPicker) EnumProperties(list core.IPropertyList) {
	list.AddProperty("R", func() int { return int(this.color.R) }, func(v int) {
		this.SetColor(paint.Color{uint8(v), this.color.G, this.color.B, this.color.A})
	})
	list.AddProperty("G", func() int { return int(this.color.G) }, func(v int) {
		this.SetColor(paint.Color{this.color.R, uint8(v), this.color.B, this.color.A})
	})
	list.AddProperty("B", func() int { return int(this.color.B) }, func(v int) {
		this.SetColor(paint.Color{this.color.R, this.color.G, uint8(v), this.color.A})
	})
}

// --- Color Palette Popup ---

// paletteColors is an 8x6 grid of common colors.
var paletteColors = []paint.Color{
	// Row 1 - reds/pinks
	{244, 67, 54, 255}, {233, 30, 99, 255}, {156, 39, 176, 255}, {103, 58, 183, 255},
	{63, 81, 181, 255}, {33, 150, 243, 255}, {3, 169, 244, 255}, {0, 188, 212, 255},
	// Row 2 - greens/teals
	{0, 150, 136, 255}, {76, 175, 80, 255}, {139, 195, 74, 255}, {205, 220, 57, 255},
	{255, 235, 59, 255}, {255, 193, 7, 255}, {255, 152, 0, 255}, {255, 87, 34, 255},
	// Row 3 - light variants
	{239, 154, 154, 255}, {244, 143, 177, 255}, {206, 147, 216, 255}, {179, 157, 219, 255},
	{159, 168, 218, 255}, {144, 202, 249, 255}, {129, 212, 250, 255}, {128, 222, 234, 255},
	// Row 4 - more light variants
	{128, 203, 196, 255}, {165, 214, 167, 255}, {197, 225, 165, 255}, {230, 238, 156, 255},
	{255, 245, 157, 255}, {255, 224, 130, 255}, {255, 204, 128, 255}, {255, 171, 145, 255},
	// Row 5 - dark variants
	{183, 28, 28, 255}, {136, 14, 79, 255}, {74, 20, 140, 255}, {49, 27, 146, 255},
	{26, 35, 126, 255}, {13, 71, 161, 255}, {1, 87, 155, 255}, {0, 96, 100, 255},
	// Row 6 - grays + black/white
	{0, 0, 0, 255}, {66, 66, 66, 255}, {117, 117, 117, 255}, {158, 158, 158, 255},
	{189, 189, 189, 255}, {224, 224, 224, 255}, {245, 245, 245, 255}, {255, 255, 255, 255},
}

const (
	paletteCellSize = 28.0
	palettePadding  = 4.0
	paletteCols     = 8
	paletteRows     = 6
)

type colorPalettePopup struct {
	Widget
	owner    *ColorPicker
	hoverIdx int
}

func newColorPalettePopup(owner *ColorPicker) *colorPalettePopup {
	p := new(colorPalettePopup)
	p.Init(p)
	p.owner = owner
	p.hoverIdx = -1
	p.SetParent(owner)
	return p
}

func (this *colorPalettePopup) ShowAsPopup(xg, yg float64) {
	this.AttachWindow(WtPopup)
	if w := this.Window(); w != nil {
		w.SetCloseOnHide(true)
	}
	w := palettePadding*2 + paletteCellSize*paletteCols
	h := palettePadding*2 + paletteCellSize*paletteRows
	this.SetSize(0, 0)
	this.SetSize(w, h)
	LayoutPopup1(this.Self(), xg, yg)
	this.SetVisible(true)
	this.PushCapture()
}

func (this *colorPalettePopup) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	// Background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(paint.Color{255, 255, 255, 255})
	g.FillPreserve()
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Draw color cells
	for i, c := range paletteColors {
		if i >= paletteCols*paletteRows {
			break
		}
		col := i % paletteCols
		row := i / paletteCols
		cx := palettePadding + float64(col)*paletteCellSize
		cy := palettePadding + float64(row)*paletteCellSize

		inset := 2.0
		g.Rectangle(cx+inset, cy+inset, paletteCellSize-inset*2, paletteCellSize-inset*2)
		g.SetBrush1(c)
		g.FillPreserve()

		if i == this.hoverIdx {
			g.SetPen1(t.HighLightColor, 2)
		} else {
			g.SetPen1(paint.Color{200, 200, 200, 255}, 0.5)
		}
		g.Stroke()
	}
}

func (this *colorPalettePopup) hitTest(x, y float64) int {
	col := int((x - palettePadding) / paletteCellSize)
	row := int((y - palettePadding) / paletteCellSize)
	if col < 0 || col >= paletteCols || row < 0 || row >= paletteRows {
		return -1
	}
	idx := row*paletteCols + col
	if idx >= len(paletteColors) {
		return -1
	}
	return idx
}

func (this *colorPalettePopup) OnLeftDown(x, y float64) {
	w, h := this.Size()
	if x < 0 || y < 0 || x >= w || y >= h {
		this.PopCapture()
		this.Hide()
		emulateMouseDown(true)
		return
	}

	idx := this.hitTest(x, y)
	if idx >= 0 && idx < len(paletteColors) {
		this.owner.SetColor(paletteColors[idx])
		this.PopCapture()
		this.Hide()
	}
}

func (this *colorPalettePopup) OnMouseMove(x, y float64) {
	idx := this.hitTest(x, y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// Ensure math is used
var _ = math.Pi
