package ged

import (
	"testing"

	"github.com/uk0/silk/prop"
)

// recordingPropList captures AddProperty calls and satisfies prop.IPropertyList,
// so it can stand in for the real PropertySheet when driving EnumProperties and
// the mark-dirty adapter without a GL surface.
type recordingPropList struct {
	entries []recordedProp
}

type recordedProp struct {
	id  string
	get interface{}
	set interface{}
}

func (r *recordingPropList) AddProperty(id string, get, set interface{}) (*prop.PropertyItem, bool) {
	r.entries = append(r.entries, recordedProp{id: id, get: get, set: set})
	return nil, true
}

func (r *recordingPropList) find(id string) (recordedProp, bool) {
	for _, e := range r.entries {
		if e.id == id {
			return e, true
		}
	}
	return recordedProp{}, false
}

// TestWrapSetterMarkDirty: the wrapped setter keeps its func(T) type, calls the
// original setter, then fires markDirty.
func TestWrapSetterMarkDirty(t *testing.T) {
	var got string
	orig := func(s string) { got = s }
	dirty := false

	wrapped := wrapSetterMarkDirty(orig, func() { dirty = true })
	fn, ok := wrapped.(func(string))
	if !ok {
		t.Fatalf("wrapped setter has type %T, want func(string)", wrapped)
	}
	fn("hello")

	if got != "hello" {
		t.Errorf("original setter got %q, want %q", got, "hello")
	}
	if !dirty {
		t.Error("markDirty was not called by the wrapped setter")
	}
}

// TestWrapSetterMarkDirtyPassThrough: a nil setter, a non-setter shape, and a nil
// markDirty are all returned unchanged, so the property sheet's get/set type
// checks still see the original function.
func TestWrapSetterMarkDirtyPassThrough(t *testing.T) {
	if got := wrapSetterMarkDirty(nil, func() {}); got != nil {
		t.Errorf("nil setter wrapped to %v, want nil", got)
	}
	getter := func() string { return "" } // 0-in/1-out is not a setter shape
	if got := wrapSetterMarkDirty(getter, func() {}); got == nil {
		t.Error("non-setter func was dropped; want returned unchanged")
	}
	setter := func(int) {}
	if got := wrapSetterMarkDirty(setter, nil); got == nil {
		t.Error("nil markDirty dropped the setter; want returned unchanged")
	}
}

// TestMarkDirtyPropertyListWrapsSetters: the adapter forwards id + get to the
// underlying list and installs a wrapped setter that fires markDirty when the
// sheet invokes it.
func TestMarkDirtyPropertyListWrapsSetters(t *testing.T) {
	rec := &recordingPropList{}
	dirty := false
	adapter := newMarkDirtyPropertyList(rec, func() { dirty = true })

	var setVal bool
	adapter.AddProperty("可见", func() bool { return false }, func(b bool) { setVal = b })

	e, ok := rec.find("可见")
	if !ok {
		t.Fatal(`property "可见" was not forwarded to the underlying list`)
	}
	fn, ok := e.set.(func(bool))
	if !ok {
		t.Fatalf("forwarded setter has type %T, want func(bool)", e.set)
	}
	fn(true)
	if !setVal {
		t.Error("wrapped setter did not call the original setter")
	}
	if !dirty {
		t.Error("wrapped setter did not fire markDirty")
	}
}

// TestFakeWidgetEmbeddedSetterRepaints: enumerating a FakeWidget's properties
// routes the embedded widget's setters through the mark-dirty adapter, so editing
// one (gui.Button's "文本") from the sheet repaints the designer preview. The
// FakeWidget's own name setter ("控件名称") already marks dirty on its own and is
// excluded here, so a pass proves the embedded-widget path specifically.
func TestFakeWidgetEmbeddedSetterRepaints(t *testing.T) {
	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}

	rec := &recordingPropList{}
	fake.EnumProperties(rec)

	e, ok := rec.find("文本")
	if !ok {
		t.Fatal(`embedded Button property "文本" was not enumerated`)
	}
	fn, ok := e.set.(func(string))
	if !ok {
		t.Fatalf(`"文本" setter has type %T, want func(string)`, e.set)
	}

	// Clear the dirty flag set during construction so we can observe the
	// setter flip it back.
	fake.pixmapDirty = false
	fn("Renamed")

	if !fake.pixmapDirty {
		t.Error("editing the embedded widget property did not mark the FakeWidget dirty")
	}
}
