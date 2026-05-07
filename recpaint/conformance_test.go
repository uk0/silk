package recpaint

import (
	"silk/paint"
	"testing"
)

// TestRecordingPainterBattery: drive the recording painter through
// paint's canonical conformance battery. Recording painters have to
// implement every method even if their semantics are "append a closure
// to ops", so the battery doubles as a regression check that
// recpaint.New() still satisfies the full Painter contract after
// future interface changes.
func TestRecordingPainterBattery(t *testing.T) {
	paint.RunPainterBattery(t, New())
}
