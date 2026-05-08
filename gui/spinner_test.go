package gui

import (
	"testing"
	"time"

	"silk/paint"
)

// TestNewSpinnerDefaults locks in the construction-time settings so a
// regression that flipped any one of them surfaces as a single
// failing test rather than leaking through the demo's visual diff.
//
// SetBusy(false) on cleanup matters — the construction-time
// startAnim() registers a looping Animation with the global
// animManager, which would otherwise survive past this test and
// leak into TestSetBusy* assertions.
func TestNewSpinnerDefaults(t *testing.T) {
	s := NewSpinner()
	defer s.SetBusy(false)
	if !s.IsBusy() {
		t.Errorf("new spinner should default busy=true (caller toggles off if needed)")
	}
	if s.DotCount() != 8 {
		t.Errorf("default DotCount = %d, want 8", s.DotCount())
	}
	if s.CycleDuration() != time.Second {
		t.Errorf("default CycleDuration = %v, want 1s", s.CycleDuration())
	}
	// Color should be the theme accent — exact RGB depends on theme
	// but it must be opaque (alpha 255) so dots show.
	if s.Color().A != 255 {
		t.Errorf("default color must be opaque, got A=%d", s.Color().A)
	}
}

// TestSetBusyTogglesAnimationState verifies the heartbeat Animation
// transitions as IsBusy() flips. We check state transitions on the
// spinner's own Animation (s.anim) rather than HasActiveAnimations()
// — the latter is global state polluted by other tests' lingering
// animations, so a global assertion would be flaky in parallel runs.
func TestSetBusyTogglesAnimationState(t *testing.T) {
	s := NewSpinner() // busy=true at construction
	defer s.SetBusy(false)
	if s.anim == nil || s.anim.State() != AnimRunning {
		t.Errorf("new busy spinner must have a running animation; got %v", animState(s))
	}
	s.SetBusy(false)
	if s.anim != nil {
		t.Errorf("idle spinner must clear s.anim, got %v", s.anim.State())
	}
	s.SetBusy(true)
	if s.anim == nil || s.anim.State() != AnimRunning {
		t.Errorf("re-busy spinner must re-arm the animation; got %v", animState(s))
	}
}

func animState(s *Spinner) interface{} {
	if s.anim == nil {
		return "<nil>"
	}
	return s.anim.State()
}

// TestSetBusyIdempotent: calling SetBusy with the same value must
// not allocate a new Animation. We can't directly observe
// allocations without a benchmark, but we can confirm the spinner's
// internal anim pointer is the same object before and after a
// no-op SetBusy.
func TestSetBusyIdempotent(t *testing.T) {
	s := NewSpinner()
	first := s.anim
	s.SetBusy(true) // already busy, should be no-op
	if s.anim != first {
		t.Errorf("redundant SetBusy(true) should not replace the animation")
	}
	s.SetBusy(false)
}

// TestSetDotCountClamps locks the floor at 3. Below 3, the visual
// collapses to a single dot which defeats the spinner pattern.
func TestSetDotCountClamps(t *testing.T) {
	s := NewSpinner()
	defer s.SetBusy(false)
	for _, n := range []int{0, 1, 2} {
		s.SetDotCount(n)
		if s.DotCount() != 3 {
			t.Errorf("SetDotCount(%d) → %d, want clamp to 3", n, s.DotCount())
		}
	}
	s.SetDotCount(12)
	if s.DotCount() != 12 {
		t.Errorf("SetDotCount(12) → %d, want 12", s.DotCount())
	}
}

// TestSetCycleDurationFallback: zero or negative duration would
// divide-by-zero in Draw's phase calculation. Setter must coerce.
func TestSetCycleDurationFallback(t *testing.T) {
	s := NewSpinner()
	defer s.SetBusy(false)
	for _, d := range []time.Duration{0, -1, -time.Second} {
		s.SetCycleDuration(d)
		if s.CycleDuration() != time.Second {
			t.Errorf("SetCycleDuration(%v) → %v, want 1s", d, s.CycleDuration())
		}
	}
	s.SetCycleDuration(2 * time.Second)
	if s.CycleDuration() != 2*time.Second {
		t.Errorf("SetCycleDuration(2s) → %v", s.CycleDuration())
	}
}

// TestSpinnerSizeHints documents the default footprint.
func TestSpinnerSizeHints(t *testing.T) {
	s := NewSpinner()
	defer s.SetBusy(false)
	h := s.SizeHints()
	if h.Width != 24 || h.Height != 24 {
		t.Errorf("SizeHints = %v, want 24×24", h)
	}
}

// TestDrawNotBusyIsNoop: a non-busy spinner must not paint
// anything. We can't easily intercept the painter without a real
// render target, but we can at least confirm Draw with a nil-
// equivalent painter doesn't panic.
func TestDrawNotBusyIsNoop(t *testing.T) {
	s := NewSpinner()
	s.SetBusy(false)
	s.SetSize(24, 24)
	// recorderPainter sits below in the helper; if Draw skipped
	// correctly it leaves zero ops recorded.
	rec := newSpinnerRecorder()
	s.Draw(rec)
	if rec.fillCount != 0 {
		t.Errorf("Draw(!busy) should not Fill anything; got %d fills", rec.fillCount)
	}
}

// TestDrawBusyEmitsExpectedDotCount: a busy 8-dot spinner should
// issue 8 Fill calls — one per dot. This locks the rendering loop
// against a regression that drops or duplicates dots.
func TestDrawBusyEmitsExpectedDotCount(t *testing.T) {
	s := NewSpinner()
	defer s.SetBusy(false)
	s.SetSize(24, 24)
	rec := newSpinnerRecorder()
	s.Draw(rec)
	if rec.fillCount != 8 {
		t.Errorf("Draw(busy, 8 dots) issued %d Fills, want 8", rec.fillCount)
	}
	// Each Fill must be paired with a SetBrush1 — otherwise the
	// previous brush colour leaks into the dot. A spinner with all
	// dots the same colour wouldn't animate visually.
	if rec.setBrushCount != 8 {
		t.Errorf("Draw(busy) issued %d SetBrush1 calls, want 8", rec.setBrushCount)
	}
}

// TestDrawSmallSize protects against divide-by-zero / negative-radius
// when the widget is sized to 0×0 or 1×1. With r ≤ 0, Draw returns
// early; with r tiny, dot radius clamps to 1. Either way it must
// not panic.
func TestDrawSmallSize(t *testing.T) {
	for _, dim := range []float64{0, 1, 2} {
		s := NewSpinner()
		s.SetSize(dim, dim)
		rec := newSpinnerRecorder()
		s.Draw(rec) // must not panic
		s.SetBusy(false)
		_ = rec
	}
}

// spinnerRecorder is a minimal paint.Painter that counts Fill /
// SetBrush1 calls so we can assert on the dot-rendering loop.
// Implements only the methods Spinner.Draw uses — the rest panic
// (which is what we want if Spinner picks up a new draw call we
// haven't observed).
type spinnerRecorder struct {
	paint.Painter
	fillCount     int
	setBrushCount int
}

func newSpinnerRecorder() *spinnerRecorder {
	return &spinnerRecorder{Painter: nopPainter{}}
}

func (r *spinnerRecorder) Arc(xc, yc, radius, angle1, angle2 float64) {}
func (r *spinnerRecorder) SetBrush1(c paint.Color)                    { r.setBrushCount++ }
func (r *spinnerRecorder) Fill()                                      { r.fillCount++ }

// nopPainter satisfies paint.Painter with no-op stubs for every
// method Spinner doesn't use. The recorder embeds it so we don't
// have to redeclare every method on the recorder side.
type nopPainter struct{ paint.Painter }
