package svg

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/paint"
)

// --- Parser tests ----------------------------------------------------

func TestParseEmptyDocReturnsError(t *testing.T) {
	if _, err := ParseString(""); err == nil {
		t.Errorf("empty input should error")
	}
}

func TestParseRootWithViewBox(t *testing.T) {
	src := `<svg width="24" height="24" viewBox="0 0 100 100"></svg>`
	doc, err := ParseString(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Width != 24 || doc.Height != 24 {
		t.Errorf("size = (%v, %v), want (24, 24)", doc.Width, doc.Height)
	}
	if doc.ViewBox.W != 100 || doc.ViewBox.H != 100 {
		t.Errorf("viewBox W/H = (%v, %v), want (100, 100)", doc.ViewBox.W, doc.ViewBox.H)
	}
}

func TestParseRectShape(t *testing.T) {
	src := `<svg><rect x="5" y="10" width="20" height="30" fill="red" stroke="blue"/></svg>`
	doc, _ := ParseString(src)
	if len(doc.Children) != 1 {
		t.Fatalf("children = %d, want 1", len(doc.Children))
	}
	r, ok := doc.Children[0].(*Rect)
	if !ok {
		t.Fatalf("child = %T, want *Rect", doc.Children[0])
	}
	if r.X != 5 || r.Y != 10 || r.W != 20 || r.H != 30 {
		t.Errorf("rect = (%v,%v,%v,%v), want (5,10,20,30)", r.X, r.Y, r.W, r.H)
	}
	if r.Style.Fill == nil || r.Style.Fill.Val.R != 255 {
		t.Errorf("fill = %#v, want red", r.Style.Fill)
	}
	if r.Style.Stroke == nil || r.Style.Stroke.Val.B != 255 {
		t.Errorf("stroke = %#v, want blue", r.Style.Stroke)
	}
}

func TestParseCircle(t *testing.T) {
	src := `<svg><circle cx="50" cy="50" r="25"/></svg>`
	doc, _ := ParseString(src)
	c := doc.Children[0].(*Circle)
	if c.Cx != 50 || c.Cy != 50 || c.R != 25 {
		t.Errorf("circle = (%v,%v,%v), want (50,50,25)", c.Cx, c.Cy, c.R)
	}
}

func TestParseHexColor(t *testing.T) {
	c := parseColor("#ff8000")
	if c.Val.R != 255 || c.Val.G != 128 || c.Val.B != 0 {
		t.Errorf("hex #ff8000 = %+v, want R=255 G=128 B=0", c.Val)
	}
}

func TestParseShortHexColor(t *testing.T) {
	c := parseColor("#f80")
	if c.Val.R != 255 || c.Val.G != 136 || c.Val.B != 0 {
		t.Errorf("hex #f80 expanded = %+v, want R=255 G=136 B=0", c.Val)
	}
}

func TestParseNoneColor(t *testing.T) {
	c := parseColor("none")
	if !c.None {
		t.Errorf("color 'none' should set None=true")
	}
}

func TestParseNamedColor(t *testing.T) {
	c := parseColor("orange")
	if c.Val.R != 255 || c.Val.G != 165 || c.Val.B != 0 {
		t.Errorf("orange = %+v, want R=255 G=165 B=0", c.Val)
	}
}

func TestParseInlineStyle(t *testing.T) {
	src := `<svg><rect style="fill:red;stroke-width:2.5"/></svg>`
	doc, _ := ParseString(src)
	r := doc.Children[0].(*Rect)
	if r.Style.Fill == nil || r.Style.Fill.Val.R != 255 {
		t.Errorf("inline fill not parsed")
	}
	if r.Style.StrokeWidth == nil || *r.Style.StrokeWidth != 2.5 {
		t.Errorf("inline stroke-width = %v, want 2.5", r.Style.StrokeWidth)
	}
}

// --- Path data parser -----------------------------------------------

func TestPathDataMoveLine(t *testing.T) {
	cmds, err := parsePathData("M 10 20 L 30 40")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []PathCmd{
		{Kind: PathMove, X: 10, Y: 20},
		{Kind: PathLine, X: 30, Y: 40},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("cmds = %#v, want %#v", cmds, want)
	}
}

func TestPathDataRelativeNormalisedToAbsolute(t *testing.T) {
	cmds, err := parsePathData("M 10 10 l 5 5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cmds[1].X != 15 || cmds[1].Y != 15 {
		t.Errorf("relative l 5 5 from (10,10) = (%v,%v), want (15,15)", cmds[1].X, cmds[1].Y)
	}
}

func TestPathDataHorizontalCollapsesToLine(t *testing.T) {
	cmds, _ := parsePathData("M 0 0 H 10")
	if cmds[1].Kind != PathLine || cmds[1].X != 10 || cmds[1].Y != 0 {
		t.Errorf("H collapsed = %#v, want PathLine to (10, 0)", cmds[1])
	}
}

func TestPathDataClosePath(t *testing.T) {
	cmds, _ := parsePathData("M 0 0 L 10 0 Z")
	if cmds[2].Kind != PathClose {
		t.Errorf("Z command = %v, want PathClose", cmds[2].Kind)
	}
}

func TestPathDataCubicAbsolute(t *testing.T) {
	cmds, err := parsePathData("M 0 0 C 1 1 2 2 3 3")
	if err != nil {
		t.Fatalf("%v", err)
	}
	c := cmds[1]
	if c.Kind != PathCurve || c.X1 != 1 || c.Y1 != 1 || c.X2 != 2 || c.Y2 != 2 || c.X != 3 || c.Y != 3 {
		t.Errorf("cubic = %#v", c)
	}
}

func TestPathDataMultipleCoordsAfterMove(t *testing.T) {
	// "M 0 0 10 10" is equivalent to "M 0 0 L 10 10".
	cmds, _ := parsePathData("M 0 0 10 10")
	if len(cmds) != 2 {
		t.Fatalf("len cmds = %d, want 2", len(cmds))
	}
	if cmds[1].Kind != PathLine {
		t.Errorf("trailing coord pair after M should be PathLine, got %v", cmds[1].Kind)
	}
}

func TestPathDataExponentialNumbers(t *testing.T) {
	cmds, err := parsePathData("M 1e2 2e1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cmds[0].X != 100 || cmds[0].Y != 20 {
		t.Errorf("exp form: got (%v, %v), want (100, 20)", cmds[0].X, cmds[0].Y)
	}
}

// --- Transform parser -----------------------------------------------

func TestParseTransformSingle(t *testing.T) {
	tr := parseTransform("translate(10 20)")
	if len(tr.Ops) != 1 || tr.Ops[0].Kind != TransformTranslate {
		t.Fatalf("ops = %#v", tr.Ops)
	}
	if tr.Ops[0].X != 10 || tr.Ops[0].Y != 20 {
		t.Errorf("translate args = (%v, %v), want (10, 20)", tr.Ops[0].X, tr.Ops[0].Y)
	}
}

func TestParseTransformChain(t *testing.T) {
	tr := parseTransform("translate(5) scale(2) rotate(45)")
	if len(tr.Ops) != 3 {
		t.Errorf("ops len = %d, want 3", len(tr.Ops))
	}
	if tr.Ops[2].Kind != TransformRotate || tr.Ops[2].X != 45 {
		t.Errorf("rotate arg = %v, want 45", tr.Ops[2].X)
	}
}

// --- Renderer integration via recording painter ----------------------

// recPainter records the public Painter calls into a sequence of
// strings so tests can assert "the renderer emitted Rectangle then
// Fill". We embed paint.Painter (nil) so type assertions in renderer
// don't fail; only the methods listed are overridden.
type recPainter struct {
	noopPainter
	calls []string
}

func (r *recPainter) Save() int              { r.calls = append(r.calls, "Save"); return 0 }
func (r *recPainter) Restore() int           { r.calls = append(r.calls, "Restore"); return 0 }
func (r *recPainter) Translate(x, y float64) { r.calls = append(r.calls, "Translate") }
func (r *recPainter) Scale(x, y float64)     { r.calls = append(r.calls, "Scale") }
func (r *recPainter) Rotate(rad float64)     { r.calls = append(r.calls, "Rotate") }
func (r *recPainter) Rectangle(x, y, w, h float64) {
	r.calls = append(r.calls, "Rectangle")
}
func (r *recPainter) Arc(cx, cy, rad, a0, a1 float64) { r.calls = append(r.calls, "Arc") }
func (r *recPainter) MoveTo(x, y float64)             { r.calls = append(r.calls, "MoveTo") }
func (r *recPainter) LineTo(x, y float64)             { r.calls = append(r.calls, "LineTo") }
func (r *recPainter) CurveTo(x1, y1, x2, y2, x, y float64) {
	r.calls = append(r.calls, "CurveTo")
}
func (r *recPainter) Fill()                            { r.calls = append(r.calls, "Fill") }
func (r *recPainter) FillPreserve()                    { r.calls = append(r.calls, "FillPreserve") }
func (r *recPainter) Stroke()                          { r.calls = append(r.calls, "Stroke") }
func (r *recPainter) SetBrush1(c paint.Color)          { r.calls = append(r.calls, "SetBrush1") }
func (r *recPainter) SetPen1(c paint.Color, w float64) { r.calls = append(r.calls, "SetPen1") }
func (r *recPainter) CurrentPoint() (x, y float64)     { return 0, 0 }

// TestRenderRectEmitsExpectedCalls: the simplest end-to-end check.
// A rect with fill should produce: Save+ ... Rectangle SetBrush1 Fill ... Restore.
func TestRenderRectEmitsExpectedCalls(t *testing.T) {
	doc, _ := ParseString(`<svg viewBox="0 0 100 100"><rect x="10" y="10" width="50" height="40" fill="red"/></svg>`)
	rec := &recPainter{}
	Render(doc, rec, 0, 0, 100, 100)

	hasRect, hasFill, hasBrush := false, false, false
	for _, c := range rec.calls {
		switch c {
		case "Rectangle":
			hasRect = true
		case "Fill":
			hasFill = true
		case "SetBrush1":
			hasBrush = true
		}
	}
	if !hasRect || !hasFill || !hasBrush {
		t.Errorf("calls = %v; want Rectangle + SetBrush1 + Fill", rec.calls)
	}
}

func TestRenderCircleEmitsArc(t *testing.T) {
	doc, _ := ParseString(`<svg><circle cx="50" cy="50" r="25" fill="blue"/></svg>`)
	rec := &recPainter{}
	Render(doc, rec, 0, 0, 100, 100)
	hasArc := false
	for _, c := range rec.calls {
		if c == "Arc" {
			hasArc = true
			break
		}
	}
	if !hasArc {
		t.Errorf("circle render did not emit Arc; calls = %v", rec.calls)
	}
}

func TestRenderPathMoveLine(t *testing.T) {
	doc, _ := ParseString(`<svg><path d="M 10 10 L 50 50" stroke="red" fill="none"/></svg>`)
	rec := &recPainter{}
	Render(doc, rec, 0, 0, 100, 100)
	moveSeen, lineSeen, strokeSeen := false, false, false
	for _, c := range rec.calls {
		switch c {
		case "MoveTo":
			moveSeen = true
		case "LineTo":
			lineSeen = true
		case "Stroke":
			strokeSeen = true
		}
	}
	if !moveSeen || !lineSeen || !strokeSeen {
		t.Errorf("path render: MoveTo=%v LineTo=%v Stroke=%v", moveSeen, lineSeen, strokeSeen)
	}
}

func TestRenderGroupCascadesStyle(t *testing.T) {
	// fill on the group should apply to the inner rect that has none.
	doc, _ := ParseString(`<svg><g fill="red"><rect width="10" height="10"/></g></svg>`)
	rec := &recPainter{}
	Render(doc, rec, 0, 0, 100, 100)
	// Rect with inherited fill="red" should emit SetBrush1 + Fill.
	hasBrush, hasFill := false, false
	for _, c := range rec.calls {
		switch c {
		case "SetBrush1":
			hasBrush = true
		case "Fill":
			hasFill = true
		}
	}
	if !hasBrush || !hasFill {
		t.Errorf("group fill cascade: SetBrush1=%v Fill=%v", hasBrush, hasFill)
	}
}

func TestRenderNilDocSafe(t *testing.T) {
	rec := &recPainter{}
	Render(nil, rec, 0, 0, 100, 100)
	if len(rec.calls) != 0 {
		t.Errorf("nil doc should emit nothing; got %v", rec.calls)
	}
}

func TestRenderZeroSizeSafe(t *testing.T) {
	doc, _ := ParseString(`<svg><rect width="10" height="10"/></svg>`)
	rec := &recPainter{}
	Render(doc, rec, 0, 0, 0, 0)
	if len(rec.calls) != 0 {
		t.Errorf("zero size should emit nothing; got %v", rec.calls)
	}
}

func TestParseFillNone(t *testing.T) {
	src := `<svg><circle cx="50" cy="50" r="25" fill="none" stroke="black"/></svg>`
	doc, _ := ParseString(src)
	c := doc.Children[0].(*Circle)
	if c.Style.Fill == nil || !c.Style.Fill.None {
		t.Errorf("fill='none' should set Fill.None=true, got %#v", c.Style.Fill)
	}
}
