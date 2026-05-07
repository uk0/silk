package paint

import "testing"

// TestCairoPainterBattery: drives the cairoPainter through every
// method of paint.Painter to catch nil-derefs / panics on simple
// inputs. See conformance.go for the canonical battery sequence.
func TestCairoPainterBattery(t *testing.T) {
	pix := NewPixmap(200, 200)
	g := pix.NewPainter()
	RunPainterBattery(t, g)
}
