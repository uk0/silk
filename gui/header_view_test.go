package gui

import "testing"

// callLogicSection invokes LogicSection and fails the test if it panics,
// returning the section it produced for the given logic index.
func callLogicSection(t *testing.T, hv *HeaderView, sid int) (sec HeaderViewSection) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LogicSection(%d) panicked: %v", sid, r)
		}
	}()
	sec = hv.LogicSection(sid)
	return
}

// newHeaderViewWithSections builds a HeaderView and populates its sections
// directly (no model needed) so LogicSection lookups can be exercised.
func newHeaderViewWithSections(secs ...*HeaderViewSection) *HeaderView {
	hv := NewHeaderView()
	hv.sections = secs
	return hv
}

// TestLogicSectionValidIndex: a logic index that exists returns that section.
func TestHeaderViewLogicSectionValidIndex(t *testing.T) {
	hv := newHeaderViewWithSections(
		&HeaderViewSection{LogicIndex: 5, VisualIndex: 0, Offset: 0, Size: 80},
		&HeaderViewSection{LogicIndex: 9, VisualIndex: 1, Offset: 80, Size: 120},
	)

	sec := callLogicSection(t, hv, 9)
	if sec.LogicIndex != 9 {
		t.Errorf("LogicSection(9).LogicIndex = %d, want 9", sec.LogicIndex)
	}
	if sec.Size != 120 {
		t.Errorf("LogicSection(9).Size = %v, want 120", sec.Size)
	}
	if sec.Offset != 80 {
		t.Errorf("LogicSection(9).Offset = %v, want 80", sec.Offset)
	}
}

// TestLogicSectionUnknownIndexReturnsSentinel: a logic index that is present
// in no section returns the zero-value sentinel without panicking.
func TestHeaderViewLogicSectionUnknownIndexReturnsSentinel(t *testing.T) {
	hv := newHeaderViewWithSections(
		&HeaderViewSection{LogicIndex: 5, VisualIndex: 0, Offset: 0, Size: 80},
		&HeaderViewSection{LogicIndex: 9, VisualIndex: 1, Offset: 80, Size: 120},
	)

	sec := callLogicSection(t, hv, 7) // 7 is between existing indices but absent
	if (sec != HeaderViewSection{}) {
		t.Errorf("LogicSection(7) = %+v, want zero-value sentinel", sec)
	}
}

// TestLogicSectionOutOfRangeReturnsSentinel: an index past the end of any
// section returns the zero-value sentinel without panicking.
func TestHeaderViewLogicSectionOutOfRangeReturnsSentinel(t *testing.T) {
	hv := newHeaderViewWithSections(
		&HeaderViewSection{LogicIndex: 0, VisualIndex: 0, Offset: 0, Size: 80},
		&HeaderViewSection{LogicIndex: 1, VisualIndex: 1, Offset: 80, Size: 120},
	)

	sec := callLogicSection(t, hv, 1000)
	if (sec != HeaderViewSection{}) {
		t.Errorf("LogicSection(1000) = %+v, want zero-value sentinel", sec)
	}
}

// TestLogicSectionEmptyReturnsSentinel: with no sections at all, any lookup
// returns the zero-value sentinel without panicking.
func TestHeaderViewLogicSectionEmptyReturnsSentinel(t *testing.T) {
	hv := NewHeaderView()

	sec := callLogicSection(t, hv, 0)
	if (sec != HeaderViewSection{}) {
		t.Errorf("LogicSection(0) on empty header = %+v, want zero-value sentinel", sec)
	}
}
