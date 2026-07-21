package graph

import "testing"

// recordingPropView is a minimal prop.IPropertyView stand-in that records the
// calls GraphView makes into it. It lets the property-view injection be tested
// without a real prop.PropertySheet, which needs a GL surface to lay out.
type recordingPropView struct {
	clearCalls int
	clearOwner interface{}
	bindCalls  int
}

func (r *recordingPropView) Clear(owner interface{}) {
	r.clearCalls++
	r.clearOwner = owner
}

func (r *recordingPropView) Bind(objs []interface{}, cfgName string, owner interface{}) {
	r.bindCalls++
}

// TestSetGetPropertyViewInjected: an injected sheet is returned by
// GetPropertyView, taking precedence over the frame tool-view lookup.
func TestSetGetPropertyViewInjected(t *testing.T) {
	v := NewView()

	sheet := &recordingPropView{}
	v.SetPropertyView(sheet)

	if got := v.GetPropertyView(); got != sheet {
		t.Fatalf("GetPropertyView returned %v, want the injected sheet", got)
	}
}

// TestSetPropertyViewSwapClearsPrevious: replacing the injected sheet clears the
// outgoing one (so its stale entries for this view don't linger), then returns
// the new sheet; re-injecting the same sheet is a no-op.
func TestSetPropertyViewSwapClearsPrevious(t *testing.T) {
	v := NewView()

	first := &recordingPropView{}
	second := &recordingPropView{}
	v.SetPropertyView(first)
	v.SetPropertyView(second)

	if first.clearCalls != 1 {
		t.Errorf("previous sheet Clear called %d times, want 1", first.clearCalls)
	}
	if got := v.GetPropertyView(); got != second {
		t.Errorf("GetPropertyView returned %v, want the second sheet", got)
	}

	v.SetPropertyView(second)
	if second.clearCalls != 0 {
		t.Errorf("re-injecting the same sheet cleared it %d times, want 0", second.clearCalls)
	}
}

// TestGetPropertyViewNilWhenNoneInjectedNoFrame: with nothing injected and no
// owning frame, GetPropertyView returns nil without panicking (the frame lookup
// is guarded against a nil frame).
func TestGetPropertyViewNilWhenNoneInjectedNoFrame(t *testing.T) {
	v := NewView()
	if got := v.GetPropertyView(); got != nil {
		t.Fatalf("GetPropertyView with no injection and no frame = %v, want nil", got)
	}
}
