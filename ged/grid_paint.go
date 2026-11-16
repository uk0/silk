package ged

import (
	"math"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

const (
	// Bound path construction independently of authored size and inverse zoom.
	// Decimation is paint-only; model dimensions, guides and snapping never change.
	maxGridAxisLines    = 1024
	legacyGridAxisLines = 256
	minGridDeviceGap    = 2.0
	gridPaintBleed      = 0.075 // half of the existing major-tier pen width
)

type gridAxisSpan struct {
	first, pitch float64
	count        int
}

func finiteGridNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func finiteGridRect(x, y, w, h float64) bool {
	return finiteGridNumber(x) && finiteGridNumber(y) && finiteGridNumber(w) && finiteGridNumber(h) && w > 0 && h > 0 && finiteGridNumber(x+w) && finiteGridNumber(y+h)
}

// Cairo converts paths to device-space fixed-point coordinates. A finite,
// moderate authored size can still overflow after a high zoom/DPI transform.
func gridBackendSafeRect(m geom.Mat3x2, x, y, w, h float64) bool {
	for _, p := range [4][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}} {
		dx := math.FMA(m.Xx, p[0], math.FMA(m.Xy, p[1], m.X0))
		dy := math.FMA(m.Yx, p[0], math.FMA(m.Yy, p[1], m.Y0))
		if !finiteGridNumber(dx) || !finiteGridNumber(dy) || math.Abs(dx) > 1<<20 || math.Abs(dy) > 1<<20 {
			return false
		}
	}
	return true
}

// gridDisplayPitch coarsens by whole major-grid decades, keeping every visible
// line anchored to the original grid rather than to the viewport's left edge.
// The small normal viewport fast path has no logarithms or allocations.
func gridDisplayPitch(step, scale, span float64) float64 {
	return gridPitchForBudget(step, scale, span, minGridDeviceGap, maxGridAxisLines)
}

func gridPitchForBudget(step, scale, span, gap float64, budget int) float64 {
	if !finiteGridNumber(step) || !finiteGridNumber(scale) || !finiteGridNumber(span) || step <= 0 || scale <= 0 || span < 0 {
		return 0
	}
	if budget < 3 || !finiteGridNumber(gap) || gap <= 0 {
		return 0
	}
	if step*scale >= gap && span/step <= float64(budget-2) {
		return step
	}
	exponent := math.Max(0, math.Log10(gap)-math.Log10(scale)-math.Log10(step))
	if span > 0 {
		exponent = math.Max(exponent, math.Log10(span)-math.Log10(step)-math.Log10(float64(budget-2)))
	}
	exponent = math.Ceil(exponent)
	if exponent > 308 {
		return 0
	}
	pitch := step * math.Pow10(int(exponent))
	if !finiteGridNumber(pitch) || pitch <= 0 {
		return 0
	}
	return pitch
}
func makeGridAxis(origin, lo, hi, step, scale float64) gridAxisSpan {
	if !finiteGridNumber(origin) || !finiteGridNumber(lo) || !finiteGridNumber(hi) || lo > hi {
		return gridAxisSpan{}
	}
	pitch := gridDisplayPitch(step, scale, hi-lo)
	if pitch == 0 {
		return gridAxisSpan{}
	}
	index := math.Ceil((lo - origin) / pitch)
	first := math.FMA(index, pitch, origin)
	if !finiteGridNumber(first) {
		return gridAxisSpan{}
	}
	if first < lo {
		first += pitch
	}
	if first < lo || first > hi || !finiteGridNumber(first) {
		return gridAxisSpan{}
	}
	slots := math.Floor((hi - first) / pitch)
	if !finiteGridNumber(slots) || slots < 0 {
		return gridAxisSpan{}
	}
	count := maxGridAxisLines
	if slots < float64(maxGridAxisLines-1) {
		count = int(slots) + 1
	}
	return gridAxisSpan{first, pitch, count}
}
func (a gridAxisSpan) draw(g paint.Painter, vertical bool, start, end float64) {
	last := math.Inf(-1)
	for i := 0; i < a.count; i++ {
		p := math.FMA(float64(i), a.pitch, a.first)
		// Floating-point addition need not advance at very large coordinates.
		if !finiteGridNumber(p) || p <= last {
			break
		}
		if vertical {
			g.MoveTo(p, start)
			g.LineTo(p, end)
		} else {
			g.MoveTo(start, p)
			g.LineTo(end, p)
		}
		last = p
	}
}

// Cairo's rotated/sheared stroke raster can differ by a coverage bit when
// invisible subpaths are removed. Retain the exact old path for a small bounded
// tier, including its addition order, rather than weakening color parity tests.
func drawSmallGridTier(g paint.Painter, x, y, w, h, step float64) {
	for axis := 0; axis < 2; axis++ {
		p, end := x, x+w
		if axis == 1 {
			p, end = y, y+h
		}
		for i := 0; i < legacyGridAxisLines && p <= end; i++ {
			if axis == 0 {
				g.MoveTo(p, y)
				g.LineTo(p, y+h)
			} else {
				g.MoveTo(x, p)
				g.LineTo(x+w, p)
			}
			next := p + step
			if !finiteGridNumber(next) || next <= p {
				break
			}
			p = next
		}
	}
}

// Very large user coordinates overflow Cairo's fixed-point path backend even
// when the target is tiny. Clip the background path before submitting it; keep
// ordinary form paths untouched so fractional-edge antialiasing stays exact.
func paintDesignBackground(g paint.Painter, x, y, w, h float64) {
	var m geom.Mat3x2
	g.GetMatrix(&m)
	if gridBackendSafeRect(m, x, y, w, h) {
		g.Rectangle(x, y, w, h)
		g.Fill()
		return
	}
	cx, cy, cw, ch := g.ClipBounds()
	if !finiteGridRect(cx, cy, cw, ch) {
		return
	}
	sx, sy := gridNormalScales(m)
	px, py := 2.0, 2.0
	if sx > 0 && finiteGridNumber(2/sx) {
		px = 2 / sx
	}
	if sy > 0 && finiteGridNumber(2/sy) {
		py = 2 / sy
	}
	left, right := math.Max(x, cx-px), math.Min(x+w, cx+cw+px)
	top, bottom := math.Max(y, cy-py), math.Min(y+h, cy+ch+py)
	if !finiteGridRect(left, top, right-left, bottom-top) {
		return
	}
	g.Rectangle(left, top, right-left, bottom-top)
	g.Fill()
}

// Perpendicular spacing, not basis-vector length: a shear can have long basis
// vectors whose parallel grid lines remain subpixel-close. Normalizing first
// avoids overflowing a determinant for otherwise finite matrix coefficients.
func gridNormalScales(m geom.Mat3x2) (vertical, horizontal float64) {
	if !finiteGridNumber(m.Xx) || !finiteGridNumber(m.Xy) || !finiteGridNumber(m.Yx) || !finiteGridNumber(m.Yy) || !finiteGridNumber(m.X0) || !finiteGridNumber(m.Y0) {
		return 0, 0
	}
	nx, ny := math.Hypot(m.Xx, m.Yx), math.Hypot(m.Xy, m.Yy)
	if ny > 0 && finiteGridNumber(ny) {
		vertical = math.Abs(math.FMA(m.Xx, m.Yy/ny, -m.Yx*(m.Xy/ny)))
	}
	if nx > 0 && finiteGridNumber(nx) {
		horizontal = math.Abs(math.FMA(m.Xy, m.Yx/nx, -m.Yy*(m.Xx/nx)))
	}
	return
}

func drawDesignGrid(g paint.Painter, x, y, w, h float64, model GridModel) {
	minor, major, want := gridTiers(model)
	if !want || !finiteGridRect(x, y, w, h) {
		return
	}
	cx, cy, cw, ch := g.ClipBounds()
	if !finiteGridRect(cx, cy, cw, ch) {
		return
	}
	// Include pen fringes just outside the geometric clip. Cairo owns clipping
	// and antialiasing; don't snap or round authored coordinates to device pixels.
	left, right := math.Max(x, cx-gridPaintBleed), math.Min(x+w, cx+cw+gridPaintBleed)
	top, bottom := math.Max(y, cy-gridPaintBleed), math.Min(y+h, cy+ch+gridPaintBleed)
	if left > right || top > bottom {
		return
	}
	var matrix geom.Mat3x2
	g.GetMatrix(&matrix)
	sx, sy := gridNormalScales(matrix)
	safePath := gridBackendSafeRect(matrix, x, y, w, h)
	xStart, xEnd, yStart, yEnd := x, x+w, y, y+h
	// Clipping work alone is not enough: enormous segment endpoints overflow
	// Cairo's fixed-point stroker and can silently erase the visible grid.
	if !safePath && sx > 0 {
		xStart, xEnd = math.Max(x, cx-2/sx), math.Min(x+w, cx+cw+2/sx)
	}
	if !safePath && sy > 0 {
		yStart, yEnd = math.Max(y, cy-2/sy), math.Min(y+h, cy+ch+2/sy)
	}
	vx, hx := makeGridAxis(x, left, right, minor, sx), makeGridAxis(y, top, bottom, minor, sy)
	vm, hm := makeGridAxis(x, left, right, major, sx), makeGridAxis(y, top, bottom, major, sy)
	// Once both tiers converge at a dense zoom, retain only the major color.
	if vx.pitch >= vm.pitch && vm.count > 0 {
		vx.count = 0
	}
	if hx.pitch >= hm.pitch && hm.count > 0 {
		hx.count = 0
	}
	g.SetPen1(paint.Color{230, 230, 235, 220}, 0.1)
	// Keep original endpoints for exact ordinary Cairo stroke/raster semantics.
	// The number of submitted segments, unlike the original full-form loops, is
	// always bounded. Cairo clips each segment against the actual clip path.
	if safePath && vx.pitch == minor && hx.pitch == minor && w/minor <= legacyGridAxisLines-2 && h/minor <= legacyGridAxisLines-2 {
		drawSmallGridTier(g, x, y, w, h, minor)
	} else {
		vx.draw(g, true, yStart, yEnd)
		hx.draw(g, false, xStart, xEnd)
	}
	g.Stroke()
	g.SetPen1(paint.Color{200, 210, 220, 240}, 0.15)
	if safePath && vm.pitch == major && hm.pitch == major && w/major <= legacyGridAxisLines-2 && h/major <= legacyGridAxisLines-2 {
		drawSmallGridTier(g, x, y, w, h, major)
	} else {
		vm.draw(g, true, yStart, yEnd)
		hm.draw(g, false, xStart, xEnd)
	}
	g.Stroke()
}

const (
	maxDotAxis      = 64 // at most 4096 crosses / 8192 segments per paint
	minDotDeviceGap = 8.0
	designDotArm    = 0.3
)

func dotAxis(lo, hi, length, step, scale float64) gridAxisSpan {
	lo, hi = math.Max(step, lo), math.Min(math.Nextafter(length, math.Inf(-1)), hi)
	pitch := gridPitchForBudget(step, scale, hi-lo, minDotDeviceGap, maxDotAxis)
	if pitch == 0 {
		return gridAxisSpan{}
	}
	a := makeGridAxis(0, lo, hi, pitch, scale)
	a.count = min(a.count, maxDotAxis)
	return a
}

func drawDesignDots(g paint.Painter, w, h, step float64) {
	if !finiteGridRect(0, 0, w, h) || !finiteGridNumber(step) || step <= 0 {
		return
	}
	cx, cy, cw, ch := g.ClipBounds()
	if !finiteGridRect(cx, cy, cw, ch) {
		return
	}
	var m geom.Mat3x2
	g.GetMatrix(&m)
	sx, sy := gridNormalScales(m)
	// Cross arm + device-pixel pen fringe, not merely the centre's clip.
	if sx <= 0 || sy <= 0 || !finiteGridNumber(sx) || !finiteGridNumber(sy) {
		return
	}
	px, py := designDotArm+1/sx, designDotArm+1/sy
	ax := dotAxis(cx-px, cx+cw+px, w, step, sx)
	ay := dotAxis(cy-py, cy+ch+py, h, step, sy)
	g.SetPen1(paint.Color{0, 0, 0, 28}, 0)
	// Retain the old addition order and complete bounded path at ordinary
	// zoom: removing invisible subpaths can change Cairo AA coverage bits.
	if w/step <= maxDotAxis && h/step <= maxDotAxis && ax.pitch == step && ay.pitch == step {
		x := step
		for i := 0; i < maxDotAxis && x < w; i++ {
			y := step
			for j := 0; j < maxDotAxis && y < h; j++ {
				paintDesignDot(g, x, y)
				y += step
			}
			x += step
		}
	} else {
		lastX := math.Inf(-1)
		for i := 0; i < ax.count; i++ {
			x := math.FMA(float64(i), ax.pitch, ax.first)
			if !finiteGridNumber(x) || x <= lastX {
				break
			}
			lastY := math.Inf(-1)
			for j := 0; j < ay.count; j++ {
				y := math.FMA(float64(j), ay.pitch, ay.first)
				if !finiteGridNumber(y) || y <= lastY {
					break
				}
				paintDesignDot(g, x, y)
				lastY = y
			}
			lastX = x
		}
	}
	g.Stroke()
}

func paintDesignDot(g paint.Painter, x, y float64) {
	g.MoveTo(x-designDotArm, y)
	g.LineTo(x+designDotArm, y)
	g.MoveTo(x, y-designDotArm)
	g.LineTo(x, y+designDotArm)
}
