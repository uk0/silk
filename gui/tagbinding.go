package gui

import (
	"fmt"

	"github.com/uk0/silk/paint"
)

// Tag-to-widget-property binding for SCADA / 组态 screens.
//
// A tag binding subscribes to a real-time tag's value changes and drives one
// widget property: a value (via a setter func), a color (threshold-based, for
// alarm coloring), visibility, or enabled state. Codegen emitted from the
// designer's binding sheet calls these helpers.
//
// # The BindableTag seam
//
// The real-time tag model lives in a separate, UI-agnostic package (scada /
// core) that MUST NOT import gui, so gui cannot import it back without an
// import cycle — and while that package is being built in parallel it is not
// even present in this build. gui therefore binds against a MINIMAL LOCAL
// interface the tag satisfies structurally, not against a concrete type:
//
//	type BindableTag interface {
//	    Subscribe(func(interface{})) func() // returns an unsubscribe func
//	    Value() interface{}                 // latest sample
//	}
//
// The concrete tag exposes typed methods — e.g. scada.Tag has
// `Subscribe(func(scada.Value)) scada.CancelFunc` and `Value() scada.Value`.
// The scada->gui bridge (gui/tagbind.go, built in parallel) adapts it by
// unwrapping each scada.Value into its `.Raw` interface{} before calling the
// gui-side subscriber, so the value flows here as a plain float64 / bool /
// int64 / string. As a convenience the coercion helpers below ALSO accept a
// whole scada.Value structurally (it exposes Float()/Bool()/String()), so the
// bridge may pass either the raw payload or the full sample.
//
// # Threading
//
// Tag notifications fire on whatever goroutine calls the tag's Publish/SetValue
// — typically a driver poll goroutine, NOT the UI thread. Every binding here is
// pure glue: the HOST is responsible for marshalling the setter onto the main /
// event-loop thread with gui.Post before touching any widget. A designer-
// generated setter is therefore expected to look like:
//
//	binding := gui.BindTagValue(tag, func(v interface{}) {
//	    gui.Post(func() { tank.SetLevel(gui.TagFloat(v)) })
//	})
//	defer binding() // unsubscribe when the screen closes
//
// The bindings deliberately do NOT call gui.Post themselves: the setter's
// signature is opaque, and pushing the raw value straight through keeps them
// usable from headless tests and lets the caller add eased motion
// (gui.NewAnimation) inside the posted closure.

// BindableTag is the structural contract a real-time tag satisfies. Defined
// locally so gui need not import the (UI-agnostic, parallel) tag package. See
// the file header for the seam and the scada->gui bridge that adapts the
// concrete tag onto it.
type BindableTag interface {
	// Subscribe registers fn for every future sample and returns an
	// idempotent unsubscribe func.
	Subscribe(func(interface{})) func()
	// Value returns the latest sample.
	Value() interface{}
}

// ColorRange maps a numeric interval [Min, Max] to a color. Used to paint
// alarm bands (LoLo/Lo/normal/Hi/HiHi) from a tag's engineering value.
type ColorRange struct {
	Min, Max float64
	Color    paint.Color
}

// PickThresholdColor returns the color of the first ColorRange whose closed
// interval [Min, Max] contains value, and ok=true. If value falls below every
// range or above every range (or ranges is empty) it returns ok=false and the
// zero Color, so callers can leave the widget's color unchanged rather than
// blank it. Ranges are scanned in order, so overlapping bands resolve to the
// first match.
func PickThresholdColor(ranges []ColorRange, value float64) (paint.Color, bool) {
	for _, r := range ranges {
		if value >= r.Min && value <= r.Max {
			return r.Color, true
		}
	}
	return paint.Color{}, false
}

// BindTagValue subscribes setter to tag and drives it with every new sample.
// It also invokes setter once with the current value immediately (in addition
// to any prime the tag itself performs on Subscribe), so a freshly-bound widget
// paints live data at once. The returned func unsubscribes; it is idempotent.
//
// setter receives the raw tag payload as interface{} — use TagFloat / TagBool /
// TagString to coerce. The setter may fire from a poll goroutine: the host MUST
// marshal any widget mutation onto the UI thread via gui.Post (see file header).
func BindTagValue(tag BindableTag, setter func(interface{})) func() {
	cancel := tag.Subscribe(setter)
	setter(tag.Value())
	return cancel
}

// ThresholdColorBinding subscribes to tag, coerces each sample to float64, and
// calls setColor with the matching band's color (see PickThresholdColor). It
// primes setColor with the current value immediately. When a sample falls
// outside every range setColor is NOT called, leaving the last color in place.
// The returned func unsubscribes; it is idempotent.
//
// setColor may fire from a poll goroutine: the host MUST marshal the widget
// repaint onto the UI thread via gui.Post (see file header).
func ThresholdColorBinding(tag BindableTag, ranges []ColorRange, setColor func(paint.Color)) func() {
	apply := func(v interface{}) {
		if c, ok := PickThresholdColor(ranges, TagFloat(v)); ok {
			setColor(c)
		}
	}
	cancel := tag.Subscribe(apply)
	apply(tag.Value())
	return cancel
}

// BindTagVisibility subscribes to tag and drives a boolean visibility setter
// from each sample's truthiness (TagBool). It primes with the current value
// immediately. The returned func unsubscribes; it is idempotent.
//
// setVisible may fire from a poll goroutine: the host MUST marshal the widget
// mutation onto the UI thread via gui.Post (see file header).
func BindTagVisibility(tag BindableTag, setVisible func(bool)) func() {
	apply := func(v interface{}) { setVisible(TagBool(v)) }
	cancel := tag.Subscribe(apply)
	apply(tag.Value())
	return cancel
}

// BindTagEnabled subscribes to tag and drives a boolean enabled setter from
// each sample's truthiness (TagBool). It primes with the current value
// immediately. The returned func unsubscribes; it is idempotent.
//
// setEnabled may fire from a poll goroutine: the host MUST marshal the widget
// mutation onto the UI thread via gui.Post (see file header).
func BindTagEnabled(tag BindableTag, setEnabled func(bool)) func() {
	apply := func(v interface{}) { setEnabled(TagBool(v)) }
	cancel := tag.Subscribe(apply)
	apply(tag.Value())
	return cancel
}

// TagFloat coerces a raw tag payload to float64 for the common cases
// (float64/float32, int/int64/int32, bool -> 1/0). A whole scada.Value is
// accepted structurally via its Float() method. Anything else yields 0.
func TagFloat(v interface{}) float64 {
	if f, ok := v.(interface{ Float() float64 }); ok {
		return f.Float()
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	}
	return 0
}

// TagBool coerces a raw tag payload to bool: a bool directly, otherwise any
// nonzero numeric. A whole scada.Value is accepted structurally via its Bool()
// method.
func TagBool(v interface{}) bool {
	if b, ok := v.(interface{ Bool() bool }); ok {
		return b.Bool()
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return TagFloat(v) != 0
}

// TagString coerces a raw tag payload to string: a string directly, nil to "",
// a fmt.Stringer (including a whole scada.Value) via String(), otherwise
// fmt.Sprint.
func TagString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	}
	if s, ok := v.(interface{ String() string }); ok {
		return s.String()
	}
	return fmt.Sprint(v)
}
