package svgexport

import (
	"strings"
	"testing"

	"silk/paint"
)

// TestSetOperatorEmitsMixBlendMode: every paint.Operator that maps
// to a CSS keyword must show up as the matching mix-blend-mode style
// attribute on the next emitted path. Locks in the table-driven
// switch in blendModeForOp.
func TestSetOperatorEmitsMixBlendMode(t *testing.T) {
	cases := []struct {
		op   paint.Operator
		want string
	}{
		{paint.OpMultiply, "mix-blend-mode:multiply"},
		{paint.OpScreen, "mix-blend-mode:screen"},
		{paint.OpOverlay, "mix-blend-mode:overlay"},
		{paint.OpDarken, "mix-blend-mode:darken"},
		{paint.OpLighten, "mix-blend-mode:lighten"},
		{paint.OpColorDodge, "mix-blend-mode:color-dodge"},
		{paint.OpColorBurn, "mix-blend-mode:color-burn"},
		{paint.OpHardLigh, "mix-blend-mode:hard-light"},
		{paint.OpSoftLigh, "mix-blend-mode:soft-light"},
		{paint.OpDifference, "mix-blend-mode:difference"},
		{paint.OpExclusion, "mix-blend-mode:exclusion"},
		{paint.OpHslHue, "mix-blend-mode:hue"},
		{paint.OpHslSaturate, "mix-blend-mode:saturation"},
		{paint.OpHslColor, "mix-blend-mode:color"},
		{paint.OpHslLuminosity, "mix-blend-mode:luminosity"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			p := New(100, 100)
			p.SetOperator(c.op)
			p.SetBrush1(paint.Color{255, 0, 0, 255})
			p.Rectangle(0, 0, 50, 50)
			p.Fill()
			out := p.String()
			if !strings.Contains(out, c.want) {
				t.Errorf("op missing %q\n----\n%s", c.want, out)
			}
		})
	}
}

// TestSetOperatorOverOmitsAttribute: the default OpOver should NOT
// emit a mix-blend-mode attribute — bloating every path with
// "style=mix-blend-mode:normal" would balloon the SVG output for
// no visual change. The same applies to the pure-compositing ops
// (OpSource/OpClear/OpDestOver/etc.) which have no CSS analog.
func TestSetOperatorOverOmitsAttribute(t *testing.T) {
	cases := []paint.Operator{
		paint.OpOver,
		paint.OpSource,
		paint.OpClear,
		paint.OpIn,
		paint.OpDestOver,
	}
	for _, op := range cases {
		p := New(100, 100)
		p.SetOperator(op)
		p.SetBrush1(paint.Color{0, 0, 255, 255})
		p.Rectangle(0, 0, 10, 10)
		p.Fill()
		if got := p.String(); strings.Contains(got, "mix-blend-mode") {
			t.Errorf("op=%v should not emit mix-blend-mode\n----\n%s", op, got)
		}
	}
}

// TestSetOperatorRestoredBySaveRestore: Save/Restore around a
// SetOperator change must restore the prior operator, so blend
// modes follow Cairo's "operator is graphics-state" semantics.
func TestSetOperatorRestoredBySaveRestore(t *testing.T) {
	p := New(100, 100)
	p.SetOperator(paint.OpOver)
	p.Save()
	p.SetOperator(paint.OpMultiply)
	p.SetBrush1(paint.Color{255, 0, 0, 255})
	p.Rectangle(0, 0, 10, 10)
	p.Fill() // emits multiply
	p.Restore()
	p.SetBrush1(paint.Color{0, 0, 255, 255})
	p.Rectangle(0, 10, 10, 10)
	p.Fill() // should NOT emit mix-blend-mode

	out := p.String()
	if strings.Count(out, "mix-blend-mode:multiply") != 1 {
		t.Errorf("expected exactly one multiply mode\n----\n%s", out)
	}
	// Find the second <path> and ensure no mix-blend-mode on it.
	paths := strings.Split(out, "<path")
	if len(paths) < 3 {
		t.Fatalf("expected ≥2 paths in output\n----\n%s", out)
	}
	if strings.Contains(paths[2], "mix-blend-mode") {
		t.Errorf("post-Restore path inherited blend mode\n----\n%s", paths[2])
	}
}
