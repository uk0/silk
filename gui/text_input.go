package gui

import (
	"math"
	"unicode"
	"unicode/utf16"

	"github.com/uk0/silk/geom"
)

// IInputMethodCursor is an optional contract for text widgets. The rectangle
// describes the primary insertion caret in widget-local coordinates. Native
// input methods use it to place preedit/candidates without covering the text.
// It does not change IWidget or require a separate native child window.
type IInputMethodCursor interface {
	InputMethodCursorRect() geom.Rect
}

// utf16TextInput buffers only an unfinished surrogate, never committed text.
// WM_CHAR delivers UTF-16 code units, not complete Unicode scalar values.
type utf16TextInput struct{ high uint16 }

func (d *utf16TextInput) reset() { d.high = 0 }

func (d *utf16TextInput) decode(unit uint16) string {
	high := d.high
	d.high = 0
	switch {
	case unit >= 0xD800 && unit <= 0xDBFF:
		d.high = unit
		return ""
	case unit >= 0xDC00 && unit <= 0xDFFF:
		if high != 0 {
			return string(utf16.DecodeRune(rune(high), rune(unit)))
		}
		return "" // orphaned input is discarded, not inserted as replacement text
	default:
		if unicode.IsControl(rune(unit)) {
			return ""
		}
		return string(rune(unit))
	}
}

// clampInputMethodRect is also safe for zero-size or not-yet-laid-out widgets.
// Keep both the top and bottom inside the widget, including tiny viewports.
func clampInputMethodRect(r geom.Rect, w, h float64) geom.Rect {
	if math.IsNaN(w) || math.IsInf(w, 0) || w <= 0 || math.IsNaN(h) || math.IsInf(h, 0) || h <= 0 {
		return geom.Rect{}
	}
	if math.IsNaN(r.X) || math.IsInf(r.X, 0) {
		r.X = 0
	}
	if math.IsNaN(r.Y) || math.IsInf(r.Y, 0) {
		r.Y = 0
	}
	if math.IsNaN(r.Height) || math.IsInf(r.Height, 0) || r.Height < 1 {
		r.Height = 1
	}
	r.Width = math.Min(2, w)
	r.Height = math.Min(r.Height, h)
	r.X = math.Max(0, math.Min(r.X, w-r.Width))
	r.Y = math.Max(0, math.Min(r.Y, h-r.Height))
	return r
}

func (e *CodeEditor) InputMethodCursorRect() geom.Rect {
	x, bottom := e.caretLocalXY()
	height := e.font.FontExtents().Height + 2
	w, h := e.Size()
	return clampInputMethodRect(geom.Rect{X: x, Y: bottom - height, Height: height}, w, h)
}

func (e *Edit) InputMethodCursorRect() geom.Rect {
	height := e.RowHeight()
	row, col := 0, 0
	if len(e.rows) > 0 {
		row, col = e.PosToRowCol(e.sel1)
	}
	x := 0.0
	if row >= 0 && row < len(e.rows) {
		glyphs := e.rows[row].glyphs
		if col >= 0 && col < len(glyphs) {
			x = glyphs[col].X
		} else if len(glyphs) > 0 {
			g := glyphs[len(glyphs)-1]
			x = g.X + float64(g.A)
		}
	}
	sx, sy := e.ScrollPos()
	w, h := e.Size()
	return clampInputMethodRect(geom.Rect{X: e.padding.L + x - sx, Y: e.padding.T + (float64(row)-sy)*height, Height: height}, w, h)
}

func (e *SearchBox) InputMethodCursorRect() geom.Rect {
	w, h := e.Size()
	height := Theme().Font.FontExtents().Height
	// Use Draw's same scroll calculation even before the next paint arrives.
	caret := searchBoxCaretX(e.text, Theme().Font, e.cursorPos)
	width := math.Max(0, w-searchBoxTextX-searchBoxClearW-2)
	scroll := math.Max(0, math.Min(e.scrollX, caret))
	if caret-scroll > width {
		scroll = caret - width
	}
	return clampInputMethodRect(geom.Rect{X: searchBoxTextX + caret - scroll, Y: (h - height) / 2, Height: height}, w, h)
}
