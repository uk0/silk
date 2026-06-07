package gui

import (
	"reflect"
	"silk/paint"
	"testing"
)

// ---------------------------------------------------------------------------
// Diff gutter marker API (pure, no GL / no Draw)
//
// Markers are keyed 0-based, matching cursorLine / breakpoints / bookmarks.
// These tests exercise the UI/state layer and the pure render helpers only;
// the editor never computes a diff — the host pushes the marker set.
// ---------------------------------------------------------------------------

func TestCodeEditorSetDiffMarkersStoresCopy(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd")

	if got := e.DiffMarkers(); len(got) != 0 {
		t.Fatalf("new editor should have no diff markers, got %v", got)
	}

	in := map[int]DiffMarkerKind{0: DiffMarkerAdded, 2: DiffMarkerModified}
	e.SetDiffMarkers(in)

	want := map[int]DiffMarkerKind{0: DiffMarkerAdded, 2: DiffMarkerModified}
	if got := e.DiffMarkers(); !reflect.DeepEqual(got, want) {
		t.Errorf("DiffMarkers() = %v, want %v", got, want)
	}

	// Mutating the input map after Set must not change internal state.
	in[0] = DiffMarkerRemoved
	in[5] = DiffMarkerAdded
	if got := e.DiffMarkers(); !reflect.DeepEqual(got, want) {
		t.Errorf("mutating input map leaked into editor: DiffMarkers() = %v, want %v", got, want)
	}

	// Mutating the returned map must not change internal state either.
	ret := e.DiffMarkers()
	ret[2] = DiffMarkerRemoved
	delete(ret, 0)
	if got := e.DiffMarkers(); !reflect.DeepEqual(got, want) {
		t.Errorf("mutating returned map leaked into editor: DiffMarkers() = %v, want %v", got, want)
	}
}

func TestCodeEditorSetDiffMarkersDropsNone(t *testing.T) {
	e := NewCodeEditor()
	// DiffMarkerNone entries are meaningless and must be dropped on Set so the
	// stored set stays minimal.
	e.SetDiffMarkers(map[int]DiffMarkerKind{0: DiffMarkerAdded, 1: DiffMarkerNone})
	want := map[int]DiffMarkerKind{0: DiffMarkerAdded}
	if got := e.DiffMarkers(); !reflect.DeepEqual(got, want) {
		t.Errorf("DiffMarkers() = %v, want %v (None must be dropped)", got, want)
	}
}

func TestCodeEditorSetDiffMarkersNilClears(t *testing.T) {
	e := NewCodeEditor()
	e.SetDiffMarkers(map[int]DiffMarkerKind{0: DiffMarkerAdded})
	if len(e.DiffMarkers()) != 1 {
		t.Fatalf("setup: want 1 marker")
	}
	e.SetDiffMarkers(nil)
	if got := e.DiffMarkers(); len(got) != 0 {
		t.Errorf("SetDiffMarkers(nil) should clear, got %v", got)
	}
}

func TestCodeEditorClearDiffMarkers(t *testing.T) {
	e := NewCodeEditor()
	e.SetDiffMarkers(map[int]DiffMarkerKind{0: DiffMarkerAdded, 3: DiffMarkerRemoved})
	if len(e.DiffMarkers()) != 2 {
		t.Fatalf("setup: want 2 markers, got %v", e.DiffMarkers())
	}
	e.ClearDiffMarkers()
	if got := e.DiffMarkers(); len(got) != 0 {
		t.Errorf("after ClearDiffMarkers, DiffMarkers() = %v, want empty", got)
	}
}

func TestCodeEditorSetDiffFromLines(t *testing.T) {
	e := NewCodeEditor()
	e.SetDiffFromLines([]int{0, 1}, []int{4}, []int{7, 9})
	want := map[int]DiffMarkerKind{
		0: DiffMarkerAdded,
		1: DiffMarkerAdded,
		4: DiffMarkerModified,
		7: DiffMarkerRemoved,
		9: DiffMarkerRemoved,
	}
	if got := e.DiffMarkers(); !reflect.DeepEqual(got, want) {
		t.Errorf("SetDiffFromLines built %v, want %v", got, want)
	}
}

// TestCodeEditorSetDiffFromLinesPrecedence documents and pins the overlap rule:
// Removed > Modified > Added. A line listed in multiple buckets takes the
// highest-precedence kind.
func TestCodeEditorSetDiffFromLinesPrecedence(t *testing.T) {
	e := NewCodeEditor()
	// line 0: added + modified  -> Modified wins over Added
	// line 1: added + removed   -> Removed wins over Added
	// line 2: modified + removed -> Removed wins over Modified
	// line 3: all three          -> Removed wins
	e.SetDiffFromLines(
		[]int{0, 1, 3},
		[]int{0, 2, 3},
		[]int{1, 2, 3},
	)
	want := map[int]DiffMarkerKind{
		0: DiffMarkerModified,
		1: DiffMarkerRemoved,
		2: DiffMarkerRemoved,
		3: DiffMarkerRemoved,
	}
	if got := e.DiffMarkers(); !reflect.DeepEqual(got, want) {
		t.Errorf("precedence: got %v, want %v (Removed > Modified > Added)", got, want)
	}
}

func TestDiffMarkerColor(t *testing.T) {
	cases := []struct {
		kind     DiffMarkerKind
		wantDraw bool
	}{
		{DiffMarkerNone, false},
		{DiffMarkerAdded, true},
		{DiffMarkerModified, true},
		{DiffMarkerRemoved, true},
		{DiffMarkerKind(99), false}, // unknown kind -> no draw
	}
	seen := map[string]DiffMarkerKind{}
	for _, c := range cases {
		col, draw := diffMarkerColor(c.kind)
		if draw != c.wantDraw {
			t.Errorf("diffMarkerColor(%d) draw = %v, want %v", c.kind, draw, c.wantDraw)
		}
		if !draw {
			if (col != paint.Color{}) {
				t.Errorf("diffMarkerColor(%d) non-draw should return zero color, got %v", c.kind, col)
			}
			continue
		}
		// Drawable kinds must each map to a distinct, opaque-ish color.
		if col.A == 0 {
			t.Errorf("diffMarkerColor(%d) returned fully transparent color", c.kind)
		}
		if prev, ok := seen[col.String()]; ok {
			t.Errorf("diffMarkerColor(%d) collides with kind %d on color %v", c.kind, prev, col)
		}
		seen[col.String()] = c.kind
	}
}

func TestVisibleDiffMarkers(t *testing.T) {
	markers := map[int]DiffMarkerKind{
		1:  DiffMarkerAdded,
		3:  DiffMarkerModified,
		5:  DiffMarkerRemoved,
		8:  DiffMarkerAdded,
		12: DiffMarkerModified,
	}

	// Viewport [3, 8] inclusive: only lines 3, 5, 8, sorted ascending.
	got := visibleDiffMarkers(markers, 3, 8)
	want := []lineMarker{
		{line: 3, kind: DiffMarkerModified},
		{line: 5, kind: DiffMarkerRemoved},
		{line: 8, kind: DiffMarkerAdded},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("visibleDiffMarkers(3,8) = %v, want %v", got, want)
	}

	// Boundaries are inclusive on both ends.
	if got := visibleDiffMarkers(markers, 1, 1); !reflect.DeepEqual(got, []lineMarker{{line: 1, kind: DiffMarkerAdded}}) {
		t.Errorf("visibleDiffMarkers(1,1) = %v, want [{1 Added}]", got)
	}

	// Out-of-range window yields nothing.
	if got := visibleDiffMarkers(markers, 100, 200); got != nil {
		t.Errorf("visibleDiffMarkers(100,200) = %v, want nil", got)
	}

	// Nil / empty map yields nil.
	if got := visibleDiffMarkers(nil, 0, 10); got != nil {
		t.Errorf("visibleDiffMarkers(nil) = %v, want nil", got)
	}

	// DiffMarkerNone entries are skipped even when in range.
	none := map[int]DiffMarkerKind{2: DiffMarkerNone, 4: DiffMarkerAdded}
	if got := visibleDiffMarkers(none, 0, 10); !reflect.DeepEqual(got, []lineMarker{{line: 4, kind: DiffMarkerAdded}}) {
		t.Errorf("visibleDiffMarkers with None entry = %v, want [{4 Added}]", got)
	}
}
