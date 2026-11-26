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
// The document structure is the minimum PDF 1.4 viewers
// (Acrobat, macOS Preview, browser viewers) accept: %PDF-1.4 header,
// Catalog/Pages/Page/Contents/Font objects, xref table, trailer.
//
// Helvetica is the default font — one of the 14 standard PDF fonts
// every reader is required to ship, so no font embedding is needed.
// SetFont() with a paint.Font is currently advisory: the size is
// honoured but the family stays Helvetica until Type1/TrueType
// embedding lands in a follow-up.
//
// Pixmaps and icon rasters are emitted as image XObjects with a soft alpha
// mask when needed. Glyph-atlas calls are not exported.
package pdfexport

import (
	"fmt"
	"image"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

// PDFPainter implements paint.Painter and emits PDF 1.4.
//
// Concurrency: one painter per export. Not safe for concurrent use.
type PDFPainter struct {
	width, height float64
	pointsPerUnit float64

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

	// images is the per-painter image XObject pool. DrawPixmap appends
	// here on first sight; document assembly emits one PDF object per
	// entry. Names are auto-assigned ("Im1", "Im2", …) — the painter
	// doesn't deduplicate by pixmap identity, so the same icon used
	// twice produces two image objects. Real-world export sizes stay
	// modest because typical designer scenes use a small palette of
	// pixmaps; deduplication is a follow-up.
	images []imageData

	// finishedPages holds the content streams of pages that have been
	// completed via NewPage / NewPage1. The current open page's content
	// lives in p.content; finalising assembles them into a slice along
	// with each page's MediaBox dimensions.
	finishedPages []pageData

	// compress flips the document into FlateDecode-compressed
	// content-stream mode. Default off so existing tests that
	// inspect raw operators keep working; production users opt in
	// via SetCompression(true) for ~60-80% smaller PDFs.
	compress bool
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
	p := &PDFPainter{width: width, height: height, pointsPerUnit: 1}
	p.ctm.InitIdentity()
	p.brush = &paint.SolidBrush{Color: paint.Color{R: 0, G: 0, B: 0, A: 255}}
	p.pen = paint.NewPen(paint.Color{R: 0, G: 0, B: 0, A: 255}, 1)
	return p
}

// NewMillimeters records page sizes and drawing commands in millimetres.
// Serialization emits the physical page dimensions and scales the entire
// page stream once, including strokes, glyphs, images and clips.
func NewMillimeters(width, height float64) *PDFPainter {
	p := New(width, height)
	p.pointsPerUnit = 72.0 / 25.4
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

// snapshotPages assembles every finished page plus the current open
// page into a single []pageData ready for buildDocument. Used by
// WriteTo / Bytes / String — non-mutating so the painter remains
// usable for additional pages after a partial save.
func (p *PDFPainter) snapshotPages() []pageData {
	out := make([]pageData, 0, len(p.finishedPages)+1)
	out = append(out, p.finishedPages...)
	out = append(out, pageData{
		width:   p.width,
		height:  p.height,
		content: p.content.String(),
	})
	if f := p.pointsPerUnit; f != 0 && f != 1 {
		for i := range out {
			out[i].width *= f
			out[i].height *= f
			out[i].content = fmt.Sprintf("q\n%.12f 0 0 %.12f 0 0 cm\n%sQ\n", f, f, out[i].content)
		}
	}
	return out
}

// SetCompression toggles FlateDecode compression on the per-page
// content streams. Default off. With compression on, document size
// drops by 60-80% on text-heavy or graphics-heavy pages because
// repeated PDF operator names ("re", "Tj", "rg", etc.) and identical
// numeric strings collapse under zlib's LZ77 window. Image XObject
// streams already FlateDecode independently — this flag only affects
// the per-page operator streams.
//
// Toggling mid-document is allowed; the flag is read at WriteTo time,
// not at content-emission time. Existing tests inspect raw operators
// in the output so they stay compression-off; production callers can
// SetCompression(true) before WriteTo for smaller files.
func (p *PDFPainter) SetCompression(on bool) { p.compress = on }

// CompressionEnabled returns the current setting. Useful for
// production code that wants to log whether a saved PDF is in raw or
// compressed mode.
func (p *PDFPainter) CompressionEnabled() bool { return p.compress }

// WriteTo serialises the recorded pages as a complete PDF 1.4
// document. The painter is not consumed — multiple calls produce
// identical output, and additional NewPage / NewPage1 calls are
// allowed afterward.
func (p *PDFPainter) WriteTo(w io.Writer) (int64, error) {
	doc := buildDocument(p.snapshotPages(), p.images, p.compress)
	n, err := io.WriteString(w, doc)
	return int64(n), err
}

// Bytes returns the complete PDF document as a byte slice.
func (p *PDFPainter) Bytes() []byte {
	doc := buildDocument(p.snapshotPages(), p.images, p.compress)
	return []byte(doc)
}

// NewPage finalises the current page and starts a fresh page with the
// same dimensions. State (CTM / brush / pen / font / curX-Y) is reset
// to the painter's construction defaults — each page is independent,
// matching cairo_show_page's contract.
func (p *PDFPainter) NewPage() {
	p.NewPage1(p.width, p.height)
}

// NewPage1 is the explicit-size variant — useful when a document
// alternates between portrait and landscape, or carries one
// title page at a different size from the rest. Width and height are
// in the units selected by the constructor (points or millimetres).
func (p *PDFPainter) NewPage1(width, height float64) {
	// Snapshot the current page.
	p.finishedPages = append(p.finishedPages, pageData{
		width:   p.width,
		height:  p.height,
		content: p.content.String(),
	})
	// Reset for the next page. State stack is wiped (PDF q/Q is
	// per-page in the content stream, no leakage between pages).
	p.width = width
	p.height = height
	p.content.Reset()
	p.path.Reset()
	p.ctm.InitIdentity()
	p.stack = p.stack[:0]
	p.brush = &paint.SolidBrush{Color: paint.Color{R: 0, G: 0, B: 0, A: 255}}
	p.pen = paint.NewPen(paint.Color{R: 0, G: 0, B: 0, A: 255}, 1)
	p.font = nil
	p.curX, p.curY = 0, 0
}

// PageCount returns the total page count after the next WriteTo —
// finished pages plus the current open page (always at least 1).
func (p *PDFPainter) PageCount() int {
	return len(p.finishedPages) + 1
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

// writePDFOp keeps numbers in PDF decimal syntax, including the tiny
// nonzero coefficients produced by a right-angle rotation. The full
// command is buffered so invalid input cannot leave partial operands.
func writePDFOp(out *strings.Builder, op string, numbers ...float64) {
	var scratch [160]byte
	line := scratch[:0]
	for _, n := range numbers {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return
		}
		if n == 0 {
			line = append(line, '0')
		} else {
			line = strconv.AppendFloat(line, n, 'f', -1, 64)
		}
		line = append(line, ' ')
	}
	line = append(line, op...)
	line = append(line, '\n')
	out.Write(line)
}

// --- paint.Painter: path construction ---------------------------------
//
// PDF path operators take coordinates directly; we fold CTM + Y-flip
// at emit time and write the result to the path buffer. The buffer
// is only flushed to the content stream on Fill / Stroke.

func (p *PDFPainter) MoveTo(x, y float64) {
	tx, ty := p.transformPoint(x, y)
	writePDFOp(&p.path, "m", tx, ty)
	p.curX, p.curY = x, y
}

func (p *PDFPainter) LineTo(x, y float64) {
	tx, ty := p.transformPoint(x, y)
	writePDFOp(&p.path, "l", tx, ty)
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
	writePDFOp(&p.path, "c", t1x, t1y, t2x, t2y, t3x, t3y)
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
	// re starts at the bottom-left in PDF space. It remains exact for
	// axis-aligned scales, including reflections; shear/rotation need
	// the four transformed corners of the same closed path.
	if p.ctm.Xy == 0 && p.ctm.Yx == 0 {
		tx, ty := p.transformPoint(x, y+h)
		writePDFOp(&p.path, "re", tx, ty, w*p.ctm.Xx, h*p.ctm.Yy)
	} else {
		p.MoveTo(x, y+h)
		p.LineTo(x+w, y+h)
		p.LineTo(x+w, y)
		p.LineTo(x, y)
		p.path.WriteString("h\n")
	}
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
	writePDFOp(&p.content, "rg", float64(col.R)/255, float64(col.G)/255, float64(col.B)/255)
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
	writePDFOp(&p.content, "RG", float64(col.R)/255, float64(col.G)/255, float64(col.B)/255)
	writePDFOp(&p.content, "w", w)
	writePenExtensionOps(&p.content, p.pen)
	p.content.WriteString("S\n")
}

// writePenExtensionOps reads the optional DashedPen / CappedPen
// interfaces off pen and emits the matching PDF graphics-state
// operators. The operators must precede the S (stroke) so they take
// effect for that stroke. Plain pens that don't implement either
// interface produce no additional output, preserving the historical
// "solid butt-cap miter-join" defaults so existing tests pass.
func writePenExtensionOps(b *strings.Builder, pen paint.Pen) {
	if pen == nil {
		return
	}
	if dp, ok := pen.(paint.DashedPen); ok {
		dash := dp.Dash()
		if len(dash) > 0 {
			b.WriteByte('[')
			for i, v := range dash {
				if i > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
			}
			b.WriteString("] ")
			writePDFOp(b, "d", dp.DashOffset())
		}
	}
	if cp, ok := pen.(paint.CappedPen); ok {
		// PDF line cap codes: 0=butt (default), 1=round, 2=square.
		switch cp.LineCap() {
		case paint.LineCapRound:
			b.WriteString("1 J\n")
		case paint.LineCapSquare:
			b.WriteString("2 J\n")
		}
		// PDF line join codes: 0=miter (default), 1=round, 2=bevel.
		switch cp.LineJoin() {
		case paint.LineJoinRound:
			b.WriteString("1 j\n")
		case paint.LineJoinBevel:
			b.WriteString("2 j\n")
		}
		if cp.LineJoin() == paint.LineJoinMiter {
			if ml := cp.MiterLimit(); ml > 0 && ml != 10 {
				writePDFOp(b, "M", ml)
			}
		}
	}
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
// Clipping in PDF is the "W n" operator pair: W (or W* for even-odd)
// flags the current path as a clip region, n consumes the path
// without filling/stroking. The clip stays active until the next
// graphics-state restore — a Q operator that pops back to the q
// where the clip was installed.
//
// Contract: callers wanting a clip region should bracket it in
// Save/Restore. A bare Clip() with no surrounding Save means the
// clip persists for the rest of the page (which usually isn't what
// the caller wants but matches PDF semantics; we don't second-guess).
//
// ResetClip can't be expressed in PDF without leaving the current
// graphics state — it stays a no-op and the doc tells callers to use
// Save/Restore for scoped clips.

func (p *PDFPainter) ResetClip() {}

func (p *PDFPainter) Clip() {
	if p.path.Len() == 0 {
		return
	}
	p.content.WriteString(p.path.String())
	p.content.WriteString("W\nn\n")
	p.path.Reset()
}

func (p *PDFPainter) ClipPreserve() {
	if p.path.Len() == 0 {
		return
	}
	p.content.WriteString(p.path.String())
	p.content.WriteString("W\nn\n")
}

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

func (p *PDFPainter) ResetMatrix() { p.ctm.InitIdentity() }
func (p *PDFPainter) Translate(tx, ty float64) {
	var m geom.Mat3x2
	m.InitTranslate(tx, ty)
	p.Transform(&m)
}
func (p *PDFPainter) Scale(sx, sy float64) {
	var m geom.Mat3x2
	m.InitScale(sx, sy)
	p.Transform(&m)
}
func (p *PDFPainter) Rotate(radians float64) {
	var m geom.Mat3x2
	m.InitRotate(radians)
	p.Transform(&m)
}
func (p *PDFPainter) Transform(m *geom.Mat3x2) {
	// Painter transforms compose in user space, matching Cairo: CTM · m.
	// Mat3x2.Multiply takes its operands in application order (m, CTM).
	p.ctm = m.Multiply(&p.ctm)
}
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
	p.content.WriteString("/F1 ")
	writePDFOp(&p.content, "Tf", fontSize)
	writePDFOp(&p.content, "rg",
		float64(col.R)/255, float64(col.G)/255, float64(col.B)/255)
	// PDF glyphs already face up. The two basis vectors convert from
	// document Y-down to PDF Y-up without mirroring an unrotated label.
	a, b := p.ctm.Xx, -p.ctm.Yx
	c, d := -p.ctm.Xy, p.ctm.Yy
	writePDFOp(&p.content, "Tm", a, b, c, d, tx, ty)
	fmt.Fprintf(&p.content, "(%s) Tj\n", escapePDFString(text))
	p.content.WriteString("ET\n")
	p.curX, p.curY = x, y
}

func (p *PDFPainter) DrawGlyphs(glyphs []paint.Glyph) {}
func (p *PDFPainter) DrawGlyph(glyph *paint.Glyph)    {}

// --- paint.Painter: pixmap embedding ---------------------------------
//
// Pixmaps are encoded as zlib-compressed RGB byte streams and stored
// as PDF /XObject /Subtype /Image entries. Each unique pixmap goes
// into a separate object referenced from the page's Resources
// dictionary as /Im1, /Im2, etc. The content stream emits a
// "q w 0 0 h x y cm /ImN Do Q" block to draw the image at the
// requested position and full affine transform. A soft mask preserves
// transparency without compositing the image against a fixed background.

func (p *PDFPainter) DrawPixmap(pixmap paint.Pixmap) {
	if pixmap == nil {
		return
	}
	w := float64(pixmap.Width())
	h := float64(pixmap.Height())
	p.DrawPixmap5(0, 0, w, h, pixmap)
}

func (p *PDFPainter) DrawPixmap1(x, y float64, pixmap paint.Pixmap) {
	if pixmap == nil {
		return
	}
	p.DrawPixmap5(x, y, float64(pixmap.Width()), float64(pixmap.Height()), pixmap)
}

func (p *PDFPainter) DrawPixmap2(x, y float64, pixmap paint.Pixmap, x0, y0 float64) {
	p.DrawPixmap1(x+x0, y+y0, pixmap)
}

func (p *PDFPainter) DrawPixmap5(x, y, w, h float64, pixmap paint.Pixmap) {
	if pixmap == nil {
		return
	}
	t, err := p.rasterPlacement(x, y, w, h)
	if err != nil {
		return
	}
	img, err := pixmap.Image()
	if err == nil && img != nil {
		_ = p.embedRaster(img, pixmap.Width(), pixmap.Height(), t)
	}
}

// DrawRaster embeds a detached Go image. It does not retain or mutate img;
// a caller may release native drawing resources before calling it.
func (p *PDFPainter) DrawRaster(x, y, w, h float64, img image.Image) error {
	t, err := p.rasterPlacement(x, y, w, h)
	if err != nil {
		return err
	}
	if img == nil {
		return fmt.Errorf("pdf: missing raster image")
	}
	b := img.Bounds()
	return p.embedRaster(img, b.Dx(), b.Dy(), t)
}

func (p *PDFPainter) rasterPlacement(x, y, w, h float64) ([6]float64, error) {
	if !(w > 0 && h > 0) {
		return [6]float64{}, fmt.Errorf("pdf: invalid raster size %g x %g", w, h)
	}
	// PDF images originate at the bottom left with a positive UP axis.
	// Transform both basis vectors as well as the anchor.
	tx, ty := p.transformPoint(x, y+h)
	t := [6]float64{w * p.ctm.Xx, -w * p.ctm.Yx, -h * p.ctm.Xy, h * p.ctm.Yy, tx, ty}
	for _, v := range t {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return [6]float64{}, fmt.Errorf("pdf: non-finite raster transform")
		}
	}
	return t, nil
}

func (p *PDFPainter) embedRaster(img image.Image, w, h int, t [6]float64) error {
	rgb, smask, ok := encodePixmapToFlatedRGBAndSMask(img, w, h)
	if !ok {
		return fmt.Errorf("pdf: invalid or unencodable raster %d x %d", w, h)
	}
	name := fmt.Sprintf("Im%d", len(p.images)+1)
	p.images = append(p.images, imageData{
		width:      w,
		height:     h,
		compressed: rgb,
		smask:      smask,
		name:       name,
	})
	p.content.WriteString("q\n")
	writePDFOp(&p.content, "cm", t[:]...)
	fmt.Fprintf(&p.content, "/%s Do\n", name)
	p.content.WriteString("Q\n")
	return nil
}

func (p *PDFPainter) DrawIcon(ico paint.Icon, size float64, disabled bool) {
	p.DrawIcon1(ico, 0, 0, size, disabled)
}

func (p *PDFPainter) DrawIcon1(ico paint.Icon, x, y, size float64, disabled bool) {
	pix := paint.RasterizeExportIcon(ico, size, p.ctm, disabled)
	if pix != nil {
		defer paint.CloseSurface(pix)
		p.DrawPixmap5(x, y, size, size, pix)
	}
}

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
