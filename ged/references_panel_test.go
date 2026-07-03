package ged

import (
	"testing"
)

// sampleLocs is a representative set of reference rows: three usages of
// the same symbol across two files, each with a 1-based line, a column
// and a trimmed source-line preview. It exercises the round-trip, the
// label formatting and the hit-test geometry in one fixture.
func sampleLocs() []ReferenceLoc {
	return []ReferenceLoc{
		{File: "ged/foo.go", Line: 12, Col: 5, Preview: "x := Bar()"},
		{File: "ged/foo.go", Line: 30, Col: 1, Preview: "return Bar{}"},
		{File: "core/baz.go", Line: 7, Col: 9, Preview: "_ = Bar"},
	}
}

// TestReferencesSetGetRoundTrip checks SetLocations followed by
// Locations() returns the same rows field-by-field.
func TestReferencesSetGetRoundTrip(t *testing.T) {
	p := NewReferencesPanel()
	in := sampleLocs()
	p.SetLocations(in)

	got := p.Locations()
	if len(got) != len(in) {
		t.Fatalf("Locations() returned %d rows, want %d: %+v", len(got), len(in), got)
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("loc[%d] = %+v, want %+v", i, got[i], in[i])
		}
	}
}

// TestReferencesCopySemantics verifies the panel keeps its own copy: a)
// mutating the input slice AFTER SetLocations does not change internal
// state, and b) mutating the slice returned by Locations() does not
// either.
func TestReferencesCopySemantics(t *testing.T) {
	p := NewReferencesPanel()
	in := sampleLocs()
	p.SetLocations(in)

	// (a) Mutate the caller's slice after handing it in.
	in[0].File = "MUTATED"
	in[0].Line = 999
	if got := p.Locations(); got[0].File != "ged/foo.go" || got[0].Line != 12 {
		t.Errorf("input mutation leaked into panel: row 0 = %+v", got[0])
	}

	// (b) Mutate the slice the panel handed back.
	out := p.Locations()
	out[1].Preview = "MUTATED"
	if got := p.Locations(); got[1].Preview != "return Bar{}" {
		t.Errorf("returned-slice mutation leaked into panel: row 1 = %+v", got[1])
	}
}

// TestReferencesClear verifies Clear empties the list.
func TestReferencesClear(t *testing.T) {
	p := NewReferencesPanel()
	p.SetLocations(sampleLocs())
	p.Clear()
	if got := p.Locations(); len(got) != 0 {
		t.Errorf("Locations() after Clear = %d rows, want 0: %+v", len(got), got)
	}
}

// TestRefRowAtY exercises the pure hit-test helper directly: rows start
// at topOffset, rowH tall; header / out-of-range / degenerate inputs
// yield -1.
func TestRefRowAtY(t *testing.T) {
	const (
		top = 22.0
		rh  = 20.0
		n   = 3
	)
	cases := []struct {
		name string
		y    float64
		want int
	}{
		{"above rows (header)", 10, -1},
		{"top of row 0", top, 0},
		{"middle of row 0", top + rh/2, 0},
		{"middle of row 2", top + 2*rh + rh/2, 2},
		{"last pixel of row 2", top + 3*rh - 0.5, 2},
		{"just past last row", top + 3*rh, -1},
		{"far below", 10000, -1},
	}
	for _, c := range cases {
		if got := refRowAtY(c.y, top, rh, n); got != c.want {
			t.Errorf("%s: refRowAtY(%v,%v,%v,%d) = %d, want %d",
				c.name, c.y, top, rh, n, got, c.want)
		}
	}
	// Degenerate row height must not divide by zero — return -1.
	if got := refRowAtY(50, top, 0, n); got != -1 {
		t.Errorf("refRowAtY with rowH=0 = %d, want -1", got)
	}
	// Empty list: every y is out of range.
	if got := refRowAtY(top+5, top, rh, 0); got != -1 {
		t.Errorf("refRowAtY with count=0 = %d, want -1", got)
	}
}

// TestRefRowLabel checks the "basename:line" formatting drops the dir.
func TestRefRowLabel(t *testing.T) {
	cases := []struct {
		loc  ReferenceLoc
		want string
	}{
		{ReferenceLoc{File: "ged/foo.go", Line: 42}, "foo.go:42"},
		{ReferenceLoc{File: "core/baz.go", Line: 7}, "baz.go:7"},
		{ReferenceLoc{File: "main.go", Line: 1}, "main.go:1"},
	}
	for _, c := range cases {
		if got := refRowLabel(c.loc); got != c.want {
			t.Errorf("refRowLabel(%+v) = %q, want %q", c.loc, got, c.want)
		}
	}
}

// TestReferenceCountLabel checks the header tally text.
func TestReferenceCountLabel(t *testing.T) {
	if got, want := referenceCountLabel(3), "引用 / References (3)"; got != want {
		t.Errorf("referenceCountLabel(3) = %q, want %q", got, want)
	}
	if got, want := referenceCountLabel(0), "引用 / References (0)"; got != want {
		t.Errorf("referenceCountLabel(0) = %q, want %q", got, want)
	}
}

// TestReferencesRowClickActivates drives a click on row 2 through the
// hit-test + signal path (no GL) and checks SigLocationActivated fires
// with the right file, 1-based line and column.
//
// Geometry: sized 300x400, rows start at referencesHeaderH=22 with
// rowHeight=20 and no scroll, so row 2 occupies y in [22+40, 22+60) =
// [62, 82); click the middle.
func TestReferencesRowClickActivates(t *testing.T) {
	p := NewReferencesPanel()
	p.SetSize(300, 400)
	locs := sampleLocs()
	p.SetLocations(locs)

	var (
		gotFile         string
		gotLine, gotCol int
		fired           bool
	)
	p.SigLocationActivated(func(file string, line, col int) {
		gotFile = file
		gotLine = line
		gotCol = col
		fired = true
	})

	y := referencesHeaderH + 2*p.rowHeight + p.rowHeight/2 // 72
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("OnLeftDown did not fire SigLocationActivated")
	}
	want := locs[2]
	if gotFile != want.File || gotLine != want.Line || gotCol != want.Col {
		t.Errorf("SigLocationActivated = (%q,%d,%d), want (%q,%d,%d)",
			gotFile, gotLine, gotCol, want.File, want.Line, want.Col)
	}
}

// TestReferencesHeaderClickNoop verifies a click in the header band does
// not fire the activation signal.
func TestReferencesHeaderClickNoop(t *testing.T) {
	p := NewReferencesPanel()
	p.SetSize(300, 400)
	p.SetLocations(sampleLocs())

	fired := false
	p.SigLocationActivated(func(string, int, int) { fired = true })
	p.OnLeftDown(5, 5) // inside the 22px header
	if fired {
		t.Error("OnLeftDown in header region fired SigLocationActivated")
	}
}
