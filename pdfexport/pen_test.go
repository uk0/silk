package pdfexport

import (
	"strings"
	"testing"

	"silk/paint"
)

// TestPlainPenProducesNoExtensionOps locks backwards compat: a basic
// paint.NewPen emits "RG" + "w" + "S" with no dash/cap/join operators.
func TestPlainPenProducesNoExtensionOps(t *testing.T) {
	p := New(200, 200)
	p.SetPen1(paint.Color{R: 0, A: 255}, 2)
	p.MoveTo(10, 10)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	for _, unwanted := range []string{
		" d\n", // dash array
		" J\n", // line cap
		" j\n", // line join
		" M\n", // miter limit
	} {
		if strings.Contains(out, unwanted) {
			t.Errorf("plain pen should not emit %q\n%s", unwanted, out)
		}
	}
}

// TestDashedPenEmitsDashOperator
func TestDashedPenEmitsDashOperator(t *testing.T) {
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
	if !strings.Contains(out, "[5 3 1 3] 0 d") {
		t.Errorf("missing dash array operator\n%s", out)
	}
}

// TestDashedPenWithOffsetEmitsCorrectPhase
func TestDashedPenWithOffsetEmitsCorrectPhase(t *testing.T) {
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
	if !strings.Contains(out, "[4 2] 2.5 d") {
		t.Errorf("dash phase 2.5 should appear in d operator\n%s", out)
	}
}

// TestRoundCapPenEmits1J
func TestRoundCapPenEmits1J(t *testing.T) {
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
	if !strings.Contains(out, "1 J\n") {
		t.Errorf("round cap should emit '1 J'\n%s", out)
	}
}

// TestSquareCapPenEmits2J
func TestSquareCapPenEmits2J(t *testing.T) {
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
	if !strings.Contains(out, "2 J\n") {
		t.Errorf("square cap should emit '2 J'\n%s", out)
	}
}

// TestRoundJoinPenEmits1j
func TestRoundJoinPenEmits1j(t *testing.T) {
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
	if !strings.Contains(out, "1 j\n") {
		t.Errorf("round join should emit '1 j'\n%s", out)
	}
}

// TestBevelJoinPenEmits2j
func TestBevelJoinPenEmits2j(t *testing.T) {
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
	if !strings.Contains(out, "2 j\n") {
		t.Errorf("bevel join should emit '2 j'\n%s", out)
	}
}

// TestPenExtensionsPrecedeStrokeOperator: dash/cap/join must come
// BEFORE the S operator so they take effect for that stroke.
func TestPenExtensionsPrecedeStrokeOperator(t *testing.T) {
	pen := paint.NewStyledPen(
		paint.Color{R: 0, A: 255},
		2.0,
		[]float64{4, 2},
		0,
		paint.LineCapRound,
		paint.LineJoinRound,
	)
	p := New(200, 200)
	p.SetPen(pen)
	p.MoveTo(0, 0)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	dIdx := strings.Index(out, " d\n")
	jIdx := strings.Index(out, " J\n") // line cap
	jjIdx := strings.Index(out, " j\n") // line join
	sIdx := strings.LastIndex(out, "\nS\n")
	if dIdx < 0 || jIdx < 0 || jjIdx < 0 || sIdx < 0 {
		t.Fatalf("expected operators missing:\n%s", out)
	}
	if !(dIdx < sIdx && jIdx < sIdx && jjIdx < sIdx) {
		t.Errorf("dash/cap/join operators must precede S; got d=%d J=%d j=%d S=%d\n%s",
			dIdx, jIdx, jjIdx, sIdx, out)
	}
}
