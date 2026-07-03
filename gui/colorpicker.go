package gui

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.ColorPicker", core.TypeOf((*ColorPicker)(nil)))
}

// ColorPicker is a color selection widget that displays the current color as
// a swatch with its hex value, followed by an inline palette of common colors
// the user can click (or arrow-key onto) to pick. The palette can be replaced
// with SetPalette to override the defaults; the active swatch (matching the
// current Color()) gets a ring highlight.
type ColorPicker struct {
	Widget
	color          paint.Color
	palette        []paint.Color
	activeIdx      int // index in palette currently focused for keyboard nav (-1 = none)
	cbColorChanged func(paint.Color)
}

// NewColorPicker creates a new ColorPicker with a default blue color and the
// built-in palette returned by defaultColorPalette().
func NewColorPicker() *ColorPicker {
	p := new(ColorPicker)
	p.Init(p)
	p.color = paint.Color{66, 133, 244, 255}
	p.palette = defaultColorPalette()
	p.activeIdx = paletteIndexOf(p.palette, p.color)
	return p
}

// Color returns the currently selected color.
func (this *ColorPicker) Color() paint.Color { return this.color }

// SetColor sets the selected color. The change callback (if any) fires only
// when the new color differs from the existing one.
func (this *ColorPicker) SetColor(c paint.Color) {
	changed := this.color != c
	this.color = c
	if changed {
		if idx := paletteIndexOf(this.palette, c); idx >= 0 {
			this.activeIdx = idx
		}
		if this.cbColorChanged != nil {
			this.cbColorChanged(c)
		}
	}
	this.Self().Update()
}

// SigColorChanged sets the callback for when the color changes.
func (this *ColorPicker) SigColorChanged(fn func(paint.Color)) {
	this.cbColorChanged = fn
}

// Palette returns the current inline palette.
func (this *ColorPicker) Palette() []paint.Color { return this.palette }

// SetPalette replaces the inline palette. A nil or empty slice restores the
// built-in default palette. The keyboard-active index is re-anchored to the
// currently selected color, or to 0 if no match exists.
func (this *ColorPicker) SetPalette(p []paint.Color) {
	if len(p) == 0 {
		this.palette = defaultColorPalette()
	} else {
		this.palette = append([]paint.Color(nil), p...)
	}
	if idx := paletteIndexOf(this.palette, this.color); idx >= 0 {
		this.activeIdx = idx
	} else if len(this.palette) > 0 {
		this.activeIdx = 0
	} else {
		this.activeIdx = -1
	}
	this.Self().Update()
}

func (this *ColorPicker) hexText() string {
	return fmt.Sprintf("#%02X%02X%02X", this.color.R, this.color.G, this.color.B)
}

// --- Palette model (pure helpers — easy to unit-test) ---

// defaultColorPalette returns a small balanced default palette: black, white,
// the primary RGB triple, the secondary CMY triple, plus a few mid-greys —
// 16 colors total. It is a function (not a package var) so tests can compare
// against a fresh copy without worrying about callers mutating shared state.
func defaultColorPalette() []paint.Color {
	return []paint.Color{
		{0, 0, 0, 255},       // black
		{255, 255, 255, 255}, // white
		{255, 0, 0, 255},     // red
		{0, 255, 0, 255},     // green
		{0, 0, 255, 255},     // blue
		{0, 255, 255, 255},   // cyan
		{255, 0, 255, 255},   // magenta
		{255, 255, 0, 255},   // yellow
		{255, 128, 0, 255},   // orange
		{128, 0, 128, 255},   // purple
		{66, 133, 244, 255},  // brand blue (default selection)
		{52, 168, 83, 255},   // brand green
		{64, 64, 64, 255},    // dark grey
		{128, 128, 128, 255}, // mid grey
		{192, 192, 192, 255}, // light grey
		{224, 224, 224, 255}, // near-white grey
	}
}

// paletteIndexOf returns the index of c in palette, or -1 if not present.
func paletteIndexOf(palette []paint.Color, c paint.Color) int {
	for i, p := range palette {
		if p == c {
			return i
		}
	}
	return -1
}

// paletteAtIndex returns palette[idx] safely; out-of-range returns the zero
// value (transparent black).
func paletteAtIndex(palette []paint.Color, idx int) paint.Color {
	if idx < 0 || idx >= len(palette) {
		return paint.Color{}
	}
	return palette[idx]
}

// paletteHitTestIndex maps a widget-local (x,y) point to a palette index
// inside the inline strip that starts at paletteX0,paletteY0 and uses
// cell-size cell for n colors. Returns -1 when the point lies outside.
func paletteHitTestIndex(x, y, paletteX0, paletteY0, cell float64, n int) int {
	if cell <= 0 || n <= 0 {
		return -1
	}
	dx := x - paletteX0
	dy := y - paletteY0
	if dx < 0 || dy < 0 || dy >= cell {
		return -1
	}
	idx := int(dx / cell)
	if idx < 0 || idx >= n {
		return -1
	}
	return idx
}

// --- Layout constants ---

const (
	colorPickerSwatchPad = 4.0
	colorPickerCellSize  = 18.0
	colorPickerCellGap   = 2.0 // visual inset around each palette cell
	colorPickerHexGap    = 6.0
)

// paletteOrigin returns the (x0, y0) of the inline palette strip and the cell
// stride (cell size on screen including its row position). The swatch on the
// left has square side = h - 2*pad and the strip starts after the hex label.
func (this *ColorPicker) paletteOrigin() (x0, y0, cell float64) {
	_, h := this.Size()
	cell = colorPickerCellSize
	if cell > h-2*colorPickerSwatchPad {
		cell = h - 2*colorPickerSwatchPad
	}
	t := Theme()
	swatchSize := h - 2*colorPickerSwatchPad
	textW := 0.0
	if t.Font != nil {
		ext := t.Font.TextExtents(this.hexText())
		textW = ext.XAdvance
	}
	x0 = colorPickerSwatchPad + swatchSize + colorPickerHexGap + textW + colorPickerHexGap
	y0 = (h - cell) * 0.5
	return
}

// --- Drawing ---

func (this *ColorPicker) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	// Background
	g.Rectangle(0, 0, w, h)
	if this.IsHover() {
		g.SetBrush1(paint.Color{235, 235, 235, 255})
	} else {
		g.SetBrush1(paint.Color{245, 245, 245, 255})
	}
	g.Fill()

	// Border
	g.Rectangle(0, 0, w, h)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Color swatch (current selection)
	swatchSize := h - 2*colorPickerSwatchPad
	swatchX := colorPickerSwatchPad
	swatchY := colorPickerSwatchPad
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
	tx := swatchX + swatchSize + colorPickerHexGap - ext.XBearing
	ty := 0.5*(h+ext.YBearing) - ext.YBearing
	g.Translate(tx, ty)
	g.DrawText(text)
	g.Translate(-tx, -ty)

	// Inline palette strip
	x0, y0, cell := this.paletteOrigin()
	for i, c := range this.palette {
		cx := x0 + float64(i)*cell
		if cx+cell > w {
			break // clip rather than overflow into right edge
		}
		inset := colorPickerCellGap
		g.Rectangle(cx+inset, y0+inset, cell-2*inset, cell-2*inset)
		g.SetBrush1(c)
		g.FillPreserve()
		// Active palette swatch (matches current color) and keyboard-focused
		// swatch both get a ring; the focused one is drawn on top with the
		// highlight pen so it stands out when the widget owns focus.
		if c == this.color {
			g.SetPen1(t.HighLightColor, 2)
		} else {
			g.SetPen1(paint.Color{180, 180, 180, 255}, 0.5)
		}
		g.Stroke()

		if this.HasFocus() && i == this.activeIdx && c != this.color {
			g.Rectangle(cx+inset, y0+inset, cell-2*inset, cell-2*inset)
			g.SetPen1(t.HighLightColor, 1)
			g.Stroke()
		}
	}
}

// --- Events ---

func (this *ColorPicker) OnMouseEnter() { this.Self().Update() }
func (this *ColorPicker) OnMouseLeave() { this.Self().Update() }

func (this *ColorPicker) OnLeftDown(x, y float64) {
	this.SetFocus()
	x0, y0, cell := this.paletteOrigin()
	if idx := paletteHitTestIndex(x, y, x0, y0, cell, len(this.palette)); idx >= 0 {
		this.activeIdx = idx
		this.SetColor(this.palette[idx])
		return
	}
	this.Self().Update()
}

func (this *ColorPicker) OnLeftUp(x, y float64) {
	this.Self().Update()
}

// OnKeyDown navigates the inline palette by arrow keys. Left/Right step by
// one swatch (single-row layout — Up/Down behave the same as Left/Right so
// keyboards without horizontal arrow muscle memory still work). Home/End
// jump to the first/last swatch. Enter/Space commit the focused swatch.
func (this *ColorPicker) OnKeyDown(key int, repeat bool) {
	if !this.IsEnabled() || len(this.palette) == 0 {
		return
	}
	if this.activeIdx < 0 {
		this.activeIdx = 0
	}
	n := len(this.palette)
	switch key {
	case KeyLeft, KeyUp:
		if this.activeIdx > 0 {
			this.activeIdx--
		}
		this.Self().Update()
	case KeyRight, KeyDown:
		if this.activeIdx < n-1 {
			this.activeIdx++
		}
		this.Self().Update()
	case KeyHome:
		this.activeIdx = 0
		this.Self().Update()
	case KeyEnd:
		this.activeIdx = n - 1
		this.Self().Update()
	case KeyEnter, KeySpace:
		this.SetColor(this.palette[this.activeIdx])
	}
}

func (this *ColorPicker) SizeHints() SizeHints {
	// Width = swatch + hex (~60px) + palette strip; keep a sensible default
	// when no font is loaded yet (tests run without a window).
	w := colorPickerSwatchPad*2 + 18 + colorPickerHexGap + 60 + colorPickerCellSize*float64(len(this.palette))
	if w < 200 {
		w = 200
	}
	return SizeHints{Width: w, Height: 26, Policy: GrowHorizontal | GrowVertical}
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
