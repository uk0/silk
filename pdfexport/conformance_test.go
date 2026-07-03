package pdfexport

import (
	"github.com/uk0/silk/paint"
	"testing"
)

// TestPDFPainterBattery: drive the PDF painter through paint's
// canonical conformance battery. Same purpose as the SVG version —
// catches divergence as the Painter interface or its semantics evolve.
func TestPDFPainterBattery(t *testing.T) {
	p := New(200, 200)
	paint.RunPainterBattery(t, p)
}
