package svgexport

import (
	"strings"
	"testing"

	"silk/paint"
)

// TestLinearGradientEmitsDefAndURLRef: a *LinearGradient brush
// previously collapsed to its first stop colour. Now we emit a
// <linearGradient> def and reference it as fill="url(#g0)" so the
// rendered SVG actually shows the full gradient.
func TestLinearGradientEmitsDefAndURLRef(t *testing.T) {
	p := New(100, 100)

	grad := paint.NewLinearGradient(0, 0, 100, 0)
	grad.AddStop(0, paint.Color{255, 0, 0, 255})
	grad.AddStop(1, paint.Color{0, 0, 255, 255})

	p.SetBrush(grad)
	p.Rectangle(0, 0, 100, 100)
	p.Fill()

	out := p.String()

	wants := []string{
		`<defs>`,
		`<linearGradient id="g0"`,
		`x1="0" y1="0" x2="100" y2="0"`,
		`<stop offset="0" stop-color="#FF0000"/>`,
		`<stop offset="1" stop-color="#0000FF"/>`,
		`</linearGradient>`,
		`fill="url(#g0)"`,
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("svg output missing %q\n----\n%s", w, out)
		}
	}
}

// TestRadialGradientEmitsRadialDef: same shape for radial gradients.
// SVG <radialGradient> uses (cx, cy, r) for the outer circle plus
// (fx, fy, fr) for the focal — single-centre gradients collapse the
// focal to cx,cy,r0.
func TestRadialGradientEmitsRadialDef(t *testing.T) {
	p := New(100, 100)

	grad := paint.NewRadialGradient(50, 50, 0, 50)
	grad.AddStop(0, paint.Color{255, 255, 255, 255})
	grad.AddStop(1, paint.Color{0, 0, 0, 0}) // transparent at edge

	p.SetBrush(grad)
	p.Rectangle(0, 0, 100, 100)
	p.Fill()

	out := p.String()
	wants := []string{
		`<radialGradient id="g0"`,
		`cx="50" cy="50" r="50"`,
		`fx="50" fy="50" fr="0"`,
		`stop-opacity="0"`, // alpha=0 stop
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("svg output missing %q\n----\n%s", w, out)
		}
	}
}

// TestSameGradientReusedDoesNotDuplicateDef: a single brush reused
// across two Fill calls only emits one <linearGradient> def. Each
// Fill references the same url(#g0), so callers reusing brushes
// don't bloat the output proportionally to call count.
func TestSameGradientReusedDoesNotDuplicateDef(t *testing.T) {
	p := New(100, 100)

	grad := paint.NewLinearGradient(0, 0, 100, 0)
	grad.AddStop(0, paint.Color{255, 0, 0, 255})
	grad.AddStop(1, paint.Color{0, 0, 255, 255})

	p.SetBrush(grad)
	p.Rectangle(0, 0, 100, 50)
	p.Fill()
	p.Rectangle(0, 50, 100, 50)
	p.Fill()

	out := p.String()
	if got := strings.Count(out, "<linearGradient "); got != 1 {
		t.Errorf("expected 1 <linearGradient> def for repeated brush, got %d\n%s", got, out)
	}
	if got := strings.Count(out, `fill="url(#g0)"`); got != 2 {
		t.Errorf("expected 2 fill=url(#g0) refs, got %d\n%s", got, out)
	}
}
