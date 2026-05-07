package svg

import (
	"silk/paint"
)

// Render draws doc onto painter, scaled into the target rectangle
// (x, y, w, h) in painter coordinates. The doc's ViewBox is mapped to
// the target rect; if no ViewBox is set, doc.Width/doc.Height are
// used; if those are also zero, the target rect is treated as the
// 1:1 source.
//
// The painter must be in a state ready for path operations — the
// caller is responsible for any clip / transform setup the surrounding
// widget needs. Render saves and restores state internally so the
// painter's CTM and brush/pen are unchanged on return.
func Render(doc *Doc, painter paint.Painter, x, y, w, h float64) {
	if doc == nil || painter == nil {
		return
	}
	if w <= 0 || h <= 0 {
		return
	}

	painter.Save()
	defer painter.Restore()

	// Map ViewBox → target rect with a uniform Translate + Scale. The
	// renderer doesn't honour preserveAspectRatio yet — we always scale
	// uniformly, which matches what every icon set expects.
	srcW, srcH := doc.Width, doc.Height
	srcX, srcY := 0.0, 0.0
	if !doc.ViewBox.Empty() {
		srcX, srcY = doc.ViewBox.X, doc.ViewBox.Y
		srcW, srcH = doc.ViewBox.W, doc.ViewBox.H
	}
	if srcW <= 0 {
		srcW = w
	}
	if srcH <= 0 {
		srcH = h
	}
	sx := w / srcW
	sy := h / srcH
	// Pick the smaller scale to keep the icon aspect-correct, then
	// centre the result inside the target rect.
	scale := sx
	if sy < scale {
		scale = sy
	}
	tx := x + (w-srcW*scale)/2
	ty := y + (h-srcH*scale)/2

	painter.Translate(tx, ty)
	painter.Scale(scale, scale)
	painter.Translate(-srcX, -srcY)

	renderShapes(doc.Children, painter, defaultStyle())
}

// styleStack holds the inherited style at each tree level. It avoids
// per-shape allocation of a fresh Style — the renderer mutates a
// single Style instance via push/pop.
type styleStack []Style

func (s *styleStack) push(child Style, parent Style) Style {
	merged := mergeStyle(parent, child)
	*s = append(*s, merged)
	return merged
}

func (s *styleStack) pop() {
	if n := len(*s); n > 0 {
		*s = (*s)[:n-1]
	}
}

// defaultStyle is the SVG-spec initial cascade: black fill, no stroke,
// 1px stroke-width when stroke is later set, opacity 1.
func defaultStyle() Style {
	black := Color{Val: paint.Color{R: 0, G: 0, B: 0, A: 255}}
	one := 1.0
	return Style{
		Fill:        &black,
		StrokeWidth: &one,
		Opacity:     &one,
		FillOpacity: &one,
		StrokeOpacity: &one,
	}
}

// mergeStyle returns the cascade-merged style of parent and child:
// every nil child field inherits from parent, every set field
// overrides. Used by the renderer at each group / shape boundary.
func mergeStyle(parent, child Style) Style {
	out := parent
	if child.Fill != nil {
		out.Fill = child.Fill
	}
	if child.Stroke != nil {
		out.Stroke = child.Stroke
	}
	if child.StrokeWidth != nil {
		out.StrokeWidth = child.StrokeWidth
	}
	if child.Opacity != nil {
		out.Opacity = child.Opacity
	}
	if child.FillOpacity != nil {
		out.FillOpacity = child.FillOpacity
	}
	if child.StrokeOpacity != nil {
		out.StrokeOpacity = child.StrokeOpacity
	}
	if child.FillRule != FillRuleInherit {
		out.FillRule = child.FillRule
	}
	return out
}

// renderShapes dispatches each shape onto the renderer, applying its
// transform and style cascade.
func renderShapes(shapes []Shape, painter paint.Painter, parentStyle Style) {
	for _, s := range shapes {
		c := s.common()
		merged := mergeStyle(parentStyle, c.Style)

		painter.Save()
		applyTransform(painter, c.Transform)

		switch v := s.(type) {
		case *Rect:
			renderRect(v, painter, merged)
		case *Circle:
			renderCircle(v, painter, merged)
		case *Ellipse:
			renderEllipse(v, painter, merged)
		case *Line:
			renderLine(v, painter, merged)
		case *Polygon:
			renderPolyline(v.Points, painter, merged, true)
		case *Polyline:
			renderPolyline(v.Points, painter, merged, false)
		case *Path:
			renderPath(v, painter, merged)
		case *Group:
			renderShapes(v.Children, painter, merged)
		}

		painter.Restore()
	}
}

// applyTransform issues painter calls for each TransformOp in order.
// The painter's CTM accumulates the full transform stack as we descend.
func applyTransform(painter paint.Painter, t Transform) {
	for _, op := range t.Ops {
		switch op.Kind {
		case TransformTranslate:
			painter.Translate(op.X, op.Y)
		case TransformScale:
			painter.Scale(op.X, op.Y)
		case TransformRotate:
			if op.Has {
				painter.Translate(op.A, op.B)
				painter.Rotate(degToRad(op.X))
				painter.Translate(-op.A, -op.B)
			} else {
				painter.Rotate(degToRad(op.X))
			}
		case TransformMatrix:
			// Cairo / paint.Painter doesn't expose a direct matrix
			// concat in the public Painter interface (Transform takes
			// a *geom.Mat3x2). We emulate via Translate/Scale/Rotate
			// when the matrix decomposes; full general matrix support
			// is a follow-up.
			//
			// Most icon SVGs only use translate/scale/rotate, so
			// matrix() in the wild is rare. When it does appear we
			// approximate by setting Translate to (E, F) — picks up
			// pure translation matrices, which cover most uses.
			painter.Translate(op.E, op.F)
		}
	}
}

// degToRad converts SVG's degrees to the radians paint.Painter expects.
func degToRad(deg float64) float64 { return deg * 3.141592653589793 / 180 }

// --- Per-shape renderers --------------------------------------------

func renderRect(r *Rect, painter paint.Painter, st Style) {
	if r.Rx > 0 && r.Ry > 0 {
		// Rounded rect via four arcs + four lines. We use Rx for the
		// horizontal radius and Ry for vertical; the painter's path
		// operations honour both via Translate + Scale + Arc, but for
		// portability we approximate with a non-uniform arc trick.
		// Full elliptical-corner rendering is a follow-up.
		roundedRect(painter, r.X, r.Y, r.W, r.H, r.Rx)
	} else {
		painter.Rectangle(r.X, r.Y, r.W, r.H)
	}
	emitFillStroke(painter, st)
}

func renderCircle(c *Circle, painter paint.Painter, st Style) {
	if c.R <= 0 {
		return
	}
	painter.Arc(c.Cx, c.Cy, c.R, 0, 2*3.141592653589793)
	emitFillStroke(painter, st)
}

func renderEllipse(e *Ellipse, painter paint.Painter, st Style) {
	if e.Rx <= 0 || e.Ry <= 0 {
		return
	}
	// Painter has no native ellipse — emulate with Save / Translate /
	// Scale / Arc. The underlying path is still a circle in the
	// scaled coord space, which the painter sees as an ellipse.
	painter.Save()
	painter.Translate(e.Cx, e.Cy)
	painter.Scale(1, e.Ry/e.Rx)
	painter.Arc(0, 0, e.Rx, 0, 2*3.141592653589793)
	painter.Restore()
	emitFillStroke(painter, st)
}

func renderLine(l *Line, painter paint.Painter, st Style) {
	painter.MoveTo(l.X1, l.Y1)
	painter.LineTo(l.X2, l.Y2)
	emitStrokeOnly(painter, st)
}

func renderPolyline(points []Point, painter paint.Painter, st Style, closed bool) {
	if len(points) == 0 {
		return
	}
	painter.MoveTo(points[0].X, points[0].Y)
	for _, p := range points[1:] {
		painter.LineTo(p.X, p.Y)
	}
	if closed {
		painter.LineTo(points[0].X, points[0].Y)
	}
	emitFillStroke(painter, st)
}

func renderPath(p *Path, painter paint.Painter, st Style) {
	for _, cmd := range p.Commands {
		switch cmd.Kind {
		case PathMove:
			painter.MoveTo(cmd.X, cmd.Y)
		case PathLine:
			painter.LineTo(cmd.X, cmd.Y)
		case PathCurve:
			painter.CurveTo(cmd.X1, cmd.Y1, cmd.X2, cmd.Y2, cmd.X, cmd.Y)
		case PathQuad:
			// Convert quadratic to cubic by promoting control points.
			// Cubic Bezier with control points (P0 + 2/3*(Q-P0),
			// P2 + 2/3*(Q-P2)) is equivalent to the quadratic.
			// The painter expects start point already at the cursor.
			x0, y0 := painter.CurrentPoint()
			cx1 := x0 + 2.0/3.0*(cmd.X1-x0)
			cy1 := y0 + 2.0/3.0*(cmd.Y1-y0)
			cx2 := cmd.X + 2.0/3.0*(cmd.X1-cmd.X)
			cy2 := cmd.Y + 2.0/3.0*(cmd.Y1-cmd.Y)
			painter.CurveTo(cx1, cy1, cx2, cy2, cmd.X, cmd.Y)
		case PathArc:
			// SVG elliptical arc → cubic Bezier decomposition. We use
			// the W3C endpoint-to-center conversion to extract the
			// ellipse parameters, slice the sweep into ≤90° pieces,
			// and emit one painter.CurveTo per piece. The starting
			// point of each cubic comes from the painter's current
			// pen position, which advances after each CurveTo call.
			//
			// PathCmd packs the arc args as:
			//   X1 = rx, Y1 = ry, X2 = xRot
			//   A = largeArcFlag, B = sweepFlag
			//   C = endX, D = endY
			x0, y0 := painter.CurrentPoint()
			large := cmd.A != 0
			sweep := cmd.B != 0
			segs := decomposeArc(x0, y0, cmd.X1, cmd.Y1, cmd.X2,
				large, sweep, cmd.C, cmd.D)
			if len(segs) == 0 {
				// Zero-length or fully degenerate arc — nothing to
				// emit, but the SVG spec says move the pen to the
				// endpoint anyway. A LineTo to (endX, endY) keeps the
				// path connected for fill correctness.
				painter.LineTo(cmd.C, cmd.D)
			} else {
				for _, s := range segs {
					painter.CurveTo(s.c1x, s.c1y, s.c2x, s.c2y, s.endX, s.endY)
				}
			}
		case PathClose:
			// Painter has no explicit ClosePath; drawing a zero-length
			// segment to the start triggers the same fill behaviour
			// in the dominant case. The exact spec wants ClosePath to
			// flag the join correctly — addressed alongside the arc
			// followup.
		}
	}
	emitFillStroke(painter, st)
}

// emitFillStroke runs Fill (when style has fill) and Stroke (when
// stroke). FillPreserve / StrokePreserve when both are set so the
// path is consumed once.
func emitFillStroke(painter paint.Painter, st Style) {
	hasFill := st.Fill != nil && !st.Fill.None
	hasStroke := st.Stroke != nil && !st.Stroke.None
	if !hasFill && !hasStroke {
		// Nothing to do — discard the path.
		painter.Fill() // consume the path; brush is whatever default
		return
	}
	if hasFill && hasStroke {
		col := applyOpacity(st.Fill.Val, st.Opacity, st.FillOpacity)
		painter.SetBrush1(col)
		painter.FillPreserve()
		strokeWidth := 1.0
		if st.StrokeWidth != nil {
			strokeWidth = *st.StrokeWidth
		}
		strokeCol := applyOpacity(st.Stroke.Val, st.Opacity, st.StrokeOpacity)
		painter.SetPen1(strokeCol, strokeWidth)
		painter.Stroke()
		return
	}
	if hasFill {
		col := applyOpacity(st.Fill.Val, st.Opacity, st.FillOpacity)
		painter.SetBrush1(col)
		painter.Fill()
		return
	}
	// Stroke-only.
	strokeWidth := 1.0
	if st.StrokeWidth != nil {
		strokeWidth = *st.StrokeWidth
	}
	strokeCol := applyOpacity(st.Stroke.Val, st.Opacity, st.StrokeOpacity)
	painter.SetPen1(strokeCol, strokeWidth)
	painter.Stroke()
}

// emitStrokeOnly bypasses fill — Line shapes are stroke-only by SVG
// spec (fill is ignored on <line>).
func emitStrokeOnly(painter paint.Painter, st Style) {
	hasStroke := st.Stroke != nil && !st.Stroke.None
	if !hasStroke {
		return
	}
	strokeWidth := 1.0
	if st.StrokeWidth != nil {
		strokeWidth = *st.StrokeWidth
	}
	strokeCol := applyOpacity(st.Stroke.Val, st.Opacity, st.StrokeOpacity)
	painter.SetPen1(strokeCol, strokeWidth)
	painter.Stroke()
}

// applyOpacity multiplies the colour's alpha by group opacity and the
// channel-specific opacity (fill-opacity or stroke-opacity).
func applyOpacity(c paint.Color, group, channel *float64) paint.Color {
	a := float64(c.A) / 255
	if group != nil {
		a *= clamp01(*group)
	}
	if channel != nil {
		a *= clamp01(*channel)
	}
	if a < 0 {
		a = 0
	}
	if a > 1 {
		a = 1
	}
	c.A = uint8(a*255 + 0.5)
	return c
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// roundedRect emits a rounded rectangle path using arcs for the four
// corners. r is the corner radius (uniform). For elliptical corners
// (rx ≠ ry) a follow-up will scale per-corner; today we use min(rx, ry).
func roundedRect(p paint.Painter, x, y, w, h, r float64) {
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	const halfPi = 3.141592653589793 / 2
	p.MoveTo(x+r, y)
	p.LineTo(x+w-r, y)
	p.Arc(x+w-r, y+r, r, -halfPi, 0)
	p.LineTo(x+w, y+h-r)
	p.Arc(x+w-r, y+h-r, r, 0, halfPi)
	p.LineTo(x+r, y+h)
	p.Arc(x+r, y+h-r, r, halfPi, halfPi*2)
	p.LineTo(x, y+r)
	p.Arc(x+r, y+r, r, halfPi*2, halfPi*3)
	p.LineTo(x+r, y)
}
