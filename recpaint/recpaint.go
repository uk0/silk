// Package recpaint implements a paint.Painter that records every
// operation into an in-memory log and can replay the log onto any
// other paint.Painter target. It fills the cairo_recording_surface
// gap left after the opengl branch dropped Cairo.
//
// Use cases:
//
//   - Scene cache: build a complex sub-scene once, replay cheaply on
//     each frame instead of rebuilding it from scratch.
//   - Multi-target export: render to screen + PDF + SVG from one
//     traversal — record once, replay against three targets.
//   - Debug / fixture capture: record a Painter session during a
//     test, replay against an instrumented Painter to inspect every
//     call ordering without modifying the producer.
//
// Implementation: each Painter method captures its arguments in a
// closure and appends to ops. Replay iterates ops and calls each
// closure with the target. Closure-per-op trades a little allocation
// for code that's a 1:1 mirror of paint.Painter — easier to keep in
// sync when the interface gains a method.
//
// Mirror state: CurrentPoint / CurrentState / GetMatrix return live
// answers based on the recorder's own CTM + state stack, so
// recording-time queries behave like a normal Painter rather than
// "undefined until replay".
package recpaint

import (
	"silk/geom"
	"silk/paint"
)

// RecordingPainter is the recorder. Construct with New(); drive it
// like any paint.Painter; call Replay to reproduce the operations on
// a real target.
//
// Concurrency: not safe for concurrent use. One recorder per session.
type RecordingPainter struct {
	ops []func(paint.Painter)

	// Mirror state — kept so CurrentPoint / CurrentState / GetMatrix
	// return non-stub answers during recording. Replay does NOT
	// consult these; the mirrors exist so the recording driver
	// (typically a widget Draw method) sees the same query behaviour
	// it would from a real Painter.
	curX, curY float64
	ctm        geom.Mat3x2
	stack      []state
	brush      paint.Brush
	pen        paint.Pen
	font       paint.Font
}

type state struct {
	ctm   geom.Mat3x2
	brush paint.Brush
	pen   paint.Pen
	font  paint.Font
	curX  float64
	curY  float64
}

// New constructs an empty RecordingPainter.
func New() *RecordingPainter {
	r := &RecordingPainter{}
	r.ctm.InitIdentity()
	return r
}

// OpCount reports how many ops have been recorded. Useful for tests
// and quick scene-complexity sniffing.
func (r *RecordingPainter) OpCount() int { return len(r.ops) }

// Reset clears the recorded log without touching mirror state. Use
// when reusing one recorder across frames where each frame's scene
// is independent — saves the New() allocation.
func (r *RecordingPainter) Reset() {
	r.ops = r.ops[:0]
}

// Replay iterates the recorded log and applies each op to target.
// The recorder is not consumed; multiple Replay calls fire identical
// op sequences. Order matters: a Save/Translate/Restore block on the
// recorder replays the same Save/Translate/Restore on target.
func (r *RecordingPainter) Replay(target paint.Painter) {
	if target == nil {
		return
	}
	for _, op := range r.ops {
		op(target)
	}
}

func (r *RecordingPainter) record(op func(paint.Painter)) {
	r.ops = append(r.ops, op)
}

// --- paint.Painter: scene root ----------------------------------------

func (r *RecordingPainter) Target() paint.Surface { return nil }

// --- paint.Painter: state stack ---------------------------------------

func (r *RecordingPainter) Save() int {
	r.stack = append(r.stack, state{
		ctm: r.ctm, brush: r.brush, pen: r.pen, font: r.font,
		curX: r.curX, curY: r.curY,
	})
	r.record(func(t paint.Painter) { t.Save() })
	return len(r.stack)
}

func (r *RecordingPainter) Restore() int {
	if len(r.stack) == 0 {
		return 0
	}
	s := r.stack[len(r.stack)-1]
	r.stack = r.stack[:len(r.stack)-1]
	r.ctm, r.brush, r.pen, r.font = s.ctm, s.brush, s.pen, s.font
	r.curX, r.curY = s.curX, s.curY
	r.record(func(t paint.Painter) { t.Restore() })
	return len(r.stack)
}

func (r *RecordingPainter) RestoreTo(n int) bool {
	for len(r.stack) > n {
		r.Restore()
	}
	return len(r.stack) == n
}

func (r *RecordingPainter) CurrentState() int { return len(r.stack) }

// --- paint.Painter: pen position --------------------------------------

func (r *RecordingPainter) CurrentPoint() (float64, float64) { return r.curX, r.curY }

// --- paint.Painter: path construction ---------------------------------

func (r *RecordingPainter) MoveTo(x, y float64) {
	r.curX, r.curY = x, y
	r.record(func(t paint.Painter) { t.MoveTo(x, y) })
}

func (r *RecordingPainter) LineTo(x, y float64) {
	r.curX, r.curY = x, y
	r.record(func(t paint.Painter) { t.LineTo(x, y) })
}

func (r *RecordingPainter) Line(x1, y1, x2, y2 float64) {
	r.curX, r.curY = x2, y2
	r.record(func(t paint.Painter) { t.Line(x1, y1, x2, y2) })
}

func (r *RecordingPainter) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	r.curX, r.curY = x3, y3
	r.record(func(t paint.Painter) { t.CurveTo(x1, y1, x2, y2, x3, y3) })
}

func (r *RecordingPainter) Arc(xc, yc, radius, angle1, angle2 float64) {
	r.record(func(t paint.Painter) { t.Arc(xc, yc, radius, angle1, angle2) })
}

func (r *RecordingPainter) ArcNegative(xc, yc, radius, angle1, angle2 float64) {
	r.record(func(t paint.Painter) { t.ArcNegative(xc, yc, radius, angle1, angle2) })
}

func (r *RecordingPainter) Rectangle(x, y, w, h float64) {
	r.curX, r.curY = x, y
	r.record(func(t paint.Painter) { t.Rectangle(x, y, w, h) })
}

func (r *RecordingPainter) Rectangle1(rect geom.Rect) {
	r.curX, r.curY = rect.X, rect.Y
	r.record(func(t paint.Painter) { t.Rectangle1(rect) })
}

// --- paint.Painter: fill / stroke -------------------------------------

func (r *RecordingPainter) Fill()           { r.record(func(t paint.Painter) { t.Fill() }) }
func (r *RecordingPainter) FillPreserve()   { r.record(func(t paint.Painter) { t.FillPreserve() }) }
func (r *RecordingPainter) Stroke()         { r.record(func(t paint.Painter) { t.Stroke() }) }
func (r *RecordingPainter) StrokePreserve() { r.record(func(t paint.Painter) { t.StrokePreserve() }) }

func (r *RecordingPainter) Paint() {
	r.record(func(t paint.Painter) { t.Paint() })
}

func (r *RecordingPainter) PaintWithAlpha(alpha uint8) {
	r.record(func(t paint.Painter) { t.PaintWithAlpha(alpha) })
}

// --- paint.Painter: clipping ------------------------------------------

func (r *RecordingPainter) ResetClip()      { r.record(func(t paint.Painter) { t.ResetClip() }) }
func (r *RecordingPainter) Clip()           { r.record(func(t paint.Painter) { t.Clip() }) }
func (r *RecordingPainter) ClipPreserve()   { r.record(func(t paint.Painter) { t.ClipPreserve() }) }
func (r *RecordingPainter) ClipBounds() (float64, float64, float64, float64) {
	return 0, 0, 0, 0
}
func (r *RecordingPainter) ClipBounds1() geom.Rect { return geom.Rect{} }

// --- paint.Painter: blend operator ------------------------------------

func (r *RecordingPainter) SetOperator(op paint.Operator) {
	r.record(func(t paint.Painter) { t.SetOperator(op) })
}

// --- paint.Painter: transform stack -----------------------------------

func (r *RecordingPainter) ResetMatrix() {
	r.ctm.InitIdentity()
	r.record(func(t paint.Painter) { t.ResetMatrix() })
}

func (r *RecordingPainter) Translate(tx, ty float64) {
	r.ctm.Translate(tx, ty)
	r.record(func(t paint.Painter) { t.Translate(tx, ty) })
}

func (r *RecordingPainter) Scale(sx, sy float64) {
	r.ctm.Scale(sx, sy)
	r.record(func(t paint.Painter) { t.Scale(sx, sy) })
}

func (r *RecordingPainter) Rotate(radians float64) {
	r.ctm.Rotate(radians)
	r.record(func(t paint.Painter) { t.Rotate(radians) })
}

func (r *RecordingPainter) Transform(m *geom.Mat3x2) {
	if m != nil {
		r.ctm.MultiplyWidth(m)
		mc := *m // capture by value so subsequent mutations don't leak
		r.record(func(t paint.Painter) { t.Transform(&mc) })
	}
}

func (r *RecordingPainter) SetMatrix(m *geom.Mat3x2) {
	if m != nil {
		r.ctm = *m
		mc := *m
		r.record(func(t paint.Painter) { t.SetMatrix(&mc) })
	}
}

func (r *RecordingPainter) GetMatrix(m *geom.Mat3x2) {
	if m != nil {
		*m = r.ctm
	}
}

// --- paint.Painter: pen / brush / font --------------------------------

func (r *RecordingPainter) SetPen(pen paint.Pen) {
	r.pen = pen
	r.record(func(t paint.Painter) { t.SetPen(pen) })
}

func (r *RecordingPainter) SetPen1(cr paint.Color, w float64) {
	r.pen = paint.NewPen(cr, w)
	r.record(func(t paint.Painter) { t.SetPen1(cr, w) })
}

func (r *RecordingPainter) SetBrush(br paint.Brush) {
	r.brush = br
	r.record(func(t paint.Painter) { t.SetBrush(br) })
}

func (r *RecordingPainter) SetBrush1(cr paint.Color) {
	r.brush = &paint.SolidBrush{Color: cr}
	r.record(func(t paint.Painter) { t.SetBrush1(cr) })
}

func (r *RecordingPainter) SetFont(f paint.Font) {
	r.font = f
	r.record(func(t paint.Painter) { t.SetFont(f) })
}

func (r *RecordingPainter) Font() paint.Font            { return r.font }
func (r *RecordingPainter) ScaledFont() paint.ScaledFont { return nil }

// --- paint.Painter: text ----------------------------------------------

func (r *RecordingPainter) DrawText(text string) {
	r.record(func(t paint.Painter) { t.DrawText(text) })
}

func (r *RecordingPainter) DrawText1(x, y float64, text string) {
	r.curX, r.curY = x, y
	r.record(func(t paint.Painter) { t.DrawText1(x, y, text) })
}

func (r *RecordingPainter) DrawGlyphs(glyphs []paint.Glyph) {
	g := append([]paint.Glyph(nil), glyphs...) // copy so mutations don't leak
	r.record(func(t paint.Painter) { t.DrawGlyphs(g) })
}

func (r *RecordingPainter) DrawGlyph(glyph *paint.Glyph) {
	if glyph == nil {
		return
	}
	g := *glyph
	r.record(func(t paint.Painter) { t.DrawGlyph(&g) })
}

// --- paint.Painter: pixmap / icon -------------------------------------

func (r *RecordingPainter) DrawPixmap(pixmap paint.Pixmap) {
	r.record(func(t paint.Painter) { t.DrawPixmap(pixmap) })
}

func (r *RecordingPainter) DrawPixmap1(x, y float64, pixmap paint.Pixmap) {
	r.record(func(t paint.Painter) { t.DrawPixmap1(x, y, pixmap) })
}

func (r *RecordingPainter) DrawPixmap2(x, y float64, pixmap paint.Pixmap, x0, y0 float64) {
	r.record(func(t paint.Painter) { t.DrawPixmap2(x, y, pixmap, x0, y0) })
}

func (r *RecordingPainter) DrawPixmap5(x, y, w, h float64, pixmap paint.Pixmap) {
	r.record(func(t paint.Painter) { t.DrawPixmap5(x, y, w, h, pixmap) })
}

func (r *RecordingPainter) DrawIcon(ico paint.Icon, fSize float64, grayed bool) {
	r.record(func(t paint.Painter) { t.DrawIcon(ico, fSize, grayed) })
}

func (r *RecordingPainter) DrawIcon1(ico paint.Icon, x, y, fSize float64, grayed bool) {
	r.record(func(t paint.Painter) { t.DrawIcon1(ico, x, y, fSize, grayed) })
}
