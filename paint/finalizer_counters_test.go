package paint

import (
	"runtime"
	"testing"
)

// TestGradientPatternCounterUnderGC is written to be run under -race. The cairo
// object counters are bumped on whatever thread allocates and dropped again in
// a runtime finalizer, which runs on the runtime's own goroutine — as plain
// ints that is an unsynchronized write pair, and the detector reported it as
// "WARNING: DATA RACE ... paint/brush.go:97 / :103". Because gui and ged
// allocate patterns, surfaces and painters on every repaint, plain-int counters
// turn any -race run of those packages into a coin flip.
//
// Discarding each gradient right after it builds its pattern keeps the
// finalizer goroutine dropping counts while this one keeps adding them, which
// is what makes the pair observable. The count going negative would mean an
// add was lost to a torn read-modify-write.
func TestGradientPatternCounterUnderGC(t *testing.T) {
	for i := 0; i < 20000; i++ {
		g := NewLinearGradient(0, 0, float32(i), 1)
		g.AddStop(0, Color{1, 2, 3, 4})
		g.cairoPattern()
		if i%500 != 0 {
			continue
		}
		runtime.GC()
		if n := cairoPatternCount.Load(); n < 0 {
			t.Fatalf("pattern count went negative (%d): an increment was lost", n)
		}
	}
}
