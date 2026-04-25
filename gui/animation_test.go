package gui

import (
	"math"
	"testing"
	"time"
)

func TestEasingFunctionBoundaries(t *testing.T) {
	easings := []struct {
		name string
		fn   EaseFunc
	}{
		{"Linear", EaseLinear},
		{"InQuad", EaseInQuad},
		{"OutQuad", EaseOutQuad},
		{"InOutQuad", EaseInOutQuad},
		{"InCubic", EaseInCubic},
		{"OutCubic", EaseOutCubic},
		{"InOutCubic", EaseInOutCubic},
		{"OutBounce", EaseOutBounce},
		{"InBounce", EaseInBounce},
		{"InBack", EaseInBack},
		{"OutBack", EaseOutBack},
		{"InOutBack", EaseInOutBack},
		{"InElastic", EaseInElastic},
		{"OutElastic", EaseOutElastic},
	}

	for _, e := range easings {
		t.Run(e.name, func(t *testing.T) {
			v0 := e.fn(0)
			v1 := e.fn(1)

			// f(0) should be 0 (or very close)
			if math.Abs(v0) > 1e-10 {
				t.Errorf("f(0) = %f, want 0", v0)
			}
			// f(1) should be 1 (or very close)
			if math.Abs(v1-1) > 1e-10 {
				t.Errorf("f(1) = %f, want 1", v1)
			}
		})
	}
}

func TestEaseLinearMidpoint(t *testing.T) {
	v := EaseLinear(0.5)
	if v != 0.5 {
		t.Errorf("EaseLinear(0.5) = %f, want 0.5", v)
	}
}

func TestEaseInQuadMonotonic(t *testing.T) {
	prev := 0.0
	for i := 1; i <= 100; i++ {
		tt := float64(i) / 100.0
		v := EaseInQuad(tt)
		if v < prev {
			t.Errorf("EaseInQuad not monotonic at t=%f: %f < %f", tt, v, prev)
		}
		prev = v
	}
}

func TestEaseOutQuadMonotonic(t *testing.T) {
	prev := 0.0
	for i := 1; i <= 100; i++ {
		tt := float64(i) / 100.0
		v := EaseOutQuad(tt)
		if v < prev {
			t.Errorf("EaseOutQuad not monotonic at t=%f: %f < %f", tt, v, prev)
		}
		prev = v
	}
}

func TestAnimationInitialValue(t *testing.T) {
	anim := NewAnimation(0, 100, time.Second)
	anim.SetEase(EaseLinear)

	// Before starting, current is 0
	if anim.Value() != 0 {
		t.Errorf("initial Value() = %f, want 0", anim.Value())
	}
	if anim.State() != AnimIdle {
		t.Errorf("initial State() = %d, want AnimIdle", anim.State())
	}
}

func TestAnimationSetEaseChaining(t *testing.T) {
	anim := NewAnimation(0, 1, time.Second).
		SetEase(EaseLinear).
		SetLoop(true).
		SetReverse(true)

	if anim == nil {
		t.Fatal("chained methods returned nil")
	}
}

func TestAnimationGroup(t *testing.T) {
	a1 := NewAnimation(0, 10, time.Millisecond*100)
	a2 := NewAnimation(0, 20, time.Millisecond*100)

	group := NewAnimationGroup(AnimParallel).Add(a1).Add(a2)
	if group == nil {
		t.Fatal("NewAnimationGroup returned nil")
	}
}

func TestAnimationStopSetsState(t *testing.T) {
	anim := NewAnimation(0, 100, time.Second)
	anim.Stop()
	if anim.State() != AnimDone {
		t.Errorf("after Stop(), State() = %d, want AnimDone", anim.State())
	}
}

func TestAnimationPauseResumeIdle(t *testing.T) {
	anim := NewAnimation(0, 100, time.Second)
	// Pause on an idle animation does nothing
	anim.Pause()
	if anim.State() != AnimIdle {
		t.Errorf("Pause on idle should keep state AnimIdle, got %d", anim.State())
	}
	// Resume on idle does nothing
	anim.Resume()
	if anim.State() != AnimIdle {
		t.Errorf("Resume on idle should keep state AnimIdle, got %d", anim.State())
	}
}

func TestHasActiveAnimationsEmpty(t *testing.T) {
	// Reset the global manager's state for this test
	origAnims := animManager.animations
	origGroups := animManager.groups
	animManager.animations = nil
	animManager.groups = nil
	defer func() {
		animManager.animations = origAnims
		animManager.groups = origGroups
	}()

	if HasActiveAnimations() {
		t.Error("should return false when no animations active")
	}
}

func TestAnimationTickCompletesLinear(t *testing.T) {
	anim := NewAnimation(0, 100, time.Millisecond*10)
	anim.SetEase(EaseLinear)
	anim.state = AnimRunning
	anim.start = time.Now().Add(-time.Second) // started 1 second ago

	done := anim.tick()
	if !done {
		t.Error("tick should report done for animation started long ago")
	}
	if anim.State() != AnimDone {
		t.Errorf("state = %d, want AnimDone", anim.State())
	}
	if math.Abs(anim.Value()-100) > 0.01 {
		t.Errorf("final value = %f, want 100", anim.Value())
	}
}

func TestAnimationOnDoneCallback(t *testing.T) {
	called := false
	anim := NewAnimation(0, 1, time.Millisecond*10)
	anim.SetEase(EaseLinear)
	anim.OnDone(func() { called = true })

	anim.state = AnimRunning
	anim.start = time.Now().Add(-time.Second)
	anim.tick()

	if !called {
		t.Error("OnDone callback was not called")
	}
}

func TestAnimationOnUpdateCallback(t *testing.T) {
	var lastVal float64
	anim := NewAnimation(0, 100, time.Millisecond*10)
	anim.SetEase(EaseLinear)
	anim.OnUpdate(func(v float64) { lastVal = v })

	anim.state = AnimRunning
	anim.start = time.Now().Add(-time.Second)
	anim.tick()

	if lastVal != 100 {
		t.Errorf("OnUpdate received %f, want 100", lastVal)
	}
}
