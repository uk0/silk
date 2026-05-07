// Package pdfexport implements a paint.Painter that records draw
// operations and serialises them as a PDF 1.4 document. Sister of
// silk/svgexport — together they restore the export-surface
// capability lost when the opengl branch dropped Cairo
// (cairo_pdf_surface).
//
// Usage:
//
//	p := pdfexport.New(595, 842) // A4 in points (72 dpi)
//	p.SetBrush1(paint.Color{R: 200, G: 200, B: 200, A: 255})
//	p.Rectangle(0, 0, 595, 842)
//	p.Fill()
//	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
//	p.DrawText1(72, 80, "Hello PDF")
//	p.WriteTo(file)
//
// PDF coordinate convention is bottom-left origin with Y up; paint's
// is top-left, Y down. We flip Y per coordinate at emit time so the
// resulting PDF reads pixels with the same (x, y) the caller passed.
//
// Single-page output. The doc structure is the minimum PDF 1.4 viewers
// (Acrobat, macOS Preview, browser viewers) accept: %PDF-1.4 header,
// Catalog/Pages/Page/Contents/Font objects, xref table, trailer.
//
// Helvetica is the default font — one of the 14 standard PDF fonts
// every reader is required to ship, so no font embedding is needed.
// SetFont() with a paint.Font is currently advisory: the size is
// honoured but the family stays Helvetica until Type1/TrueType
// embedding lands in a follow-up.
//
// Pixmap / Icon / Glyphs operations are recorded as no-ops (the same
// limitation as svgexport); raster image embedding via the /XObject
// dictionary is a follow-up.
package pdfexport

import (
	"fmt"
	"io"
	"math"
	"strings"

	"silk/geom"
	"silk/paint"
)

// PDFPainter implements paint.Painter and emits PDF 1.4.
//
// Concurrency: one painter per export. Not safe for concurrent use.
type PDFPainter struct {
	width, height float64

	// content is the active page's content stream — PDF drawing
	// commands accumulated as Fill / Stroke / DrawText / etc. fire.
	content strings.Builder

	// path holds the current open path's PDF commands. Reset on Fill /
	// Stroke unless the *Preserve variant is used.
	path strings.Builder

	// Transform stack. ctm is the current matrix; saved snapshots are
	// in stack and popped by Restore. We DO emit the q/Q PDF save/
	// restore operators on each Save/Restore so transforms set via
	// the cm operator are properly nested at viewer time — but our
	// own coordinate-emission folds the CTM in directly, mirroring
	// svgexport. That redundancy means Save/Restore at viewer level
	// only matters for graphics-state attributes the painter API
	// doesn't directly track (e.g. dash pattern carried by stroke
	// commands when extension pens land).
	ctm   geom.Mat3x2
	stack []state

	brush paint.Brush
	pen   paint.Pen
	font  paint.Font

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

// New constructs a fresh painter with the given page size in points
// (1/72 inch). A4 ≈ (595, 842); US Letter ≈ (612, 792); designer
// canvases typically pass their own sizes.
func New(width, height float64) *PDFPainter {
	p := &PDFPainter{width: width, height: height}
	p.ctm.InitIdentity()
	p.brush = &paint.SolidBrush{Color: paint.Color{R: 0, G: 0, B: 0, A: 255}}
	p.pen = paint.NewPen(paint.Color{R: 0, G: 0, B: 0, A: 255}, 1)
	return p
}

// transformY flips a top-down y to PDF's bottom-up y. Paired with
// transformPoint to map paint.Painter coords to PDF coords.
func (p *PDFPainter) transformY(y float64) float64 { return p.height - y }

// transformPoint applies CTM then Y-flips to PDF user space.
func (p *PDFPainter) transformPoint(x, y float64) (float64, float64) {
	tx, ty := p.ctm.Transform(x, y)
	return tx, p.transformY(ty)
}

// WriteTo serialises the recorded page as a complete PDF 1.4 document.
// The painter is not consumed — multiple calls produce identical
// output.
func (p *PDFPainter) WriteTo(w io.Writer) (int64, error) {
	doc := buildDocument(p.width, p.height, p.content.String())
	n, err := io.WriteString(w, doc)
	return int64(n), err
}

// Bytes returns the complete PDF document as a byte slice.
func (p *PDFPainter) Bytes() []byte {
	doc := buildDocument(p.width, p.height, p.content.String())
	return []byte(doc)
}

// String returns the document as a string. Convenient for tests; PDF
// content is mostly ASCII so this is safe for inspection (the binary
// marker after the header is the only non-ASCII byte sequence).
func (p *PDFPainter) String() string {
	return string(p.Bytes())
}

// --- paint.Painter: scene root ----------------------------------------

func (p *PDFPainter) Target() paint.Surface { return nil }

// --- paint.Painter: state stack ---------------------------------------

func (p *PDFPainter) Save() int {
	p.stack = append(p.stack, state{
		ctm: p.ctm, brush: p.brush, pen: p.pen, font: p.font,
		curX: p.curX, curY: p.curY,
	})
	p.content.WriteString("q\n")
	return len(p.stack)
}

func (p *PDFPainter) Restore() int {
	if len(p.stack) == 0 {
		return 0
	}
	s := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	p.ctm, p.brush, p.pen, p.font = s.ctm, s.brush, s.pen, s.font
	p.curX, p.curY = s.curX, s.curY
	p.content.WriteString("Q\n")
	return len(p.stack)
}

func (p *PDFPainter) RestoreTo(n int) bool {
	for len(p.stack) > n {
		p.Restore()
	}
	return len(p.stack) == n
}

func (p *PDFPainter) CurrentState() int { return len(p.stack) }

// --- paint.Painter: pen position --------------------------------------

func (p *PDFPainter) CurrentPoint() (float64, float64) { return p.curX, p.curY }

// --- paint.Painter: path construction ---------------------------------
//
// PDF path operators take coordinates directly; we fold CTM + Y-flip
// at emit time and write the result to the path buffer. The buffer
// is only flushed to the content stream on Fill / Stroke.

func (p *PDFPainter) MoveTo(x, y float64) {
	tx, ty := p.transformPoint(x, y)
	fmt.Fprintf(&p.path, "%g %g m\n", tx, ty)
	p.curX, p.curY = x, y
}

func (p *PDFPainter) LineTo(x, y float64) {
	tx, ty := p.transformPoint(x, y)
	fmt.Fprintf(&p.path, "%g %g l\n", tx, ty)
	p.curX, p.curY = x, y
}

func (p *PDFPainter) Line(x1, y1, x2, y2 float64) {
	p.MoveTo(x1, y1)
	p.LineTo(x2, y2)
}

func (p *PDFPainter) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	t1x, t1y := p.transformPoint(x1, y1)
	t2x, t2y := p.transformPoint(x2, y2)
	t3x, t3y := p.transformPoint(x3, y3)
	fmt.Fprintf(&p.path, "%g %g %g %g %g %g c\n", t1x, t1y, t2x, t2y, t3x, t3y)
	p.curX, p.curY = x3, y3
}

// Arc decomposes into ≤90° cubic Bezier slices. PDF has no native
// elliptical-arc operator; the standard practice is the same as SVG's
// "A" expansion — split into quarter-or-less spans and approximate
// each with one cubic.
func (p *PDFPainter) Arc(xc, yc, radius, angle1, angle2 float64) {
	p.appendArc(xc, yc, radius, angle1, angle2, +1)
}

func (p *PDFPainter) ArcNegative(xc, yc, radius, angle1, angle2 float64) {
	p.appendArc(xc, yc, radius, angle1, angle2, -1)
}

func (p *PDFPainter) appendArc(xc, yc, radius, a0, a1 float64, sign float64) {
	if sign > 0 {
		for a1 < a0 {
			a1 += 2 * math.Pi
		}
	} else {
		for a1 > a0 {
			a1 -= 2 * math.Pi
		}
	}
	span := math.Abs(a1 - a0)
	steps := int(math.Ceil(span / (math.Pi * 0.5)))
	if steps == 0 {
		steps = 1
	}
	dt := (a1 - a0) / float64(steps)
	startX := xc + radius*math.Cos(a0)
	startY := yc + radius*math.Sin(a0)
	if p.path.Len() == 0 {
		p.MoveTo(startX, startY)
	} else {
		p.LineTo(startX, startY)
	}
	curA := a0
	for i := 0; i < steps; i++ {
		nextA := curA + dt
		// Standard cubic-Bezier approximation of an ellipse arc:
		// k = (4/3) * tan(dt/4).
		t := math.Tan(dt * 0.25)
		alpha := math.Sin(dt) * (math.Sqrt(4+3*t*t) - 1) / 3
		c1 := curA
		c2 := nextA
		s1, k1 := math.Sin(c1), math.Cos(c1)
		s2, k2 := math.Sin(c2), math.Cos(c2)
		x1 := xc + radius*(k1-alpha*s1)
		y1 := yc + radius*(s1+alpha*k1)
		x2 := xc + radius*(k2+alpha*s2)
		y2 := yc + radius*(s2-alpha*k2)
		x3 := xc + radius*k2
		y3 := yc + radius*s2
		p.CurveTo(x1, y1, x2, y2, x3, y3)
		curA = nextA
	}
}

func (p *PDFPainter) Rectangle(x, y, w, h float64) {
	// PDF has a native "re" rectangle operator: x y w h re. It expects
	// the BOTTOM-left corner. After Y-flip our top-left (x, y) becomes
	// PDF (x, H - y - h). w and h pass through unchanged.
	tx, ty := p.transformPoint(x, y)
	// Y has been flipped; the rect's bottom-left in PDF coords is at
	// y = transformY(y+h) = H - (y+h) = (H - y) - h = ty - h.
	bottomY := ty - h
	fmt.Fprintf(&p.path, "%g %g %g %g re\n", tx, bottomY, w, h)
	p.curX, p.curY = x, y
}

func (p *PDFPainter) Rectangle1(rect geom.Rect) {
	p.Rectangle(rect.X, rect.Y, rect.Width, rect.Height)
}

// --- paint.Painter: fill / stroke -------------------------------------

func (p *PDFPainter) Fill() {
	p.emitFill(false)
	p.path.Reset()
}

func (p *PDFPainter) FillPreserve() {
	p.emitFill(false)
}

func (p *PDFPainter) Stroke() {
	p.emitStroke()
	p.path.Reset()
}

func (p *PDFPainter) StrokePreserve() {
	p.emitStroke()
}

// emitFill writes the active brush colour as a non-stroking RGB and
// emits the path + 'f' (nonzero fill) operator.
func (p *PDFPainter) emitFill(stroke bool) {
	if p.path.Len() == 0 {
		return
	}
	col := brushColor(p.brush)
	p.content.WriteString(p.path.String())
	fmt.Fprintf(&p.content, "%g %g %g rg\n", float64(col.R)/255, float64(col.G)/255, float64(col.B)/255)
	if stroke {
		p.content.WriteString("B\n")
	} else {
		p.content.WriteString("f\n")
	}
}

func (p *PDFPainter) emitStroke() {
	if p.path.Len() == 0 {
		return
	}
	col := paint.Color{}
	w := 1.0
	if p.pen != nil {
		col = p.pen.Color()
		w = p.pen.Width()
		if w <= 0 {
			w = 1
		}
	}
	p.content.WriteString(p.path.String())
	fmt.Fprintf(&p.content, "%g %g %g RG\n", float64(col.R)/255, float64(col.G)/255, float64(col.B)/255)
	fmt.Fprintf(&p.content, "%g w\n", w)
	p.content.WriteString("S\n")
}

// Paint fills the entire page with the active brush.
func (p *PDFPainter) Paint() {
	old := p.path.String()
	p.path.Reset()
	p.Rectangle(0, 0, p.width, p.height)
	p.Fill()
	if old != "" {
		p.path.WriteString(old)
	}
}

func (p *PDFPainter) PaintWithAlpha(alpha uint8) {
	if alpha == 0 {
		return
	}
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
// PDF supports clipping via the W (winding) / W* (even-odd) operators
// followed by n (no-op fill). A correct integration would emit a
// W;n sequence after path construction. For now Clip is a no-op like
// svgexport — designer/chart use cases rarely depend on clip. Tracked
// in roadmap as follow-up.

func (p *PDFPainter) ResetClip()    {}
func (p *PDFPainter) Clip()         { p.path.Reset() }
func (p *PDFPainter) ClipPreserve() {}
func (p *PDFPainter) ClipBounds() (float64, float64, float64, float64) {
	return 0, 0, p.width, p.height
}
func (p *PDFPainter) ClipBounds1() geom.Rect {
	return geom.Rect{X: 0, Y: 0, Width: p.width, Height: p.height}
}

// --- paint.Painter: blend operator ------------------------------------
//
// PDF graphics-state ExtGState dictionaries can carry blend modes —
// out of scope for this round.

func (p *PDFPainter) SetOperator(op paint.Operator) {}

// --- paint.Painter: transform stack -----------------------------------

func (p *PDFPainter) ResetMatrix()             { p.ctm.InitIdentity() }
func (p *PDFPainter) Translate(tx, ty float64) { p.ctm.Translate(tx, ty) }
func (p *PDFPainter) Scale(sx, sy float64)     { p.ctm.Scale(sx, sy) }
func (p *PDFPainter) Rotate(radians float64)   { p.ctm.Rotate(radians) }
func (p *PDFPainter) Transform(m *geom.Mat3x2) { p.ctm.MultiplyWidth(m) }
func (p *PDFPainter) SetMatrix(m *geom.Mat3x2) { p.ctm = *m }
func (p *PDFPainter) GetMatrix(m *geom.Mat3x2) { *m = p.ctm }

// --- paint.Painter: pen / brush / font --------------------------------

func (p *PDFPainter) SetPen(pen paint.Pen)              { p.pen = pen }
func (p *PDFPainter) SetPen1(cr paint.Color, w float64) { p.pen = paint.NewPen(cr, w) }
func (p *PDFPainter) SetBrush(br paint.Brush)           { p.brush = br }
func (p *PDFPainter) SetBrush1(cr paint.Color)          { p.brush = &paint.SolidBrush{Color: cr} }
func (p *PDFPainter) SetFont(f paint.Font)              { p.font = f }
func (p *PDFPainter) Font() paint.Font                  { return p.font }
func (p *PDFPainter) ScaledFont() paint.ScaledFont      { return nil }

// --- paint.Painter: text ---------------------------------------------

func (p *PDFPainter) DrawText(text string) {
	p.DrawText1(p.curX, p.curY, text)
}

// DrawText1 emits a PDF text object (BT…ET) at (x, y). The text-matrix
// trick: PDF text is normally drawn glyph-up, but our caller's y is
// top-down; we use Tm = [1 0 0 -1 x H-y] which places origin at PDF
// (x, H-y) and flips glyph orientation back to upright for top-down
// users.
func (p *PDFPainter) DrawText1(x, y float64, text string) {
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
	p.content.WriteString("BT\n")
	fmt.Fprintf(&p.content, "/F1 %g Tf\n", fontSize)
	fmt.Fprintf(&p.content, "%g %g %g rg\n",
		float64(col.R)/255, float64(col.G)/255, float64(col.B)/255)
	fmt.Fprintf(&p.content, "1 0 0 -1 %g %g Tm\n", tx, ty)
	fmt.Fprintf(&p.content, "(%s) Tj\n", escapePDFString(text))
	p.content.WriteString("ET\n")
	p.curX, p.curY = x, y
}

func (p *PDFPainter) DrawGlyphs(glyphs []paint.Glyph) {}
func (p *PDFPainter) DrawGlyph(glyph *paint.Glyph)    {}

// --- paint.Painter: pixmap / icon -------------------------------------

func (p *PDFPainter) DrawPixmap(pixmap paint.Pixmap)                                {}
func (p *PDFPainter) DrawPixmap1(x, y float64, pixmap paint.Pixmap)                 {}
func (p *PDFPainter) DrawPixmap2(x, y float64, pixmap paint.Pixmap, x0, y0 float64) {}
func (p *PDFPainter) DrawPixmap5(x, y, w, h float64, pixmap paint.Pixmap)           {}
func (p *PDFPainter) DrawIcon(ico paint.Icon, fSize float64, grayed bool)           {}
func (p *PDFPainter) DrawIcon1(ico paint.Icon, x, y, fSize float64, grayed bool)    {}

// --- helpers ----------------------------------------------------------

// brushColor returns a Color usable for "rg" non-stroking colour. Solid
// brushes pass through; gradients use the first stop colour as a fallback
// (full gradient support requires PDF Pattern/Shading dictionaries — a
// follow-up).
func brushColor(br paint.Brush) paint.Color {
	if sb, ok := br.(*paint.SolidBrush); ok {
		return sb.Color
	}
	if g, ok := br.(*paint.LinearGradient); ok && len(g.Stops) > 0 {
		return g.Stops[0].Color
	}
	if g, ok := br.(*paint.RadialGradient); ok && len(g.Stops) > 0 {
		return g.Stops[0].Color
	}
	return paint.Color{R: 0, G: 0, B: 0, A: 255}
}

// escapePDFString escapes the three special characters in PDF literal
// strings — "(", ")", and "\" — by prefixing each with a backslash.
// Non-ASCII bytes pass through; PDF's WinAnsiEncoding handles Latin-1
// transparently for Helvetica.
func escapePDFString(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		"(", `\(`,
		")", `\)`,
	)
	return r.Replace(s)
}
