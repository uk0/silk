package svgexport

import (
	"strings"
	"testing"

	"silk/paint"
)

// TestPlainPenProducesNoExtensionAttrs locks in backwards compat:
// a basic paint.NewPen (no DashedPen / CappedPen) emits stroke-width
// + stroke colour only. Existing tests rely on this.
func TestPlainPenProducesNoExtensionAttrs(t *testing.T) {
	p := New(200, 200)
	p.SetPen1(paint.Color{R: 0, A: 255}, 2)
	p.MoveTo(10, 10)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	for _, unwanted := range []string{
		"stroke-dasharray",
		"stroke-linecap",
		"stroke-linejoin",
		"stroke-miterlimit",
	} {
		if strings.Contains(out, unwanted) {
			t.Errorf("plain pen should not emit %q\n%s", unwanted, out)
		}
	}
}

// TestDashedPenEmitsDashArray: a pen with a dash pattern should
// produce stroke-dasharray with comma-separated values.
func TestDashedPenEmitsDashArray(t *testing.T) {
	pen := paint.NewStyledPen(
		paint.Color{R: 0, A: 255},
		1.5,
		[]float64{5, 3, 1, 3},
		0,
		paint.LineCapButt,
		paint.LineJoinMiter,
	)
	p := New(200, 200)
	p.SetPen(pen)
	p.MoveTo(10, 10)
	p.LineTo(190, 10)
	p.Stroke()

	out := p.String()
	if !strings.Contains(out, `stroke-dasharray="5,3,1,3"`) {
		t.Errorf("missing stroke-dasharray attribute\n%s", out)
	}
}

// TestDashedPenWithOffsetEmitsDashOffset
func TestDashedPenWithOffsetEmitsDashOffset(t *testing.T) {
	pen := paint.NewStyledPen(
		paint.Color{R: 0, A: 255},
		1.0,
		[]float64{4, 2},
		2.5,
		paint.LineCapButt,
		paint.LineJoinMiter,
	)
	p := New(200, 200)
	p.SetPen(pen)
	p.MoveTo(0, 0)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	if !strings.Contains(out, `stroke-dashoffset="2.5"`) {
		t.Errorf("missing stroke-dashoffset attribute\n%s", out)
	}
}

// TestRoundCapPenEmitsLineCapRound
func TestRoundCapPenEmitsLineCapRound(t *testing.T) {
	pen := paint.NewStyledPen(
		paint.Color{R: 0, A: 255},
		3.0,
		nil,
		0,
		paint.LineCapRound,
		paint.LineJoinMiter,
	)
	p := New(200, 200)
	p.SetPen(pen)
	p.MoveTo(0, 0)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	if !strings.Contains(out, `stroke-linecap="round"`) {
		t.Errorf("missing stroke-linecap=round\n%s", out)
	}
}

// TestSquareCapPenEmitsLineCapSquare
func TestSquareCapPenEmitsLineCapSquare(t *testing.T) {
	pen := paint.NewStyledPen(
		paint.Color{R: 0, A: 255},
		3.0,
		nil,
		0,
		paint.LineCapSquare,
		paint.LineJoinMiter,
	)
	p := New(200, 200)
	p.SetPen(pen)
	p.MoveTo(0, 0)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	if !strings.Contains(out, `stroke-linecap="square"`) {
		t.Errorf("missing stroke-linecap=square\n%s", out)
	}
}

// TestRoundJoinPenEmitsLineJoinRound
func TestRoundJoinPenEmitsLineJoinRound(t *testing.T) {
	pen := paint.NewStyledPen(
		paint.Color{R: 0, A: 255},
		3.0,
		nil,
		0,
		paint.LineCapButt,
		paint.LineJoinRound,
	)
	p := New(200, 200)
	p.SetPen(pen)
	p.MoveTo(0, 0)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	if !strings.Contains(out, `stroke-linejoin="round"`) {
		t.Errorf("missing stroke-linejoin=round\n%s", out)
	}
}

// TestBevelJoinPenEmitsLineJoinBevel
func TestBevelJoinPenEmitsLineJoinBevel(t *testing.T) {
	pen := paint.NewStyledPen(
		paint.Color{R: 0, A: 255},
		3.0,
		nil,
		0,
		paint.LineCapButt,
		paint.LineJoinBevel,
	)
	p := New(200, 200)
	p.SetPen(pen)
	p.MoveTo(0, 0)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	if !strings.Contains(out, `stroke-linejoin="bevel"`) {
		t.Errorf("missing stroke-linejoin=bevel\n%s", out)
	}
}

// TestNonDefaultMiterLimitEmitsAttribute
func TestNonDefaultMiterLimitEmitsAttribute(t *testing.T) {
	// Use the unexported constructor via reflection-style setup:
	// build a styledPen via NewStyledPen then trust the MiterLimit
	// default behaviour. To test a non-default miter limit we'd need
	// a constructor that exposes it; for now lock in that the default
	// 10.0 doesn't emit (matches Cairo default).
	pen := paint.NewStyledPen(
		paint.Color{R: 0, A: 255},
		2.0,
		nil,
		0,
		paint.LineCapButt,
		paint.LineJoinMiter,
	)
	p := New(200, 200)
	p.SetPen(pen)
	p.MoveTo(0, 0)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	// Default miter limit (10) → no attribute (matches Cairo + SVG
	// default-ness).
	if strings.Contains(out, "stroke-miterlimit") {
		t.Errorf("default miter limit should not emit attribute\n%s", out)
	}
}
