package glui

import "math"

// PainterAdapter exposes a Cairo-like immediate-mode 2D drawing API on top
// of a Renderer. It is a *minimal* shim: only the operations existing
// silk/gui widget Draw() methods need to render their visuals are
// provided. It deliberately does not satisfy paint.Painter — that
// interface drags in Cairo Surface/Pattern/Glyph/Operator types and
// satisfying it would compromise glui's pure-OpenGL goal.
//
// The adapter records the most recent path (rectangle, arc, line) into a
// CPU-side path buffer and rasterises it lazily on Fill()/Stroke(). Brush
// and pen colours are tracked the same way Cairo tracks SOURCE colour.
//
// What is intentionally missing (vs paint.Painter):
//   - Paint / PaintWithAlpha (no current source surface concept)
//   - Pattern brushes, gradients, image patterns
//   - SetOperator / blend modes beyond default SRC_OVER
//   - Hardware glyph rendering (ScaledFont, DrawGlyphs)
//   - Clip-by-path (only ClipRect via Renderer.PushClip is supported)
//   - DrawPixmap / DrawIcon (deferred to a separate image API)
//
// Use this adapter from new widgets written against glui directly. To
// drive the existing 62-widget set you would need a thicker shim (see
// docs/glui-painter.md once that's written).
type PainterAdapter struct {
	r *Renderer
	f *Font

	// Brush and pen state. Mirrors cairoPainter.state.
	brush     Color
	hasBrush  bool
	pen       Color
	penWidth  float32
	hasPen    bool

	// Save/Restore stack of brush/pen state. Renderer manages the
	// matrix stack independently — Save() does both at once.
	stateStack []painterAdapterState

	// Active path: a flat list of (x, y) anchors plus sub-path starts.
	// Fill triangulates as a fan; Stroke walks the segments.
	pathPts   [][2]float32
	pathSubs  []int
	curX      float32
	curY      float32
}

type painterAdapterState struct {
	brush    Color
	hasBrush bool
	pen      Color
	penWidth float32
	hasPen   bool
}

// NewPainterAdapter builds an adapter around r. The adapter borrows r —
// the caller still owns the Renderer's lifecycle (Begin/End).
func NewPainterAdapter(r *Renderer) *PainterAdapter {
	return &PainterAdapter{
		r:        r,
		brush:    Color{0, 0, 0, 1},
		pen:      Color{0, 0, 0, 1},
		penWidth: 1,
	}
}

// SetFont selects the font used by DrawText / MeasureText.
func (p *PainterAdapter) SetFont(f *Font) { p.f = f }

// Font returns the current font.
func (p *PainterAdapter) Font() *Font { return p.f }

// Save pushes the current adapter state and the renderer transform onto
// their respective stacks. Returns the new stack depth, matching the
// Cairo Painter contract.
func (p *PainterAdapter) Save() int {
	p.stateStack = append(p.stateStack, painterAdapterState{
		brush:    p.brush,
		hasBrush: p.hasBrush,
		pen:      p.pen,
		penWidth: p.penWidth,
		hasPen:   p.hasPen,
	})
	p.r.Save()
	return len(p.stateStack)
}

// Restore pops a single Save level. Unbalanced Restore is a no-op.
func (p *PainterAdapter) Restore() int {
	n := len(p.stateStack)
	if n == 0 {
		return 0
	}
	st := p.stateStack[n-1]
	p.stateStack = p.stateStack[:n-1]
	p.brush, p.hasBrush = st.brush, st.hasBrush
	p.pen, p.penWidth, p.hasPen = st.pen, st.penWidth, st.hasPen
	p.r.Restore()
	return len(p.stateStack)
}

// RestoreTo unwinds Save/Restore down to depth n. Returns false if n is
// out of range. Mirrors Cairo Painter.RestoreTo.
func (p *PainterAdapter) RestoreTo(n int) bool {
	if n < 0 || n > len(p.stateStack) {
		return false
	}
	for len(p.stateStack) > n {
		p.Restore()
	}
	return true
}

// CurrentState returns the current Save depth.
func (p *PainterAdapter) CurrentState() int {
	return len(p.stateStack)
}

// Translate post-multiplies the renderer transform by a translation.
func (p *PainterAdapter) Translate(tx, ty float64) {
	p.r.Translate(float32(tx), float32(ty))
}

// Scale post-multiplies the renderer transform by a non-uniform scale.
func (p *PainterAdapter) Scale(sx, sy float64) {
	p.r.Scale(float32(sx), float32(sy))
}

// Rotate post-multiplies the renderer transform by a rotation.
func (p *PainterAdapter) Rotate(radians float64) {
	p.r.Rotate(float32(radians))
}

// SetBrush1 sets the fill colour.
func (p *PainterAdapter) SetBrush1(c Color) {
	p.brush = c
	p.hasBrush = true
}

// SetPen1 sets the stroke colour and width.
func (p *PainterAdapter) SetPen1(c Color, width float64) {
	p.pen = c
	p.penWidth = float32(width)
	p.hasPen = true
}

// MoveTo opens a new sub-path at (x, y).
func (p *PainterAdapter) MoveTo(x, y float64) {
	p.pathSubs = append(p.pathSubs, len(p.pathPts))
	p.pathPts = append(p.pathPts, [2]float32{float32(x), float32(y)})
	p.curX, p.curY = float32(x), float32(y)
}

// LineTo appends a straight segment to the active sub-path.
func (p *PainterAdapter) LineTo(x, y float64) {
	if len(p.pathSubs) == 0 {
		p.pathSubs = append(p.pathSubs, 0)
	}
	p.pathPts = append(p.pathPts, [2]float32{float32(x), float32(y)})
	p.curX, p.curY = float32(x), float32(y)
}

// Rectangle appends an axis-aligned rectangle as a closed sub-path.
func (p *PainterAdapter) Rectangle(x, y, w, h float64) {
	xf, yf, wf, hf := float32(x), float32(y), float32(w), float32(h)
	p.pathSubs = append(p.pathSubs, len(p.pathPts))
	p.pathPts = append(p.pathPts,
		[2]float32{xf, yf},
		[2]float32{xf + wf, yf},
		[2]float32{xf + wf, yf + hf},
		[2]float32{xf, yf + hf},
		[2]float32{xf, yf}, // close
	)
	p.curX, p.curY = xf, yf
}

// Arc appends a circular arc from angle1 to angle2 (radians, CCW).
func (p *PainterAdapter) Arc(cx, cy, radius, angle1, angle2 float64) {
	const stepsPerQuarter = 16
	span := math.Abs(angle2 - angle1)
	steps := int(span/(math.Pi/2)*stepsPerQuarter) + 1
	if steps < 4 {
		steps = 4
	}
	cxf, cyf, rf := float32(cx), float32(cy), float32(radius)
	if len(p.pathSubs) == 0 {
		p.pathSubs = append(p.pathSubs, len(p.pathPts))
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		a := angle1 + (angle2-angle1)*t
		x := cxf + rf*float32(math.Cos(a))
		y := cyf + rf*float32(math.Sin(a))
		p.pathPts = append(p.pathPts, [2]float32{x, y})
	}
	if len(p.pathPts) > 0 {
		end := p.pathPts[len(p.pathPts)-1]
		p.curX, p.curY = end[0], end[1]
	}
}

// Fill rasterises the current path with the active brush, then resets it.
func (p *PainterAdapter) Fill() {
	p.fillCurrentPath()
	p.resetPath()
}

// FillPreserve rasterises but keeps the path so it can be re-used.
func (p *PainterAdapter) FillPreserve() {
	p.fillCurrentPath()
}

// Stroke rasterises the current path's outline with the active pen, then
// resets it.
func (p *PainterAdapter) Stroke() {
	p.strokeCurrentPath()
	p.resetPath()
}

// StrokePreserve rasterises the outline but keeps the path.
func (p *PainterAdapter) StrokePreserve() {
	p.strokeCurrentPath()
}

// DrawText renders text at (0, 0) (the current origin) using the adapter's
// font. Mirrors paint.Painter.DrawText.
func (p *PainterAdapter) DrawText(text string) {
	if p.f == nil {
		return
	}
	p.r.DrawText(p.f, text, 0, 0, p.brush)
}

// DrawText1 renders text at (x, y) using the adapter's font.
func (p *PainterAdapter) DrawText1(x, y float64, text string) {
	if p.f == nil {
		return
	}
	p.r.DrawText(p.f, text, float32(x), float32(y), p.brush)
}

// fillCurrentPath triangulates each sub-path as a triangle fan and emits
// it through Renderer.FillTriangle. This is correct for convex paths
// (rectangles, arcs of <180°, regular polygons) and good enough for the
// shapes UI widgets actually draw. Concave / self-intersecting shapes
// will render incorrectly — a future revision will plug in a proper
// triangulator.
func (p *PainterAdapter) fillCurrentPath() {
	if !p.hasBrush || len(p.pathPts) < 3 {
		return
	}
	for i, start := range p.pathSubs {
		end := len(p.pathPts)
		if i+1 < len(p.pathSubs) {
			end = p.pathSubs[i+1]
		}
		if end-start < 3 {
			continue
		}
		anchor := p.pathPts[start]
		for j := start + 1; j+1 < end; j++ {
			a := p.pathPts[j]
			b := p.pathPts[j+1]
			p.r.FillTriangle(anchor[0], anchor[1], a[0], a[1], b[0], b[1], p.brush)
		}
	}
}

// strokeCurrentPath emits each segment as a thick line with the active
// pen. Joins are square (no miter / bevel) — adequate for UI lines but
// not for high-precision graphics.
func (p *PainterAdapter) strokeCurrentPath() {
	if !p.hasPen || p.penWidth <= 0 || len(p.pathPts) < 2 {
		return
	}
	w := p.penWidth
	for i, start := range p.pathSubs {
		end := len(p.pathPts)
		if i+1 < len(p.pathSubs) {
			end = p.pathSubs[i+1]
		}
		for j := start; j+1 < end; j++ {
			a := p.pathPts[j]
			b := p.pathPts[j+1]
			p.r.Line(a[0], a[1], b[0], b[1], w, p.pen)
		}
	}
}

func (p *PainterAdapter) resetPath() {
	p.pathPts = p.pathPts[:0]
	p.pathSubs = p.pathSubs[:0]
}
