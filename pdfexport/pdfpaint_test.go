package pdfexport

import (
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

// TestImplementsPainter is the compile-time guarantee that PDFPainter
// satisfies the paint.Painter interface. Catches signature drift.
func TestImplementsPainter(t *testing.T) {
	var _ paint.Painter = New(595, 842)
}

// TestEmptyDocumentStructure verifies the bare-minimum doc structure
// — header, all five objects, xref, trailer, %%EOF.
func TestEmptyDocumentStructure(t *testing.T) {
	p := New(595, 842)
	out := p.String()

	for _, want := range []string{
		"%PDF-1.4\n",
		"1 0 obj\n",
		"<< /Type /Catalog",
		"2 0 obj\n",
		"<< /Type /Pages",
		"3 0 obj\n",
		"<< /Type /Page",
		"/MediaBox [0 0 595 842]",
		"4 0 obj\n",
		"stream\n",
		"endstream\n",
		"5 0 obj\n",
		"/BaseFont /Helvetica",
		"xref\n",
		"0 6\n",
		"trailer\n",
		"/Size 6 /Root 1 0 R",
		"startxref\n",
		"%%EOF\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n----\n%s", want, out)
		}
	}
}

// TestRectFillEmitsReOperator: PDF has a native rectangle operator
// "x y w h re"; our Rectangle should use it (faster than four lineto +
// closepath) and flush via "f" (nonzero fill).
func TestRectFillEmitsReOperator(t *testing.T) {
	p := New(595, 842)
	p.SetBrush1(paint.Color{R: 255, G: 0, B: 0, A: 255})
	p.Rectangle(10, 20, 80, 40)
	p.Fill()

	out := p.String()
	if !strings.Contains(out, " re\n") {
		t.Errorf("rectangle should use 're' operator\n%s", out)
	}
	if !strings.Contains(out, "1 0 0 rg") {
		t.Errorf("fill colour should emit '1 0 0 rg'\n%s", out)
	}
	if !strings.Contains(out, "f\n") {
		t.Errorf("Fill should emit 'f' operator\n%s", out)
	}
}

// TestStrokeEmitsLineWidthAndStrokeColor: pen color → "RG", width →
// "w", path → "S".
func TestStrokeEmitsLineWidthAndStrokeColor(t *testing.T) {
	p := New(595, 842)
	p.SetPen1(paint.Color{R: 0, G: 0, B: 255, A: 255}, 2)
	p.MoveTo(10, 10)
	p.LineTo(100, 100)
	p.Stroke()

	out := p.String()
	if !strings.Contains(out, "0 0 1 RG") {
		t.Errorf("stroke colour should emit '0 0 1 RG'\n%s", out)
	}
	if !strings.Contains(out, "2 w\n") {
		t.Errorf("line width should emit '2 w'\n%s", out)
	}
	if !strings.Contains(out, "\nS\n") {
		t.Errorf("Stroke should emit 'S' operator\n%s", out)
	}
}

// TestYFlipMapsTopLeftToBottomLeftPDF: PDF's origin is bottom-left;
// paint.Painter's is top-left. A rect at (0, 0, w, h) in our coords
// must land at PDF (0, H-h) — i.e. flush to the top of the page.
func TestYFlipMapsTopLeftToBottomLeftPDF(t *testing.T) {
	const W, H = 100.0, 100.0
	p := New(W, H)
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
	p.Rectangle(0, 0, 50, 30)
	p.Fill()

	out := p.String()
	// Expected re command: "0 70 50 30 re" — y=H-h=70.
	if !strings.Contains(out, "0 70 50 30 re") {
		t.Errorf("Y-flip wrong: top-left rect should land at y=H-h=70\n%s", out)
	}
}

// TestArcEmitsCubicCurves: PDF has no native arc operator, so Arc
// must decompose into 'c' (cubic Bezier) calls — at least one for any
// non-zero sweep.
func TestArcEmitsCubicCurves(t *testing.T) {
	p := New(595, 842)
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
	p.MoveTo(50, 50)
	p.Arc(50, 50, 20, 0, 1.5707963)
	p.Fill()

	out := p.String()
	curveCount := strings.Count(out, " c\n")
	if curveCount == 0 {
		t.Errorf("Arc should emit ≥1 cubic Bezier (c operator); got 0\n%s", out)
	}
}

// TestSaveRestoreEmitsQAndCapitalQ verifies the q/Q nesting matches
// the PDF graphics-state convention. We isolate the content stream
// (between "stream\n" and "endstream") so q/Q from the document
// chrome don't pollute the count.
func TestSaveRestoreEmitsQAndCapitalQ(t *testing.T) {
	p := New(595, 842)
	p.Save()
	p.Save()
	p.Restore()
	p.Restore()

	out := p.String()
	streamStart := strings.Index(out, "stream\n")
	streamEnd := strings.Index(out, "endstream")
	if streamStart < 0 || streamEnd < 0 || streamEnd <= streamStart {
		t.Fatalf("could not locate content stream:\n%s", out)
	}
	body := out[streamStart+len("stream\n") : streamEnd]
	if got := strings.Count(body, "q\n"); got != 2 {
		t.Errorf("expected 2 'q' operators, got %d in stream:\n%s", got, body)
	}
	if got := strings.Count(body, "Q\n"); got != 2 {
		t.Errorf("expected 2 'Q' operators, got %d in stream:\n%s", got, body)
	}
}

// TestDrawText1EmitsBTET verifies the BT…ET block, font selection
// (/F1), text-matrix Y-flip, and Tj string emission.
func TestDrawText1EmitsBTET(t *testing.T) {
	p := New(595, 842)
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
	p.DrawText1(72, 80, "Hello PDF")

	out := p.String()
	for _, want := range []string{
		"BT\n",
		"/F1 14 Tf\n",
		"1 0 0 -1 ", // text matrix flip
		"(Hello PDF) Tj\n",
		"ET\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n----\n%s", want, out)
		}
	}
}

// TestEscapePDFStringEscapesParensAndBackslash: "(", ")", "\" must be
// backslash-escaped in PDF literal strings — otherwise the string
// terminator parser breaks.
func TestEscapePDFStringEscapesParensAndBackslash(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"a(b)c", `a\(b\)c`},
		{`back\slash`, `back\\slash`},
		{"x(y)\\z", `x\(y\)\\z`},
	}
	for _, c := range cases {
		if got := escapePDFString(c.in); got != c.want {
			t.Errorf("escape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestXrefOffsetsAreAccurate: the xref table must point at the actual
// byte positions of each "N 0 obj" header. Wrong offsets cause every
// PDF reader to refuse the document. We verify by reading the xref,
// extracting the offsets, and confirming each one points at "N 0 obj".
func TestXrefOffsetsAreAccurate(t *testing.T) {
	p := New(595, 842)
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
	p.Rectangle(0, 0, 100, 100)
	p.Fill()
	out := p.String()

	xrefIdx := strings.Index(out, "xref\n")
	if xrefIdx < 0 {
		t.Fatal("no xref")
	}
	// Skip "xref\n" + "0 6\n" + free-list line.
	cur := xrefIdx + len("xref\n0 6\n")
	cur += len("0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		// Each xref entry is exactly 20 bytes.
		entry := out[cur : cur+20]
		cur += 20
		off := 0
		for j := 0; j < 10; j++ {
			off = off*10 + int(entry[j]-'0')
		}
		// Verify the byte at offset is "N" (digit) and "N 0 obj" follows.
		want := []byte{'0' + byte(i), ' ', '0', ' ', 'o', 'b', 'j'}
		if off+len(want) > len(out) || string(out[off:off+len(want)]) != string(want) {
			t.Errorf("xref entry %d offset %d does not point at %q (saw %q)",
				i, off, string(want), out[off:min(off+len(want), len(out))])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestStartXrefMatchesActualPosition: the "startxref" value at the end
// of the trailer must be the byte offset of the "xref\n" keyword.
// Readers use this to seek without scanning.
func TestStartXrefMatchesActualPosition(t *testing.T) {
	p := New(595, 842)
	out := p.String()

	xrefIdx := strings.Index(out, "xref\n")
	if xrefIdx < 0 {
		t.Fatal("no xref")
	}

	// startxref is followed by a newline-terminated decimal number.
	startIdx := strings.Index(out, "startxref\n")
	if startIdx < 0 {
		t.Fatal("no startxref")
	}
	numStart := startIdx + len("startxref\n")
	numEnd := numStart
	for numEnd < len(out) && out[numEnd] >= '0' && out[numEnd] <= '9' {
		numEnd++
	}
	declared := 0
	for i := numStart; i < numEnd; i++ {
		declared = declared*10 + int(out[i]-'0')
	}
	if declared != xrefIdx {
		t.Errorf("startxref says %d but xref is actually at %d", declared, xrefIdx)
	}
}

// TestNonOpaqueAlphaMappingFallback: non-opaque fills currently
// degrade to opaque RGB (PDF needs an ExtGState dictionary for
// alpha). Lock the current behaviour so a future ExtGState upgrade
// is detected via test diff.
func TestNonOpaqueAlphaMappingFallback(t *testing.T) {
	p := New(100, 100)
	p.SetBrush1(paint.Color{R: 255, G: 0, B: 0, A: 128})
	p.Rectangle(0, 0, 50, 50)
	p.Fill()

	out := p.String()
	// The "rg" line is RGB only — alpha doesn't appear in fixed-function
	// PDF graphics state.
	if !strings.Contains(out, "1 0 0 rg") {
		t.Errorf("non-opaque alpha should still emit RGB rg; got\n%s", out)
	}
}
