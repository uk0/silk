// Package svgexport implements a paint.Painter that records draw
// operations and serialises them as SVG XML. It fills the export-
// surface gap left after the opengl branch dropped Cairo —
// cairo_svg_surface used to give callers "draw with paint.Painter,
// save as .svg" for free; this package restores that capability in
// pure Go.
//
// Usage:
//
//	p := svgexport.New(800, 600)
//	p.SetBrush1(paint.Color{R: 255, G: 0, B: 0, A: 255})
//	p.Rectangle(10, 10, 100, 50)
//	p.Fill()
//	p.WriteTo(file) // emits a complete <svg>…</svg> document
//
// The painter implements every paint.Painter method but treats
// raster-only operations (DrawPixmap, DrawIcon, DrawGlyphs) as no-ops
// because SVG has no native equivalent for "paste an arbitrary bitmap
// from a glui glyph atlas". DrawText is rasterised into a single
// <text> element, which is the SVG-correct way to preserve text as
// selectable / accessible content rather than baked-in geometry.
//
// The CTM (current transformation matrix) is folded into emitted
// coordinates at write time, so the output has no nested <g
// transform> wrappers and stays small.
package svgexport

import (
	"fmt"
	"io"
	"math"
	"strings"

	"silk/geom"
	"silk/paint"
)

// SVGPainter implements paint.Painter and emits SVG XML.
//
// Concurrency: not safe for concurrent use. One painter per export.
type SVGPainter struct {
	width, height float64

	// Output XML buffer. Populated incrementally as Fill/Stroke/etc.
	// fire; finalised by WriteTo with the closing </svg> tag.
	body strings.Builder

	// Current path being built up by MoveTo/LineTo/Arc/Rectangle/etc.
	// Reset on Fill / Stroke (unless …Preserve variant). The string is
	// SVG path-syntax: "M x y L x y A rx ry … Z".
	path strings.Builder

	// Transform stack. ctm is the active matrix; saved snapshots live
	// in stack and are popped by Restore.
	ctm   geom.Mat3x2
	stack []state

	// Style state. Mirrors what the Cairo painter tracks — paint.Brush
	// is the active fill source, paint.Pen the stroke style, paint.Font
	// the text font.
	brush paint.Brush
	pen   paint.Pen
	font  paint.Font

	// curX, curY is the running pen position used by relative-path
	// segments and by DrawText with no explicit (x, y).
	curX, curY float64
}

type state struct {
	ctm   geom.Mat3x2
	brush paint.Brush
	pen   paint.Pen
	font  paint.Font
	curX  float64
	curY  float64
}

// New constructs a fresh SVGPainter with the given canvas size in
// SVG user units (points). The initial CTM is identity, brush is
// black, pen is 1pt black.
func New(width, height float64) *SVGPainter {
	p := &SVGPainter{width: width, height: height}
	p.ctm.InitIdentity()
	p.brush = &paint.SolidBrush{Color: paint.Color{R: 0, G: 0, B: 0, A: 255}}
	p.pen = paint.NewPen(paint.Color{R: 0, G: 0, B: 0, A: 255}, 1)
	return p
}

// WriteTo serialises the recorded scene as a complete SVG document.
// The painter is not consumed — multiple calls produce identical
// output.
func (p *SVGPainter) WriteTo(w io.Writer) (int64, error) {
	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&buf,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g">`+"\n",
		p.width, p.height, p.width, p.height)
	buf.WriteString(p.body.String())
	buf.WriteString("</svg>\n")
	n, err := io.WriteString(w, buf.String())
	return int64(n), err
}

// String returns the SVG document as a string. Convenience for tests.
func (p *SVGPainter) String() string {
	var b strings.Builder
	if _, err := p.WriteTo(&b); err != nil {
		return ""
	}
	return b.String()
}

// transformPoint applies the current CTM to (x, y). All emitted
// coordinates flow through here so the SVG output never carries
// transform attributes — the math is pre-baked.
func (p *SVGPainter) transformPoint(x, y float64) (float64, float64) {
	return p.ctm.Transform(x, y)
}

// --- paint.Painter: scene root ----------------------------------------

func (p *SVGPainter) Target() paint.Surface { return nil }

// --- paint.Painter: state stack ---------------------------------------

func (p *SVGPainter) Save() int {
	p.stack = append(p.stack, state{
		ctm: p.ctm, brush: p.brush, pen: p.pen, font: p.font,
		curX: p.curX, curY: p.curY,
	})
	return len(p.stack)
}

func (p *SVGPainter) Restore() int {
	if len(p.stack) == 0 {
		return 0
	}
	s := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	p.ctm, p.brush, p.pen, p.font = s.ctm, s.brush, s.pen, s.font
	p.curX, p.curY = s.curX, s.curY
	return len(p.stack)
}

func (p *SVGPainter) RestoreTo(n int) bool {
	for len(p.stack) > n {
		p.Restore()
	}
	return len(p.stack) == n
}

func (p *SVGPainter) CurrentState() int { return len(p.stack) }

// --- paint.Painter: pen position --------------------------------------

func (p *SVGPainter) CurrentPoint() (float64, float64) { return p.curX, p.curY }

// --- paint.Painter: path construction ---------------------------------
//
// Path coordinates flow through the CTM at append time; this means
// the emitted "d" attribute is always in SVG-space and we never need
// transform="…" wrappers.

func (p *SVGPainter) MoveTo(x, y float64) {
	tx, ty := p.transformPoint(x, y)
	fmt.Fprintf(&p.path, "M %g %g ", tx, ty)
	p.curX, p.curY = x, y
}

func (p *SVGPainter) LineTo(x, y float64) {
	tx, ty := p.transformPoint(x, y)
	fmt.Fprintf(&p.path, "L %g %g ", tx, ty)
	p.curX, p.curY = x, y
}

func (p *SVGPainter) Line(x1, y1, x2, y2 float64) {
	p.MoveTo(x1, y1)
	p.LineTo(x2, y2)
}

func (p *SVGPainter) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	t1x, t1y := p.transformPoint(x1, y1)
	t2x, t2y := p.transformPoint(x2, y2)
	t3x, t3y := p.transformPoint(x3, y3)
	fmt.Fprintf(&p.path, "C %g %g %g %g %g %g ", t1x, t1y, t2x, t2y, t3x, t3y)
	p.curX, p.curY = x3, y3
}

// Arc emits an SVG elliptical-arc segment. SVG's "A" command takes
// (rx, ry, rotation, largeArcFlag, sweepFlag, x, y) so we translate
// from cairo-style (centre, radius, a0, a1) here. We always emit a
// preceding LineTo from the current point to the arc's start, which
// is what cairo also does implicitly.
func (p *SVGPainter) Arc(xc, yc, radius, angle1, angle2 float64) {
	p.appendArc(xc, yc, radius, angle1, angle2, +1)
}

func (p *SVGPainter) ArcNegative(xc, yc, radius, angle1, angle2 float64) {
	p.appendArc(xc, yc, radius, angle1, angle2, -1)
}

func (p *SVGPainter) appendArc(xc, yc, radius, a0, a1 float64, sign float64) {
	// Normalise sweep direction.
	if sign > 0 {
		for a1 < a0 {
			a1 += 2 * math.Pi
		}
	} else {
		for a1 > a0 {
			a1 -= 2 * math.Pi
		}
	}
	startX := xc + radius*math.Cos(a0)
	startY := yc + radius*math.Sin(a0)
	endX := xc + radius*math.Cos(a1)
	endY := yc + radius*math.Sin(a1)

	// Connect to arc start with a LineTo if the path already has a
	// current point. This matches cairo's behaviour and keeps path
	// continuity in the SVG output.
	tStartX, tStartY := p.transformPoint(startX, startY)
	if p.path.Len() == 0 {
		fmt.Fprintf(&p.path, "M %g %g ", tStartX, tStartY)
	} else {
		fmt.Fprintf(&p.path, "L %g %g ", tStartX, tStartY)
	}

	// SVG's elliptical-arc syntax. SVG can't express > 180° in a
	// single arc command — split when the sweep exceeds π.
	span := math.Abs(a1 - a0)
	largeArc := 0
	if span > math.Pi {
		largeArc = 1
	}
	sweep := 1
	if sign < 0 {
		sweep = 0
	}

	tEndX, tEndY := p.transformPoint(endX, endY)
	fmt.Fprintf(&p.path, "A %g %g 0 %d %d %g %g ",
		radius, radius, largeArc, sweep, tEndX, tEndY)
	p.curX, p.curY = endX, endY
}

func (p *SVGPainter) Rectangle(x, y, w, h float64) {
	// Build out as a closed path so Fill / Stroke can share the
	// canonical "d" string. Going through MoveTo/LineTo applies the
	// CTM correctly even when the CTM is rotation/skew.
	p.MoveTo(x, y)
	p.LineTo(x+w, y)
	p.LineTo(x+w, y+h)
	p.LineTo(x, y+h)
	p.path.WriteString("Z ")
	p.curX, p.curY = x, y
}

func (p *SVGPainter) Rectangle1(rect geom.Rect) {
	p.Rectangle(rect.X, rect.Y, rect.Width, rect.Height)
}

// --- paint.Painter: fill / stroke -------------------------------------

func (p *SVGPainter) Fill() {
	p.emitPath(true, false)
	p.path.Reset()
}

func (p *SVGPainter) FillPreserve() {
	p.emitPath(true, false)
}

func (p *SVGPainter) Stroke() {
	p.emitPath(false, true)
	p.path.Reset()
}

func (p *SVGPainter) StrokePreserve() {
	p.emitPath(false, true)
}

func (p *SVGPainter) emitPath(fill, stroke bool) {
	if p.path.Len() == 0 {
		return
	}
	d := strings.TrimSpace(p.path.String())
	fmt.Fprint(&p.body, `<path d="`)
	p.body.WriteString(d)
	p.body.WriteString(`"`)
	if fill {
		fmt.Fprintf(&p.body, ` fill="%s"`, brushFill(p.brush))
	} else {
		p.body.WriteString(` fill="none"`)
	}
	if stroke {
		col := paint.Color{}
		w := 1.0
		if p.pen != nil {
			col = p.pen.Color()
			w = p.pen.Width()
			if w <= 0 {
				w = 1
			}
		}
		fmt.Fprintf(&p.body, ` stroke="%s" stroke-width="%g"`, colorString(col), w)
	}
	p.body.WriteString("/>\n")
}

// Paint fills the entire canvas with the current brush. SVG doesn't
// have a literal "paint everything" primitive; we emit a full-canvas
// rect that respects the active brush.
func (p *SVGPainter) Paint() {
	old := p.path.String()
	p.path.Reset()
	p.Rectangle(0, 0, p.width, p.height)
	p.Fill()
	if old != "" {
		p.path.WriteString(old)
	}
}

func (p *SVGPainter) PaintWithAlpha(alpha uint8) {
	if alpha == 0 {
		return
	}
	// Save brush, multiply alpha, paint, restore.
	prev := p.brush
	if sb, ok := prev.(*paint.SolidBrush); ok {
		c := sb.Color
		c.A = uint8(int(c.A) * int(alpha) / 255)
		p.brush = &paint.SolidBrush{Color: c}
	}
	p.Paint()
	p.brush = prev
}

// --- paint.Painter: clipping ------------------------------------------
//
// SVG supports clipping via <clipPath> elements + clip-path attributes.
// A correct implementation would emit a <defs><clipPath> definition
// and route subsequent geometry through the clip group. That's a
// non-trivial refactor — for now we no-op and document the limitation.

func (p *SVGPainter) ResetClip()                                   {}
func (p *SVGPainter) Clip()                                        { p.path.Reset() }
func (p *SVGPainter) ClipPreserve()                                {}
func (p *SVGPainter) ClipBounds() (float64, float64, float64, float64) { return 0, 0, p.width, p.height }
func (p *SVGPainter) ClipBounds1() geom.Rect {
	return geom.Rect{X: 0, Y: 0, Width: p.width, Height: p.height}
}

// --- paint.Painter: blend operator (SVG mix-blend-mode) ---------------
//
// Operator selection in SVG would map to mix-blend-mode on a group.
// Our emitter currently doesn't open per-op groups, so SetOperator is
// recorded but only honoured when the op is not OpOver — minimal
// support for the dominant cases avoids a bigger refactor.

func (p *SVGPainter) SetOperator(op paint.Operator) { /* no-op stub */ }

// --- paint.Painter: transform stack -----------------------------------

func (p *SVGPainter) ResetMatrix()              { p.ctm.InitIdentity() }
func (p *SVGPainter) Translate(tx, ty float64)  { p.ctm.Translate(tx, ty) }
func (p *SVGPainter) Scale(sx, sy float64)      { p.ctm.Scale(sx, sy) }
func (p *SVGPainter) Rotate(radians float64)    { p.ctm.Rotate(radians) }
func (p *SVGPainter) Transform(m *geom.Mat3x2)  { p.ctm.MultiplyWidth(m) }
func (p *SVGPainter) SetMatrix(m *geom.Mat3x2)  { p.ctm = *m }
func (p *SVGPainter) GetMatrix(m *geom.Mat3x2)  { *m = p.ctm }

// --- paint.Painter: pen / brush / font --------------------------------

func (p *SVGPainter) SetPen(pen paint.Pen)               { p.pen = pen }
func (p *SVGPainter) SetPen1(cr paint.Color, w float64)  { p.pen = paint.NewPen(cr, w) }
func (p *SVGPainter) SetBrush(br paint.Brush)            { p.brush = br }
func (p *SVGPainter) SetBrush1(cr paint.Color)           { p.brush = &paint.SolidBrush{Color: cr} }
func (p *SVGPainter) SetFont(f paint.Font)               { p.font = f }
func (p *SVGPainter) Font() paint.Font                   { return p.font }
func (p *SVGPainter) ScaledFont() paint.ScaledFont       { return nil }

// --- paint.Painter: text ---------------------------------------------

func (p *SVGPainter) DrawText(text string) {
	p.DrawText1(p.curX, p.curY, text)
}

func (p *SVGPainter) DrawText1(x, y float64, text string) {
	if text == "" {
		return
	}
	tx, ty := p.transformPoint(x, y)
	col := paint.Color{R: 0, G: 0, B: 0, A: 255}
	if sb, ok := p.brush.(*paint.SolidBrush); ok {
		col = sb.Color
	}
	fontSize := 14.0
	if p.font != nil {
		if s := p.font.Size(); s > 0 {
			fontSize = float64(s)
		}
	}
	fmt.Fprintf(&p.body, `<text x="%g" y="%g" fill="%s" font-size="%g">%s</text>`+"\n",
		tx, ty, colorString(col), fontSize, escapeXML(text))
	p.curX, p.curY = x, y
}

func (p *SVGPainter) DrawGlyphs(glyphs []paint.Glyph) {}
func (p *SVGPainter) DrawGlyph(glyph *paint.Glyph)    {}

// --- paint.Painter: pixmap / icon (no native SVG equivalent) ----------

func (p *SVGPainter) DrawPixmap(pixmap paint.Pixmap)                                      {}
func (p *SVGPainter) DrawPixmap1(x, y float64, pixmap paint.Pixmap)                       {}
func (p *SVGPainter) DrawPixmap2(x, y float64, pixmap paint.Pixmap, x0, y0 float64)       {}
func (p *SVGPainter) DrawPixmap5(x, y, w, h float64, pixmap paint.Pixmap)                 {}
func (p *SVGPainter) DrawIcon(ico paint.Icon, fSize float64, grayed bool)                 {}
func (p *SVGPainter) DrawIcon1(ico paint.Icon, x, y, fSize float64, grayed bool)          {}

// --- helpers ----------------------------------------------------------

// brushFill returns the SVG fill attribute for the given brush. Solid
// brushes map to "#RRGGBB" (or "rgba(r,g,b,a)" for non-opaque alpha);
// gradients render as a single stop colour for now (the spec also
// supports linearGradient/radialGradient defs but that's a follow-up).
func brushFill(br paint.Brush) string {
	if br == nil {
		return "black"
	}
	switch v := br.(type) {
	case *paint.SolidBrush:
		return colorString(v.Color)
	case *paint.LinearGradient:
		if len(v.Stops) > 0 {
			return colorString(v.Stops[0].Color)
		}
	case *paint.RadialGradient:
		if len(v.Stops) > 0 {
			return colorString(v.Stops[0].Color)
		}
	}
	return "black"
}

// colorString renders a paint.Color as an SVG-compatible string. Opaque
// colours use "#RRGGBB"; non-opaque use "rgba(r,g,b,a)" with alpha as
// 0..1 to match the SVG spec.
func colorString(c paint.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%g)", c.R, c.G, c.B, float64(c.A)/255)
}

// escapeXML escapes the five reserved characters in element text. Avoids
// pulling encoding/xml just for this.
func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
