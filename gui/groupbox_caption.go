package gui

import (
	"math"
	"reflect"

	"github.com/uk0/silk/paint"
)

// One natural measurement and one assigned-width fit, not a width-indexed map.
// Cached getters never change geometry or invoke layout callbacks.
type groupBoxCaptionCache struct {
	valid, fitted   bool
	font            paint.Font
	theme           uint64
	text, family    string
	size            int
	bold, italic    bool
	ext             paint.TextExtents
	ascent, descent float64
	lineHeight      float64
	fitWidth        float64
	fitText         string
	fitExt          paint.TextExtents
}

func (this *GroupBox) captionMetrics() *groupBoxCaptionCache {
	f := Theme().Font
	c := &this.caption
	// A custom Font may be a noncomparable value. Such fonts simply miss this
	// small cache instead of panicking during interface equality.
	if c.valid && reflect.ValueOf(f).Comparable() && c.font == f &&
		c.theme == themeRev && c.text == this.title && c.family == f.Family() &&
		c.size == f.Size() && c.bold == f.Bold() && c.italic == f.Italic() {
		return c
	}
	fe, ext := f.FontExtents(), f.TextExtents(this.title)
	ascent, descent := max(0, fe.Ascent, -ext.YBearing), max(0, fe.Descent, ext.YBearing+ext.Height)
	*c = groupBoxCaptionCache{
		valid: true, font: f, theme: themeRev, text: this.title,
		family: f.Family(), size: f.Size(), bold: f.Bold(), italic: f.Italic(),
		ext: *ext, ascent: ascent, descent: descent,
		lineHeight: max(fe.Height, ascent+descent),
	}
	return c
}

func (this *GroupBox) fitCaption(width float64) (string, paint.TextExtents) {
	c := this.captionMetrics()
	if c.ext.Width <= width {
		return this.title, c.ext
	}
	if width <= 0 {
		return "", paint.TextExtents{}
	}
	if !c.fitted || c.fitWidth != width {
		c.fitText = ElideTextMiddle(c.font, this.title, width)
		c.fitExt = paint.TextExtents{}
		if c.fitText != "" {
			c.fitExt = *c.font.TextExtents(c.fitText)
		}
		c.fitWidth, c.fitted = width, true
	}
	return c.fitText, c.fitExt
}

func groupBoxCheckSize(iconSize float64) float64 {
	if iconSize <= 0 || math.IsNaN(iconSize) || math.IsInf(iconSize, 0) {
		return 0
	}
	return math.Floor(iconSize * 0.75)
}

func groupBoxBorderPath(g paint.Painter, left, top, right, bottom, gapStart, gapEnd float64, hasGap bool) {
	if right <= left || bottom <= top {
		return
	}
	r := min(4, (right-left)*0.5, (bottom-top)*0.5)
	if !hasGap {
		gapStart, gapEnd = left+r, left+r
	} else {
		gapStart = min(right-r, max(left+r, gapStart))
		gapEnd = min(right-r, max(gapStart, gapEnd))
	}
	g.MoveTo(gapStart, top)
	g.LineTo(left+r, top)
	// This outline travels counterclockwise. Arc would take the long 270deg
	// route around each corner, producing the visible inward curls.
	g.ArcNegative(left+r, top+r, r, -math.Pi/2, -math.Pi)
	g.LineTo(left, bottom-r)
	g.ArcNegative(left+r, bottom-r, r, math.Pi, math.Pi/2)
	g.LineTo(right-r, bottom)
	g.ArcNegative(right-r, bottom-r, r, math.Pi/2, 0)
	g.LineTo(right, top+r)
	g.ArcNegative(right-r, top+r, r, 0, -math.Pi/2)
	g.LineTo(gapEnd, top)
}
