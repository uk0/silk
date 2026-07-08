package gui

import (
	"time"

	"github.com/uk0/silk/core"
)

// core.Tag -> gui.BindableTag bridge for SCADA / 组态 screens.
//
// The real-time tag model (core.Tag) is UI-agnostic: it speaks core.Value and
// core.CancelFunc, and MUST NOT import gui. gui binds against the structural
// BindableTag seam (Subscribe(func(interface{})) func() + Value() interface{},
// see tagbinding.go). Their signatures differ, so this file adapts a concrete
// *core.Tag onto BindableTag by unwrapping each core.Value to its .Raw payload,
// and adds the value-driven animation glue the audit flagged as missing
// (tag-changed -> animate-property).

// tagAdapter adapts a concrete *core.Tag to the gui.BindableTag structural
// interface. core.Tag.Subscribe takes func(core.Value) and returns
// core.CancelFunc; BindableTag wants func(interface{}) returning func(). The
// adapter unwraps each core.Value into its .Raw interface{} before calling the
// gui-side subscriber, so the value flows to the binding layer as a plain
// float64 / bool / int64 / string.
type tagAdapter struct{ t *core.Tag }

// WrapTag adapts a concrete *core.Tag to gui.BindableTag so it can drive the
// binding helpers in tagbinding.go (BindTagValue / ThresholdColorBinding / ...).
func WrapTag(t *core.Tag) BindableTag { return tagAdapter{t: t} }

// Subscribe wraps the gui-side func(interface{}) callback as a core.Subscriber
// that forwards each sample's raw payload. The returned core.CancelFunc is an
// idempotent unsubscribe and satisfies BindableTag's func() return.
func (a tagAdapter) Subscribe(fn func(interface{})) func() {
	return a.t.Subscribe(func(v core.Value) { fn(v.Raw) })
}

// Value returns the tag's latest raw payload (core.Value.Raw).
func (a tagAdapter) Value() interface{} { return a.t.Value().Raw }

// BindTag wraps t and drives setter with every new sample (WrapTag then
// BindTagValue). setter is primed with the current value immediately and
// receives the raw payload as interface{} — coerce with TagFloat / TagBool /
// TagString. The returned func unsubscribes and is idempotent.
//
// setter may fire from the tag's poll goroutine: the host MUST marshal any
// widget mutation onto the UI thread via gui.Post (see tagbinding.go header).
func BindTag(t *core.Tag, setter func(interface{})) func() {
	return BindTagValue(WrapTag(t), setter)
}

// BindTagAnimated subscribes to t and, on every new sample, EASES the displayed
// float from its current value to the tag's new Float() over dur, calling
// setFloat on each animation tick. This is the tag-changed -> animate-property
// glue: a Tank level or Gauge needle sweeps smoothly to the new setpoint
// instead of jumping. A sample that arrives mid-ease re-targets from wherever
// the needle currently is (the previous animation is stopped first), so setFloat
// never sees two animations fighting over the same widget.
//
// Threading. The tag's subscriber fires on whatever goroutine calls
// Publish / SetValue — typically a background driver-poll goroutine, NOT the UI
// thread. Animations touch widget state, which is main-thread-only, so the
// subscriber marshals the animation setup onto the UI thread via gui.Post; the
// per-tick setFloat then runs on the UI thread too, because AnimationTick is
// driven from the event loop. The eased-from value and the live *Animation are
// therefore only ever read/written inside the posted closure and the OnUpdate
// tick — both on the UI thread — so they need no lock.
//
// The returned func unsubscribes and is idempotent.
func BindTagAnimated(t *core.Tag, setFloat func(float64), dur time.Duration) func() {
	// current: the last value handed to setFloat, i.e. what the widget is
	// showing now. Each new sample eases from here. anim: the in-flight ease,
	// stopped and replaced when a newer sample arrives. Both are UI-thread-only
	// (see the Threading note above).
	var current float64
	var anim *Animation
	return t.Subscribe(func(v core.Value) {
		target := v.Float()
		Post(func() {
			if anim != nil {
				anim.Stop() // cancel the in-flight ease; re-target from current
			}
			anim = NewAnimation(current, target, dur).
				OnUpdate(func(val float64) {
					current = val
					setFloat(val)
				})
			anim.Start()
		})
	})
}
