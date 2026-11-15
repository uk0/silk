package gui

import (
	"math"
	"slices"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

const minimapRasterMaxPixels = 1 << 20 // two ARGB surfaces: at most 8 MiB/editor

type editorMinimapRasterKey struct {
	w, h, samples                        int
	sx, sy, width, height, contentHeight float64
	offsetX, offsetY                     float64
	x, y, tx, ty                         float64
	background, border                   paint.Color
	colors                               [tokFunction + 1]paint.Color
}

// UI-thread owned, native-device-resolution pixels. The viewport indicator is
// deliberately not cached: scrolling never repaints the document overview.
// Geometry is compared exactly, without hashes or whole-document snapshots.
type editorMinimapRaster struct {
	key      editorMinimapRasterKey
	pixmap   paint.Pixmap
	painter  paint.Painter
	scratch  paint.Pixmap
	ink      paint.Painter
	segments []editorMinimapSegment // owned copy, not minimapCache's scratch slice
}

func (c *editorMinimapRaster) release() {
	paint.ClosePainter(c.painter)
	paint.ClosePainter(c.ink)
	paint.CloseSurface(c.pixmap)
	paint.CloseSurface(c.scratch)
	*c = editorMinimapRaster{}
}

// Removing/reparenting a tab must not leave its native bitmap waiting for GC.
func (e *CodeEditor) SetParent(parent IWidget) {
	if parent == nil || parent.Self() != e.Parent() {
		e.minimapRaster.release()
	}
	e.Widget.SetParent(parent)
}

// Cache only fully covered interior device pixels. Fractional outer pixels
// remain vector draws, so small fractional layout offsets do not disable the
// cache or double-antialias a translucent edge. Never resample cached pixels.
func minimapRasterKey(g paint.Painter, x, y, w, h float64, m editorMinimapLayout, pal *editorPalette) (key editorMinimapRasterKey, dx, dy float64, ok bool) {
	capable, yes := g.(interface{ SupportsRasterCache() bool })
	if !yes || !capable.SupportsRasterCache() || pal.minimapBG.A != 255 {
		return
	}
	cx, cy, cw, ch := g.ClipBounds()
	if cx > x || cy > y || cx+cw < x+w || cy+ch < y+h {
		// A partially covered background and each translucent token must
		// receive the parent's clip independently, not one combined mask.
		return
	}
	var matrix geom.Mat3x2
	g.GetMatrix(&matrix)
	if matrix.Xy != 0 || matrix.Yx != 0 || matrix.Xx < 1.0/3 || matrix.Yy <= 0 {
		return
	}
	dx, dy = x*matrix.Xx+matrix.X0, y*matrix.Yy+matrix.Y0
	right, bottom := dx+w*matrix.Xx, dy+h*matrix.Yy
	for _, n := range [...]float64{dx, dy, right, bottom} {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return key, dx, dy, false
		}
	}
	ix, iy := math.Ceil(dx), math.Ceil(dy)
	pw, ph := math.Floor(right)-ix, math.Floor(bottom)-iy
	if pw < 1 || ph < 1 || pw+4 > 32768 || ph+4 > 32768 || (pw+4)*(ph+4) > minimapRasterMaxPixels {
		return key, dx, dy, false
	}
	key = editorMinimapRasterKey{w: int(pw), h: int(ph), sx: matrix.Xx, sy: matrix.Yy, width: w, height: h,
		offsetX: (dx - ix) / matrix.Xx, offsetY: (dy - iy) / matrix.Yy,
		x: x, y: y, tx: matrix.X0 - ix, ty: matrix.Y0 - iy,
		contentHeight: m.contentHeight, samples: m.samples, background: pal.minimapBG, border: pal.minimapBorder}
	for typ := tokNormal; typ <= tokFunction; typ++ {
		key.colors[typ] = minimapSegmentColor(pal, typ)
	}
	return key, ix, iy, true
}

func (c *editorMinimapRaster) draw(g paint.Painter, x, y, w, h float64, m editorMinimapLayout, pal *editorPalette, segments []editorMinimapSegment) bool {
	key, dx, dy, ok := minimapRasterKey(g, x, y, w, h, m, pal)
	if !ok {
		c.release()
		return false
	}
	first, last := 0, m.samples-1
	full := c.pixmap == nil || c.key != key
	if full {
		c.release()
		c.key = key
		// Preserve the original fractional clip inside a two-pixel guard.
		// Cairo's masked-fill rounding differs if that clip is optimized away
		// at the surface boundary, including some fully covered interior pixels.
		c.pixmap = paint.NewPixmap(key.w+4, key.h+4)
		c.painter = c.pixmap.NewPainter()
		c.scratch = paint.NewPixmap(key.w+4, key.h+4)
		c.ink = c.scratch.NewPainter()
		matrix := geom.Mat3x2{Xx: key.sx, Yy: key.sy, X0: key.tx + 2, Y0: key.ty + 2}
		c.ink.SetMatrix(&matrix)
		c.ink.Rectangle(x, y, w, h)
		c.ink.Clip()
	} else {
		first, last = changedMinimapSamples(c.segments, segments)
	}
	if full || first <= last {
		lo, hi := 0.0, float64(key.h)
		if !full && m.samples > 0 {
			row := m.contentHeight * key.sy / float64(m.samples)
			// Integer pixel damage plus a one-pixel halo: repaint all touching
			// neighbours in their original order, preserving translucent edges.
			lo = math.Max(0, math.Floor(key.offsetY*key.sy+float64(first)*row)-1)
			hi = math.Min(float64(key.h), math.Ceil(key.offsetY*key.sy+float64(last+1)*row)+1)
		}
		// Keep the original logical coordinates and operation order. Moving
		// arithmetic into local fractional coordinates changes Cairo's fixed-
		// point edge rounding, even when the mathematical transform is equal.
		// Keep the ORIGINAL clip while drawing ink too: a narrowed dirty clip
		// can change Cairo's masked-fill rounding. Copy only the damage band
		// from reusable scratch pixels, not a differently antialiased repaint.
		drawMinimapContent(c.ink, x, y, w, h, m, pal, segments, lo/key.sy-key.offsetY, hi/key.sy-key.offsetY)
		c.scratch.Flush()
		p := c.painter
		p.Save()
		p.Rectangle(2, lo+2, float64(key.w), hi-lo)
		p.Clip()
		p.SetOperator(paint.OpSource)
		p.DrawPixmap(c.scratch)
		p.Restore()
		c.pixmap.Flush()
		c.segments = append(c.segments[:0], segments...)
	}
	g.Save()
	g.ResetMatrix()
	g.Rectangle(dx, dy, float64(key.w), float64(key.h))
	g.Clip()
	g.DrawPixmap1(dx-2, dy-2, c.pixmap) // 1:1 device pixels; no scaling/filter blur
	g.Restore()
	c.drawFringe(g, x, y, w, h, dx, dy, m, pal, segments)
	return true
}

func (c *editorMinimapRaster) drawFringe(g paint.Painter, x, y, w, h, dx, dy float64, m editorMinimapLayout, pal *editorPalette, segments []editorMinimapSegment) {
	k := c.key
	left, top := dx+k.offsetX*k.sx, dy+k.offsetY*k.sy
	right, bottom := left+w*k.sx, top+h*k.sy
	var matrix geom.Mat3x2
	g.GetMatrix(&matrix)
	strip := func(px, py, pw, ph, low, high float64, ink []editorMinimapSegment) {
		if pw <= 0 || ph <= 0 {
			return
		}
		g.Save()
		g.ResetMatrix()
		g.Rectangle(px, py, pw, ph)
		g.Clip() // integer device boundaries, no additional partial coverage
		g.SetMatrix(&matrix)
		drawMinimapContent(g, x, y, w, h, m, pal, ink, low, high)
		g.Restore()
	}
	// Token bars have a 3-logical-pixel inset. At the supported >=1/3 X
	// scale they cannot touch these at-most-one-device-pixel side strips.
	strip(math.Floor(left), math.Floor(top), dx-math.Floor(left), math.Ceil(bottom)-math.Floor(top), 0, h, nil)
	strip(dx+float64(k.w), math.Floor(top), math.Ceil(right)-dx-float64(k.w), math.Ceil(bottom)-math.Floor(top), 0, h, nil)
	strip(dx, math.Floor(top), float64(k.w), dy-math.Floor(top), 0, -k.offsetY, segments)
	strip(dx, dy+float64(k.h), float64(k.w), math.Ceil(bottom)-dy-float64(k.h), (float64(k.h)/k.sy)-k.offsetY, h, segments)
}

func changedMinimapSamples(old, next []editorMinimapSegment) (first, last int) {
	if slices.Equal(old, next) {
		return 1, 0
	}
	i := 0
	for i < len(old) && i < len(next) && old[i] == next[i] {
		i++
	}
	first = math.MaxInt
	if i < len(old) {
		first = old[i].sample
	}
	if i < len(next) {
		first = min(first, next[i].sample)
	}
	a, b := len(old), len(next)
	for a > i && b > i && old[a-1] == next[b-1] {
		a--
		b--
	}
	last = first
	if a > i {
		last = max(last, old[a-1].sample)
	}
	if b > i {
		last = max(last, next[b-1].sample)
	}
	return
}

func minimapSegmentColor(pal *editorPalette, typ tokenType) paint.Color {
	if c, ok := pal.tokens[typ]; ok && typ != tokNormal {
		return paint.Color{R: c.R, G: c.G, B: c.B, A: 200}
	}
	return pal.minimapDim
}

// Shared uncached reference and cache-damage renderer. Always retain rectangle
// order, alpha and fractional row heights; never merge touching paths by colour.
func drawMinimapContent(g paint.Painter, x, y, w, h float64, m editorMinimapLayout, pal *editorPalette, segments []editorMinimapSegment, low, high float64) {
	g.SetBrush1(pal.minimapBG)
	g.Rectangle(x, y, w, h)
	g.Fill()
	g.SetPen1(pal.minimapBorder, 1)
	g.MoveTo(x, y)
	g.LineTo(x, y+h)
	g.Stroke()
	if m.samples == 0 {
		return
	}
	rowHeight := m.contentHeight / float64(m.samples)
	mmScale := math.Max(0, w-6)
	for _, segment := range segments {
		rowY := float64(segment.sample) * rowHeight
		if rowY+rowHeight < low || rowY > high {
			continue
		}
		g.SetBrush1(minimapSegmentColor(pal, segment.typ))
		g.Rectangle(x+3+mmScale*float64(segment.col)/80, y+rowY, mmScale*float64(segment.columns)/80, rowHeight)
		g.Fill()
	}
}
