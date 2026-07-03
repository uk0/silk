package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"testing"
)

// sampleTodoRows is a representative set of marker rows: one of each kind
// across a couple of files, each with a 1-based line and the comment text.
// It exercises the round-trip, the label formatting and the hit-test
// geometry in one fixture.
func sampleTodoRows() []TodoRow {
	return []TodoRow{
		{File: "ged/foo.go", Line: 12, Kind: "TODO", Text: "wire up SetRows"},
		{File: "ged/foo.go", Line: 30, Kind: "FIXME", Text: "off-by-one in scroll clamp"},
		{File: "core/baz.go", Line: 7, Kind: "NOTE", Text: "host converts scanner hits"},
	}
}

// TestTodoSetGetRoundTrip checks SetRows followed by Rows() returns the
// same rows field-by-field.
func TestTodoSetGetRoundTrip(t *testing.T) {
	p := NewTodoPanel()
	in := sampleTodoRows()
	p.SetRows(in)

	got := p.Rows()
	if len(got) != len(in) {
		t.Fatalf("Rows() returned %d rows, want %d: %+v", len(got), len(in), got)
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("row[%d] = %+v, want %+v", i, got[i], in[i])
		}
	}
}

// TestTodoCopySemantics verifies the panel keeps its own copy: a) mutating
// the input slice AFTER SetRows does not change internal state, and b)
// mutating the slice returned by Rows() does not either.
func TestTodoCopySemantics(t *testing.T) {
	p := NewTodoPanel()
	in := sampleTodoRows()
	p.SetRows(in)

	// (a) Mutate the caller's slice after handing it in.
	in[0].File = "MUTATED"
	in[0].Line = 999
	if got := p.Rows(); got[0].File != "ged/foo.go" || got[0].Line != 12 {
		t.Errorf("input mutation leaked into panel: row 0 = %+v", got[0])
	}

	// (b) Mutate the slice the panel handed back.
	out := p.Rows()
	out[1].Text = "MUTATED"
	if got := p.Rows(); got[1].Text != "off-by-one in scroll clamp" {
		t.Errorf("returned-slice mutation leaked into panel: row 1 = %+v", got[1])
	}
}

// TestTodoClear verifies Clear empties the list.
func TestTodoClear(t *testing.T) {
	p := NewTodoPanel()
	p.SetRows(sampleTodoRows())
	p.Clear()
	if got := p.Rows(); len(got) != 0 {
		t.Errorf("Rows() after Clear = %d rows, want 0: %+v", len(got), got)
	}
}

// TestTodoRowAtY exercises the pure hit-test helper directly: rows start at
// topOffset, rowH tall; header / out-of-range / degenerate inputs yield -1.
func TestTodoRowAtY(t *testing.T) {
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
		if got := todoRowAtY(c.y, top, rh, n); got != c.want {
			t.Errorf("%s: todoRowAtY(%v,%v,%v,%d) = %d, want %d",
				c.name, c.y, top, rh, n, got, c.want)
		}
	}
	// Degenerate row height must not divide by zero — return -1.
	if got := todoRowAtY(50, top, 0, n); got != -1 {
		t.Errorf("todoRowAtY with rowH=0 = %d, want -1", got)
	}
	// Empty list: every y is out of range.
	if got := todoRowAtY(top+5, top, rh, 0); got != -1 {
		t.Errorf("todoRowAtY with count=0 = %d, want -1", got)
	}
}

// TestTodoKindColor checks the per-kind palette, the XXX/HACK alias
// sharing one colour, and the neutral default for an unknown kind.
func TestTodoKindColor(t *testing.T) {
	amber := paint.Color{R: 230, G: 180, B: 60, A: 255}
	red := paint.Color{R: 230, G: 80, B: 80, A: 255}
	orange := paint.Color{R: 230, G: 140, B: 60, A: 255}
	grey := paint.Color{R: 150, G: 150, B: 160, A: 255}
	def := paint.Color{R: 130, G: 130, B: 140, A: 255}

	cases := []struct {
		kind string
		want paint.Color
	}{
		{"TODO", amber},
		{"FIXME", red},
		{"XXX", orange},
		{"HACK", orange},
		{"NOTE", grey},
		{"WHATEVER", def},
		{"", def},
	}
	for _, c := range cases {
		if got := todoKindColor(c.kind); got != c.want {
			t.Errorf("todoKindColor(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
	// The default must be distinct from NOTE's grey so an unknown kind is
	// not silently rendered as a NOTE.
	if todoKindColor("WHATEVER") == todoKindColor("NOTE") {
		t.Error("default colour must differ from NOTE colour")
	}
}

// TestTodoRowLabel checks the "basename:line" formatting drops the dir.
func TestTodoRowLabel(t *testing.T) {
	cases := []struct {
		row  TodoRow
		want string
	}{
		{TodoRow{File: "ged/foo.go", Line: 42}, "foo.go:42"},
		{TodoRow{File: "core/baz.go", Line: 7}, "baz.go:7"},
		{TodoRow{File: "main.go", Line: 1}, "main.go:1"},
	}
	for _, c := range cases {
		if got := todoRowLabel(c.row); got != c.want {
			t.Errorf("todoRowLabel(%+v) = %q, want %q", c.row, got, c.want)
		}
	}
}

// TestTodoCountLabel checks the header tally text.
func TestTodoCountLabel(t *testing.T) {
	if got, want := todoCountLabel(3), "待办 / TODO (3)"; got != want {
		t.Errorf("todoCountLabel(3) = %q, want %q", got, want)
	}
	if got, want := todoCountLabel(0), "待办 / TODO (0)"; got != want {
		t.Errorf("todoCountLabel(0) = %q, want %q", got, want)
	}
}

// TestTodoRowClickActivates drives a click on row 2 through the hit-test +
// signal path (no GL) and checks SigRowActivated fires with the right file
// and 1-based line.
//
// Geometry: sized 300x400, rows start at todoHeaderH=22 with rowHeight=20
// and no scroll, so row 2 occupies y in [22+40, 22+60) = [62, 82); click
// the middle.
func TestTodoRowClickActivates(t *testing.T) {
	p := NewTodoPanel()
	p.SetSize(300, 400)
	rows := sampleTodoRows()
	p.SetRows(rows)

	var (
		gotFile string
		gotLine int
		fired   bool
	)
	p.SigRowActivated(func(file string, line int) {
		gotFile = file
		gotLine = line
		fired = true
	})

	y := todoHeaderH + 2*p.rowHeight + p.rowHeight/2 // 72
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("OnLeftDown did not fire SigRowActivated")
	}
	want := rows[2]
	if gotFile != want.File || gotLine != want.Line {
		t.Errorf("SigRowActivated = (%q,%d), want (%q,%d)",
			gotFile, gotLine, want.File, want.Line)
	}
}

// TestTodoHeaderClickNoop verifies a click in the header band does not fire
// the activation signal.
func TestTodoHeaderClickNoop(t *testing.T) {
	p := NewTodoPanel()
	p.SetSize(300, 400)
	p.SetRows(sampleTodoRows())

	fired := false
	p.SigRowActivated(func(string, int) { fired = true })
	p.OnLeftDown(5, 5) // inside the 22px header
	if fired {
		t.Error("OnLeftDown in header region fired SigRowActivated")
	}
}

// TestTodoPanelFactoryRegistered checks the factory id resolves to a
// constructible *TodoPanel, matching how silkide will instantiate it for
// docking.
func TestTodoPanelFactoryRegistered(t *testing.T) {
	obj := core.New("ged.TodoPanel")
	if _, ok := obj.(*TodoPanel); !ok {
		t.Fatalf("factory ged.TodoPanel built %T, want *TodoPanel", obj)
	}
}
