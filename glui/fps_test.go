package glui

import (
	"strings"
	"testing"
	"time"
)

// TestFPSCounterBasic seeds the sliding window directly with 60 evenly
// spaced timestamps (16 ms apart) and verifies FPS() reports near 60.
// Bypassing Tick() lets us test the math without depending on real time.
func TestFPSCounterBasic(t *testing.T) {
	f := NewFPSCounter()
	if f.FPS() != 0 {
		t.Error("empty counter should report 0")
	}

	// Simulate 60 fps.
	base := time.Now()
	for i := 0; i < 60; i++ {
		f.frameTimes = append(f.frameTimes, base.Add(time.Duration(i)*time.Millisecond*16))
	}
	fps := f.FPS()
	if fps < 50 || fps > 70 {
		t.Errorf("expected ~60 fps, got %.1f", fps)
	}
}

// TestFPSCounterAvgFrameMs verifies the AvgFrameMs() reciprocal: at
// ~62.5 fps the average should be ~16 ms (1000/62.5).
func TestFPSCounterAvgFrameMs(t *testing.T) {
	f := NewFPSCounter()
	base := time.Now()
	for i := 0; i < 60; i++ {
		f.frameTimes = append(f.frameTimes, base.Add(time.Duration(i)*time.Millisecond*16))
	}
	ms := f.AvgFrameMs()
	if ms < 14 || ms > 18 {
		t.Errorf("expected ~16 ms, got %.2f", ms)
	}
}

// TestFPSCounterTickDropsStale exercises the sliding-window trim path:
// after a >1 s gap, Tick() must drop every previously recorded frame so
// FPS reflects the new cadence rather than a misleading average that
// includes the stale entries.
func TestFPSCounterTickDropsStale(t *testing.T) {
	f := NewFPSCounter()
	old := time.Now().Add(-2 * time.Second)
	for i := 0; i < 30; i++ {
		f.frameTimes = append(f.frameTimes, old.Add(time.Duration(i)*time.Millisecond))
	}
	f.Tick()
	if len(f.frameTimes) != 1 {
		t.Errorf("expected stale entries dropped, got len=%d", len(f.frameTimes))
	}
}

// TestFPSCounterFormat checks the HUD string layout doesn't drift.
func TestFPSCounterFormat(t *testing.T) {
	f := NewFPSCounter()
	base := time.Now()
	for i := 0; i < 60; i++ {
		f.frameTimes = append(f.frameTimes, base.Add(time.Duration(i)*time.Millisecond*16))
	}
	s := f.Format()
	if len(s) == 0 {
		t.Error("Format() returned empty string")
	}
	// Must contain both the rate and ms tokens.
	if !strings.Contains(s, "fps") || !strings.Contains(s, "ms") {
		t.Errorf("Format output missing tokens: %q", s)
	}
}
