package paint

import (
	"reflect"
	"sort"
	"testing"

	"github.com/uk0/silk/geom"
)

// coveragePainter is a minimal Painter that records the names of every
// method invoked on it. The conformance-coverage test uses this to
// reflect on the Painter interface and verify that RunPainterBattery
// actually exercises every declared method — a regression net for
// "added a Painter method but forgot to extend the battery".
type coveragePainter struct {
	called map[string]bool
}

func newCoveragePainter() *coveragePainter {
	return &coveragePainter{called: map[string]bool{}}
}

func (c *coveragePainter) note(name string) { c.called[name] = true }

func (c *coveragePainter) Target() Surface                                 { c.note("Target"); return nil }
func (c *coveragePainter) Save() int                                       { c.note("Save"); return 0 }
func (c *coveragePainter) Restore() int                                    { c.note("Restore"); return 0 }
func (c *coveragePainter) RestoreTo(int) bool                              { c.note("RestoreTo"); return true }
func (c *coveragePainter) CurrentState() int                               { c.note("CurrentState"); return 0 }
func (c *coveragePainter) CurrentPoint() (float64, float64)                { c.note("CurrentPoint"); return 0, 0 }
func (c *coveragePainter) Arc(float64, float64, float64, float64, float64) { c.note("Arc") }
func (c *coveragePainter) ArcNegative(float64, float64, float64, float64, float64) {
	c.note("ArcNegative")
}
func (c *coveragePainter) CurveTo(float64, float64, float64, float64, float64, float64) {
	c.note("CurveTo")
}
func (c *coveragePainter) Line(float64, float64, float64, float64) { c.note("Line") }
func (c *coveragePainter) LineTo(float64, float64)                 { c.note("LineTo") }
func (c *coveragePainter) MoveTo(float64, float64)                 { c.note("MoveTo") }
func (c *coveragePainter) Rectangle(float64, float64, float64, float64) {
	c.note("Rectangle")
}
func (c *coveragePainter) Rectangle1(geom.Rect) { c.note("Rectangle1") }
func (c *coveragePainter) Stroke()              { c.note("Stroke") }
func (c *coveragePainter) StrokePreserve()      { c.note("StrokePreserve") }
func (c *coveragePainter) Fill()                { c.note("Fill") }
func (c *coveragePainter) FillPreserve()        { c.note("FillPreserve") }
func (c *coveragePainter) Paint()               { c.note("Paint") }
func (c *coveragePainter) PaintWithAlpha(uint8) { c.note("PaintWithAlpha") }
func (c *coveragePainter) ResetClip()           { c.note("ResetClip") }
func (c *coveragePainter) Clip()                { c.note("Clip") }
func (c *coveragePainter) ClipPreserve()        { c.note("ClipPreserve") }
func (c *coveragePainter) ClipBounds() (float64, float64, float64, float64) {
	c.note("ClipBounds")
	return 0, 0, 0, 0
}
func (c *coveragePainter) ClipBounds1() geom.Rect     { c.note("ClipBounds1"); return geom.Rect{} }
func (c *coveragePainter) SetOperator(Operator)       { c.note("SetOperator") }
func (c *coveragePainter) ResetMatrix()               { c.note("ResetMatrix") }
func (c *coveragePainter) Translate(float64, float64) { c.note("Translate") }
func (c *coveragePainter) Scale(float64, float64)     { c.note("Scale") }
func (c *coveragePainter) Rotate(float64)             { c.note("Rotate") }
func (c *coveragePainter) Transform(*geom.Mat3x2)     { c.note("Transform") }
func (c *coveragePainter) SetMatrix(*geom.Mat3x2)     { c.note("SetMatrix") }
func (c *coveragePainter) GetMatrix(*geom.Mat3x2)     { c.note("GetMatrix") }
func (c *coveragePainter) SetPen(Pen)                 { c.note("SetPen") }
func (c *coveragePainter) SetPen1(Color, float64)     { c.note("SetPen1") }
func (c *coveragePainter) SetBrush(Brush)             { c.note("SetBrush") }
func (c *coveragePainter) SetBrush1(Color)            { c.note("SetBrush1") }
func (c *coveragePainter) SetFont(Font)               { c.note("SetFont") }
func (c *coveragePainter) Font() Font                 { c.note("Font"); return nil }
func (c *coveragePainter) ScaledFont() ScaledFont     { c.note("ScaledFont"); return nil }
func (c *coveragePainter) DrawText(string)            { c.note("DrawText") }
func (c *coveragePainter) DrawText1(float64, float64, string) {
	c.note("DrawText1")
}
func (c *coveragePainter) DrawGlyphs([]Glyph) { c.note("DrawGlyphs") }
func (c *coveragePainter) DrawGlyph(*Glyph)   { c.note("DrawGlyph") }
func (c *coveragePainter) DrawPixmap(Pixmap)  { c.note("DrawPixmap") }
func (c *coveragePainter) DrawPixmap1(float64, float64, Pixmap) {
	c.note("DrawPixmap1")
}
func (c *coveragePainter) DrawPixmap2(float64, float64, Pixmap, float64, float64) {
	c.note("DrawPixmap2")
}
func (c *coveragePainter) DrawPixmap5(float64, float64, float64, float64, Pixmap) {
	c.note("DrawPixmap5")
}
func (c *coveragePainter) DrawIcon(Icon, float64, bool) { c.note("DrawIcon") }
func (c *coveragePainter) DrawIcon1(Icon, float64, float64, float64, bool) {
	c.note("DrawIcon1")
}

// TestRunPainterBatteryCoversAllInterfaceMethods reflects on
// paint.Painter to enumerate its declared methods, then runs the
// battery on a coverage-instrumented painter and asserts that every
// method got called at least once. Catches "added a method to
// Painter but forgot to extend RunPainterBattery" — without this
// guard the new method silently lives outside the conformance net.
//
// The few methods we deliberately exclude are pure-accessors that
// don't have side effects worth a dedicated check; they're noted
// inline below so a reader knows the omission is intentional.
func TestRunPainterBatteryCoversAllInterfaceMethods(t *testing.T) {
	cp := newCoveragePainter()
	RunPainterBattery(t, cp)

	piface := reflect.TypeOf((*Painter)(nil)).Elem()
	var missing []string
	for i := 0; i < piface.NumMethod(); i++ {
		name := piface.Method(i).Name
		if !cp.called[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Errorf("RunPainterBattery left %d Painter methods uncalled: %v\n"+
			"Add a check() entry in paint/conformance.go for each.",
			len(missing), missing)
	}
}
