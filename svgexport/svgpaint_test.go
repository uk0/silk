package svgexport

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

// TestImplementsPainter is a compile-time check that SVGPainter
// satisfies the paint.Painter interface. If a method changes
// signature in paint and breaks here, the build catches it.
func TestImplementsPainter(t *testing.T) {
	var _ paint.Painter = New(100, 100)
}

// TestRectFillEmitsRectPath confirms that a basic Rectangle + Fill
// produces a <path> with the canonical M/L/Z d string and the brush
// colour as fill.
func TestRectFillEmitsRectPath(t *testing.T) {
	p := New(200, 100)
	p.SetBrush1(paint.Color{R: 255, G: 0, B: 0, A: 255})
	p.Rectangle(10, 20, 80, 40)
	p.Fill()

	out := p.String()
	for _, want := range []string{
		`<svg`,
		`width="200"`,
		`height="100"`,
		`<path`,
		`fill="#FF0000"`,
		`M 10 20`,
		`L 90 20`,
		`L 90 60`,
		`L 10 60`,
		`Z`,
		`</svg>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n----\n%s", want, out)
		}
	}
}

// TestStrokeEmitsStrokeAttributes verifies the stroke path: stroke
// colour + stroke-width come from the active pen, fill is "none".
func TestStrokeEmitsStrokeAttributes(t *testing.T) {
	p := New(200, 100)
	p.SetPen1(paint.Color{R: 0, G: 0, B: 255, A: 255}, 3)
	p.MoveTo(10, 10)
	p.LineTo(90, 60)
	p.Stroke()

	out := p.String()
	for _, want := range []string{
		`fill="none"`,
		`stroke="#0000FF"`,
		`stroke-width="3"`,
		`M 10 10`,
		`L 90 60`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n----\n%s", want, out)
		}
	}
}

// TestTransformFoldsIntoCoords ensures CTM is pre-applied to emitted
// coordinates — output should NOT carry transform="…" attributes.
func TestTransformFoldsIntoCoords(t *testing.T) {
	p := New(200, 100)
	p.Translate(50, 30)
	p.SetBrush1(paint.Color{R: 0, G: 255, B: 0, A: 255})
	p.Rectangle(10, 10, 20, 20)
	p.Fill()

	out := p.String()
	if strings.Contains(out, `transform=`) {
		t.Errorf("output should not carry transform attribute; CTM should be folded into coords:\n%s", out)
	}
	// After the +50/+30 translate, rect at (10,10) lands at (60,40).
	if !strings.Contains(out, `M 60 40`) {
		t.Errorf("translated rect should start at M 60 40:\n%s", out)
	}
}

// TestSaveRestoreRestoresCTMAndStyle locks in the basic state-stack
// contract: after Save → mutate → Restore the painter is back to its
// pre-Save configuration.
func TestSaveRestoreRestoresCTMAndStyle(t *testing.T) {
	p := New(200, 100)

	depth0 := p.CurrentState()
	if depth0 != 0 {
		t.Fatalf("initial depth = %d, want 0", depth0)
	}
	p.SetBrush1(paint.Color{R: 1, G: 2, B: 3, A: 255})

	p.Save()
	p.Translate(100, 100)
	p.SetBrush1(paint.Color{R: 9, G: 9, B: 9, A: 255})
	if p.CurrentState() != 1 {
		t.Fatal("depth should be 1 after Save")
	}

	p.Restore()
	if p.CurrentState() != 0 {
		t.Fatal("depth should be 0 after Restore")
	}

	// CTM should be back to identity → rect at (10,10) lands at (10,10).
	p.Rectangle(10, 10, 5, 5)
	p.Fill()
	out := p.String()
	if !strings.Contains(out, `M 10 10`) {
		t.Errorf("post-Restore rect should land at original CTM:\n%s", out)
	}
	// Brush should be back to (1,2,3,255).
	if !strings.Contains(out, `fill="#010203"`) {
		t.Errorf("post-Restore brush should be #010203:\n%s", out)
	}
}

// TestArcEmitsSVGArcCommand confirms the Arc → SVG "A" translation:
// the path should contain an A command after its initial position.
func TestArcEmitsSVGArcCommand(t *testing.T) {
	p := New(200, 100)
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
	p.MoveTo(50, 50)
	p.Arc(50, 50, 20, 0, 1.5707963) // quarter turn from (70,50) to (50,70)
	p.Fill()

	out := p.String()
	if !strings.Contains(out, " A ") {
		t.Errorf("output should contain SVG arc command:\n%s", out)
	}
}

// TestDrawText1EmitsTextElement confirms that DrawText1 produces a
// <text> element with the supplied position + brush colour.
func TestDrawText1EmitsTextElement(t *testing.T) {
	p := New(200, 100)
	p.SetBrush1(paint.Color{R: 100, G: 100, B: 100, A: 255})
	p.DrawText1(20, 50, "Hello & welcome")

	out := p.String()
	for _, want := range []string{
		`<text x="20" y="50"`,
		`fill="#646464"`,
		`Hello &amp; welcome`,
		`</text>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n----\n%s", want, out)
		}
	}
}

// TestOutputParsesAsXML verifies the emitted document is well-formed
// XML — encoding/xml must consume it without error.
func TestOutputParsesAsXML(t *testing.T) {
	p := New(300, 200)
	p.SetBrush1(paint.Color{R: 200, G: 200, B: 200, A: 255})
	p.Rectangle(0, 0, 300, 200)
	p.Fill()
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 255, A: 255})
	p.MoveTo(10, 10)
	p.LineTo(290, 190)
	p.Stroke()
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
	p.DrawText1(20, 50, "<embedded />")

	dec := xml.NewDecoder(strings.NewReader(p.String()))
	for {
		tok, err := dec.Token()
		if err != nil && err.Error() == "EOF" {
			break
		}
		if err != nil {
			t.Fatalf("xml parse failed: %v\noutput:\n%s", err, p.String())
		}
		_ = tok
	}
}

// TestNonOpaqueAlphaUsesRgba confirms the alpha-aware colour
// stringifier — opaque colours go through #RRGGBB, translucent
// through rgba(...).
func TestNonOpaqueAlphaUsesRgba(t *testing.T) {
	p := New(100, 100)
	p.SetBrush1(paint.Color{R: 255, G: 0, B: 0, A: 128})
	p.Rectangle(0, 0, 50, 50)
	p.Fill()

	out := p.String()
	if !strings.Contains(out, `rgba(255,0,0,`) {
		t.Errorf("non-opaque brush should emit rgba(...); got:\n%s", out)
	}
}

// TestEmptyPathFillIsNoOp covers a defensive case: calling Fill before
// any path commands must not crash and must not emit a stray <path/>.
func TestEmptyPathFillIsNoOp(t *testing.T) {
	p := New(100, 100)
	p.Fill()

	out := p.String()
	if strings.Contains(out, `<path`) {
		t.Errorf("empty Fill should not produce a <path> element:\n%s", out)
	}
}

// TestTextEscapingHandlesMarkup ensures user-supplied text doesn't
// break the document or smuggle elements in.
func TestTextEscapingHandlesMarkup(t *testing.T) {
	p := New(100, 100)
	p.DrawText1(0, 50, `<script>alert("xss")</script>`)

	out := p.String()
	if strings.Contains(out, "<script>") {
		t.Errorf("text content should be escaped:\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("text content should appear escaped:\n%s", out)
	}
}
