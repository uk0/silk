package svgexport

import (
	"github.com/uk0/silk/paint"
	"testing"
)

// TestSVGPainterBattery: drive the SVG painter through paint's
// canonical conformance battery. Catches drift if a paint.Painter
// method gains a behaviour the SVG side hasn't kept up with.
func TestSVGPainterBattery(t *testing.T) {
	p := New(200, 200)
	paint.RunPainterBattery(t, p)
}
