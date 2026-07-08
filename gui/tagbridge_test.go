package gui

import (
	"math"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

// TestWrapTagAdaptsCoreTag: WrapTag adapts a real *core.Tag onto BindableTag —
// Value() unwraps core.Value.Raw and Subscribe delivers each raw payload as
// interface{}, then stops on unsubscribe.
func TestWrapTagAdaptsCoreTag(t *testing.T) {
	tag := core.NewTagDB().GetOrCreate("temp", core.Meta{})
	tag.SetValue(42.0)

	bt := WrapTag(tag)

	if got := bt.Value(); got != 42.0 {
		t.Fatalf("WrapTag Value() = %v, want 42.0", got)
	}

	// core.Tag delivers synchronously on the SetValue goroutine (this one), so
	// a plain variable is race-free here.
	var last interface{}
	unsub := bt.Subscribe(func(v interface{}) { last = v })
	defer unsub()

	if last != 42.0 { // Subscribe primes with the current sample
		t.Fatalf("Subscribe prime = %v, want 42.0", last)
	}

	tag.SetValue(true) // raw payload flows through as interface{}
	if last != true {
		t.Fatalf("Subscribe delivered %v, want true", last)
	}

	unsub()
	tag.SetValue("frozen")
	if last != true {
		t.Fatalf("after unsub delivered %v, want still true", last)
	}
}

// TestTagBridgeBindTagDrivesSetter: BindTag primes then drives a setter on every
// change, and freezes after unsubscribe.
func TestTagBridgeBindTagDrivesSetter(t *testing.T) {
	tag := core.NewTagDB().GetOrCreate("flow", core.Meta{})
	tag.SetValue(10.0)

	var got interface{}
	unsub := BindTag(tag, func(v interface{}) { got = v })
	defer unsub()

	if got != 10.0 { // BindTagValue primes with the current value
		t.Fatalf("BindTag prime = %v, want 10.0", got)
	}

	tag.SetValue(20.0)
	if got != 20.0 {
		t.Fatalf("BindTag after change = %v, want 20.0", got)
	}

	unsub()
	tag.SetValue(30.0)
	if got != 20.0 {
		t.Fatalf("BindTag after unsub = %v, want frozen 20.0", got)
	}
}

// TestBindTagAnimatedConverges: a new tag value posts an animation onto the UI
// thread that eases the setter to the tag's Float(). We drain the UI queue to
// start the ease, assert it registered, then drive it to completion and assert
// the setter converged to the target.
func TestBindTagAnimatedConverges(t *testing.T) {
	// Isolate the process-wide UI queue + animation manager from other tests.
	// uiWakeupFn is nil'd so Post stays headless: the window layer may have
	// pointed it at glfw.PostEmptyEvent, a cgo call that faults without an
	// initialized GLFW (see uiqueue.go / resetUIQueue).
	resetBridgeGlobals := func() {
		uiTaskMu.Lock()
		uiTasks = nil
		uiWakeupFn = nil
		uiTaskMu.Unlock()
		animManager.animations = nil
		animManager.groups = nil
	}
	resetBridgeGlobals()
	t.Cleanup(resetBridgeGlobals)

	tag := core.NewTagDB().GetOrCreate("level", core.Meta{})

	var got float64
	unsub := BindTagAnimated(tag, func(f float64) { got = f }, time.Millisecond)
	defer unsub()

	tag.SetValue(80.0)

	// The subscriber only gui.Post-ed the animation setup — nothing has run on
	// the "UI thread" yet, so the setter must still be at its start value.
	if got != 0 {
		t.Fatalf("setter moved before UI drain: got %v, want 0", got)
	}

	drainUITasks() // run the posted closures -> start the ease

	if !HasActiveAnimations() {
		t.Fatal("BindTagAnimated registered no animation on tag change")
	}

	// Drive every live animation to completion deterministically (mirrors
	// animation_test.go: force start into the past, then tick).
	for _, a := range animManager.animations {
		if a.state == AnimRunning {
			a.start = time.Now().Add(-time.Second)
		}
	}
	AnimationTick()

	if math.Abs(got-80.0) > 0.01 {
		t.Fatalf("setter converged to %v, want 80.0", got)
	}
}
