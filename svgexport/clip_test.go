package svgexport

import (
	"encoding/xml"
	"strings"
	"testing"

	"silk/paint"
)

// TestClipEmitsClipPathAndOpensGroup verifies the basic clip flow:
// build a path, call Clip, and the output should contain a <clipPath>
// element with the path's "d" attribute, plus a <g clip-path="…">
// wrapper for subsequent content.
func TestClipEmitsClipPathAndOpensGroup(t *testing.T) {
	p := New(200, 200)
	p.Rectangle(10, 10, 80, 80)
	p.Clip()

	out := p.String()
	for _, want := range []string{
		`<defs>`,
		`<clipPath id="c0"><path d="M 10 10 L 90 10 L 90 90 L 10 90 Z"`,
		`</clipPath>`,
		`</defs>`,
		`<g clip-path="url(#c0)">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n----\n%s", want, out)
		}
	}
}

// TestClippedFillIsInsideGroup: drawing a rectangle then clipping
// then filling another rectangle — the fill <path> should appear
// inside the <g clip-path> wrapper.
func TestClippedFillIsInsideGroup(t *testing.T) {
	p := New(200, 200)
	// Build outer clip path.
	p.Rectangle(10, 10, 100, 100)
	p.Clip()
	// Fill another rect inside the clipped region.
	p.SetBrush1(paint.Color{R: 0, G: 200, B: 0, A: 255})
	p.Rectangle(50, 50, 200, 200)
	p.Fill()

	out := p.String()
	gIdx := strings.Index(out, `<g clip-path="url(#c0)">`)
	gCloseIdx := strings.LastIndex(out, "</g>")
	fillIdx := strings.Index(out, `fill="#00C800"`)
	if gIdx < 0 || gCloseIdx < 0 || fillIdx < 0 {
		t.Fatalf("missing key elements:\n%s", out)
	}
	if !(gIdx < fillIdx && fillIdx < gCloseIdx) {
		t.Errorf("fill should be between <g> open (%d) and </g> close (%d), got fill at %d\n%s",
			gIdx, gCloseIdx, fillIdx, out)
	}
}

// TestClipScopedBySaveRestore: Save, Clip, draw, Restore — the </g>
// should appear at Restore time, not at end of document.
func TestClipScopedBySaveRestore(t *testing.T) {
	p := New(200, 200)
	p.Save()
	p.Rectangle(10, 10, 80, 80)
	p.Clip()
	p.SetBrush1(paint.Color{R: 255, A: 255})
	p.Rectangle(0, 0, 200, 200)
	p.Fill()
	p.Restore()
	// After Restore, draw something — it should be OUTSIDE the
	// closed clip group.
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 255, A: 255})
	p.Rectangle(0, 0, 50, 50)
	p.Fill()

	out := p.String()
	// First the clip group opens.
	gOpen := strings.Index(out, `<g clip-path="url(#c0)">`)
	gClose := strings.Index(out, "</g>")
	blueFill := strings.Index(out, `fill="#0000FF"`)
	redFill := strings.Index(out, `fill="#FF0000"`)
	if gOpen < 0 || gClose < 0 || blueFill < 0 || redFill < 0 {
		t.Fatalf("missing key elements:\n%s", out)
	}
	// Red fill (inside clip) must be between <g> and </g>.
	if !(gOpen < redFill && redFill < gClose) {
		t.Errorf("red rect should be inside clip group:\n%s", out)
	}
	// Blue fill (after Restore) must be after </g>.
	if !(gClose < blueFill) {
		t.Errorf("blue rect should be outside clip group (after </g>):\n%s", out)
	}
}

// TestNestedClipsEmitSequentialIDs: two clips → c0 and c1.
func TestNestedClipsEmitSequentialIDs(t *testing.T) {
	p := New(200, 200)
	p.Save()
	p.Rectangle(0, 0, 100, 100)
	p.Clip()
	p.Save()
	p.Rectangle(10, 10, 50, 50)
	p.Clip()
	p.SetBrush1(paint.Color{R: 0, A: 255})
	p.Rectangle(0, 0, 200, 200)
	p.Fill()
	p.Restore()
	p.Restore()

	out := p.String()
	for _, want := range []string{
		`id="c0"`,
		`id="c1"`,
		`url(#c0)`,
		`url(#c1)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n----\n%s", want, out)
		}
	}
	// Two </g> tags should appear (one per Restore that closed a clip).
	if got := strings.Count(out, "</g>"); got < 2 {
		t.Errorf("expected ≥2 </g> tags for nested clips; got %d\n%s", got, out)
	}
}

// TestClipPreserveKeepsPath: ClipPreserve should leave the path
// intact so a follow-on Fill renders the same geometry that was
// clipped.
func TestClipPreserveKeepsPath(t *testing.T) {
	p := New(200, 200)
	p.Rectangle(10, 10, 80, 80)
	p.ClipPreserve()
	p.SetBrush1(paint.Color{R: 0, A: 255})
	p.Fill()

	out := p.String()
	if !strings.Contains(out, `<g clip-path="url(#c0)">`) {
		t.Errorf("clip group missing\n%s", out)
	}
	// Two paths emitted: one inside <clipPath>, one as the post-Fill
	// solid fill. The clipPath one carries no fill attribute but the
	// rect-Fill one does.
	pathCount := strings.Count(out, `<path d="`)
	if pathCount != 2 {
		t.Errorf("expected 2 path elements (clipPath def + Fill), got %d\n%s",
			pathCount, out)
	}
}

// TestEmptyClipIsNoOp: calling Clip with no path doesn't emit
// anything.
func TestEmptyClipIsNoOp(t *testing.T) {
	p := New(200, 200)
	p.Clip()

	out := p.String()
	if strings.Contains(out, `<clipPath`) || strings.Contains(out, `<g clip-path`) {
		t.Errorf("empty Clip should not emit clip path or group:\n%s", out)
	}
}

// TestClippedSVGOutputParsesAsXML: end-to-end well-formedness.
func TestClippedSVGOutputParsesAsXML(t *testing.T) {
	p := New(300, 300)
	p.SetBrush1(paint.Color{R: 240, G: 240, B: 240, A: 255})
	p.Rectangle(0, 0, 300, 300)
	p.Fill()

	p.Save()
	p.Rectangle(50, 50, 200, 200)
	p.Clip()
	p.SetBrush1(paint.Color{R: 200, G: 50, B: 50, A: 255})
	p.Rectangle(0, 0, 300, 300)
	p.Fill()
	p.Restore()

	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
	p.DrawText1(20, 290, "outside clip")

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

// TestUnclosedClipsAutoCloseAtWriteTime: a Clip without matching
// Save/Restore would leave an open <g>. WriteTo must still produce
// well-formed output by closing any remaining open <g> before
// </svg>.
func TestUnclosedClipsAutoCloseAtWriteTime(t *testing.T) {
	p := New(100, 100)
	p.Rectangle(10, 10, 80, 80)
	p.Clip()
	// No matching Restore.

	out := p.String()
	if !strings.Contains(out, "</g>") {
		t.Errorf("WriteTo should auto-close open <g>; got:\n%s", out)
	}

	dec := xml.NewDecoder(strings.NewReader(out))
	for {
		_, err := dec.Token()
		if err != nil && err.Error() == "EOF" {
			break
		}
		if err != nil {
			t.Fatalf("xml parse failed: %v\noutput:\n%s", err, out)
		}
	}
}

// TestResetClipIsNoOp documents that ResetClip stays a no-op (mirrors
// the pdfexport surface). Lock the contract in case someone tries to
// "fix" it without considering scope semantics.
func TestResetClipIsNoOp(t *testing.T) {
	p := New(100, 100)
	p.Rectangle(10, 10, 80, 80)
	p.Clip()
	p.ResetClip()

	// ResetClip didn't close the <g>; the auto-close at WriteTo will.
	out := p.String()
	gOpen := strings.Count(out, "<g clip-path=")
	gClose := strings.Count(out, "</g>")
	if gOpen != gClose {
		t.Errorf("g open/close mismatch (gOpen=%d gClose=%d):\n%s",
			gOpen, gClose, out)
	}
}
