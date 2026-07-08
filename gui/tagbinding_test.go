package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

// fakeTag is a minimal in-memory BindableTag for tests. Unlike the real
// scada.Tag it does NOT prime a new subscriber, so the immediate push each
// binding performs is the sole source of the initial callback and can be
// asserted unambiguously. Single-threaded; no locking needed for these tests.
type fakeTag struct {
	cur  interface{}
	subs map[int]func(interface{})
	next int
}

func newFakeTag(initial interface{}) *fakeTag {
	return &fakeTag{cur: initial, subs: map[int]func(interface{}){}}
}

func (f *fakeTag) Value() interface{} { return f.cur }

func (f *fakeTag) Subscribe(fn func(interface{})) func() {
	id := f.next
	f.next++
	f.subs[id] = fn
	return func() { delete(f.subs, id) }
}

// push stores a new sample and fans out to current subscribers.
func (f *fakeTag) push(v interface{}) {
	f.cur = v
	for _, fn := range f.subs {
		fn(v)
	}
}

var _ BindableTag = (*fakeTag)(nil)

func TestTagBindValueImmediateAndOnChange(t *testing.T) {
	tag := newFakeTag(3.0)
	var got []interface{}
	cancel := BindTagValue(tag, func(v interface{}) { got = append(got, v) })
	defer cancel()

	// Fires immediately with the current value.
	if len(got) != 1 || got[0] != 3.0 {
		t.Fatalf("expected immediate prime with 3.0, got %v", got)
	}

	// Fires again on change.
	tag.push(7.0)
	if len(got) != 2 || got[1] != 7.0 {
		t.Fatalf("expected on-change delivery of 7.0, got %v", got)
	}
}

func TestTagBindUnsubscribeStopsDelivery(t *testing.T) {
	tag := newFakeTag(1.0)
	calls := 0
	cancel := BindTagValue(tag, func(v interface{}) { calls++ })

	if calls != 1 {
		t.Fatalf("want 1 call after immediate prime, got %d", calls)
	}

	cancel()
	tag.push(2.0)
	if calls != 1 {
		t.Fatalf("setter fired after unsubscribe: %d calls", calls)
	}

	// Unsubscribe is idempotent — a second call must not panic.
	cancel()
}

func TestTagBindPickThresholdColor(t *testing.T) {
	blue := paint.Color{R: 0, G: 0, B: 255, A: 255}
	green := paint.Color{R: 0, G: 255, B: 0, A: 255}
	red := paint.Color{R: 255, G: 0, B: 0, A: 255}
	ranges := []ColorRange{
		{Min: 0, Max: 20, Color: blue},
		{Min: 20, Max: 80, Color: green},
		{Min: 80, Max: 100, Color: red},
	}

	// Below every range: no match.
	if c, ok := PickThresholdColor(ranges, -5); ok {
		t.Errorf("below-all should not match, got %v", c)
	}
	// Inside a range: the band color.
	if c, ok := PickThresholdColor(ranges, 50); !ok || c != green {
		t.Errorf("in-range want green ok, got %v ok=%v", c, ok)
	}
	// Above every range: no match.
	if c, ok := PickThresholdColor(ranges, 200); ok {
		t.Errorf("above-all should not match, got %v", c)
	}
	// First match wins on an overlapping boundary (20 is in both bands).
	if c, ok := PickThresholdColor(ranges, 20); !ok || c != blue {
		t.Errorf("boundary want first-match blue, got %v ok=%v", c, ok)
	}
	// Empty ranges never match.
	if _, ok := PickThresholdColor(nil, 5); ok {
		t.Error("empty ranges should not match")
	}
}

func TestTagBindThresholdColorBinding(t *testing.T) {
	blue := paint.Color{R: 0, G: 0, B: 255, A: 255}
	green := paint.Color{R: 0, G: 255, B: 0, A: 255}
	red := paint.Color{R: 255, G: 0, B: 0, A: 255}
	ranges := []ColorRange{
		{Min: 0, Max: 20, Color: blue},
		{Min: 21, Max: 80, Color: green},
		{Min: 81, Max: 100, Color: red},
	}

	tag := newFakeTag(50.0) // in the green band
	var got paint.Color
	calls := 0
	cancel := ThresholdColorBinding(tag, ranges, func(c paint.Color) { got = c; calls++ })
	defer cancel()

	// Primed immediately with the in-range color.
	if calls != 1 || got != green {
		t.Fatalf("prime want green, got %v calls=%d", got, calls)
	}

	// Above every range: setColor is not called, last color stays.
	tag.push(200.0)
	if calls != 1 {
		t.Fatalf("above-all must not call setColor, calls=%d", calls)
	}

	// Below every range: setColor is not called.
	tag.push(-5.0)
	if calls != 1 {
		t.Fatalf("below-all must not call setColor, calls=%d", calls)
	}

	// Back into the blue band: setColor fires with blue.
	tag.push(10.0)
	if calls != 2 || got != blue {
		t.Fatalf("in-range want blue, got %v calls=%d", got, calls)
	}

	// After unsubscribe, no further delivery.
	cancel()
	tag.push(90.0) // would be red
	if calls != 2 || got != blue {
		t.Fatalf("delivery after unsubscribe: got %v calls=%d", got, calls)
	}
}

func TestTagBindVisibilityAndEnabled(t *testing.T) {
	// Visibility follows tag truthiness.
	visTag := newFakeTag(false)
	vis := true
	visCalls := 0
	visCancel := BindTagVisibility(visTag, func(b bool) { vis = b; visCalls++ })
	if visCalls != 1 || vis != false {
		t.Fatalf("visibility prime want false, got %v calls=%d", vis, visCalls)
	}
	visTag.push(true)
	if vis != true {
		t.Fatalf("visibility should follow tag to true, got %v", vis)
	}
	visCancel()
	visTag.push(false)
	if vis != true {
		t.Fatalf("visibility fired after unsubscribe, got %v", vis)
	}

	// Enabled follows tag truthiness, coercing a numeric sample.
	enTag := newFakeTag(0.0)
	en := true
	enCancel := BindTagEnabled(enTag, func(b bool) { en = b })
	defer enCancel()
	if en != false {
		t.Fatalf("enabled prime want false for 0.0, got %v", en)
	}
	enTag.push(1.0)
	if en != true {
		t.Fatalf("enabled want true for 1.0, got %v", en)
	}
}

// fakeValue mimics scada.Value structurally (Float/Bool/String) to prove the
// coercion helpers accept a whole sample struct, not just raw primitives.
type fakeValue struct{}

func (fakeValue) Float() float64 { return 42 }
func (fakeValue) Bool() bool     { return true }
func (fakeValue) String() string { return "sv" }

func TestTagBindCoercionHelpers(t *testing.T) {
	// TagFloat over the common primitive cases.
	if got := TagFloat(int64(5)); got != 5 {
		t.Errorf("TagFloat(int64) = %v, want 5", got)
	}
	if got := TagFloat(float32(2.5)); got != 2.5 {
		t.Errorf("TagFloat(float32) = %v, want 2.5", got)
	}
	if got := TagFloat(true); got != 1 {
		t.Errorf("TagFloat(true) = %v, want 1", got)
	}
	if got := TagFloat("nope"); got != 0 {
		t.Errorf("TagFloat(string) = %v, want 0", got)
	}

	// TagBool.
	if !TagBool(3.0) || TagBool(0.0) || TagBool(false) {
		t.Error("TagBool numeric/false coercion wrong")
	}

	// TagString.
	if got := TagString("hi"); got != "hi" {
		t.Errorf("TagString(string) = %q, want hi", got)
	}
	if got := TagString(nil); got != "" {
		t.Errorf("TagString(nil) = %q, want empty", got)
	}
	if got := TagString(3); got != "3" {
		t.Errorf("TagString(int) = %q, want 3", got)
	}

	// The scada.Value seam: helpers use the struct's methods.
	if got := TagFloat(fakeValue{}); got != 42 {
		t.Errorf("TagFloat(scada.Value) = %v, want 42 via Float()", got)
	}
	if !TagBool(fakeValue{}) {
		t.Error("TagBool(scada.Value) should use Bool()")
	}
	if got := TagString(fakeValue{}); got != "sv" {
		t.Errorf("TagString(scada.Value) = %q, want sv via String()", got)
	}
}
