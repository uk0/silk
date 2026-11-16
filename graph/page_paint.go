package graph

import (
	"math"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// drawViewPage keeps ordinary Cairo paths identical. Huge page coordinates
// overflow Cairo's fixed-point backend even with a tiny device clip; submit a
// bounded rectangle whose artificial edges lie beyond the actual clip instead.
func drawViewPage(g paint.Painter, w, h float64) {
	if math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) || w <= 0 || h <= 0 {
		return
	}
	page := geom.Rect{Width: w, Height: h}
	if w > 1<<20 || h > 1<<20 {
		clip := g.ClipBounds1()
		if clip.IsEmpty() {
			return
		}
		var m geom.Mat3x2
		g.GetMatrix(&m)
		// The page is axis-aligned in view coordinates. Keep the extra border
		// at least two device pixels outside the clip at normal UI scales.
		sx, sy := math.Hypot(m.Xx, m.Yx), math.Hypot(m.Xy, m.Yy)
		px, py := 2.0, 2.0
		if sx > 0 {
			px = math.Max(px, 2/sx)
		}
		if sy > 0 {
			py = math.Max(py, 2/sy)
		}
		left, top := math.Max(0, clip.X-px), math.Max(0, clip.Y-py)
		right, bottom := math.Min(w, clip.Right()+px), math.Min(h, clip.Bottom()+py)
		if left >= right || top >= bottom {
			return
		}
		page = geom.Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
	}
	g.Rectangle1(page)
	g.SetBrush1(paint.Color{255, 255, 255, 255})
	g.FillPreserve()
	g.SetPen1(paint.Color{0, 127, 255, 255}, 0)
	g.Stroke()
}
