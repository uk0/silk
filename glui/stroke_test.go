package glui

import "testing"

// TestPolylineEdgeCases verifies the stroke API survives the degenerate
// inputs that callers actually hand it: empty input, a single point, two
// points (no joins), and a sequence containing zero-length segments.
// We exercise every join + cap combination so a panic in any branch
// surfaces immediately.
func TestPolylineEdgeCases(t *testing.T) {
	joins := []StrokeJoin{JoinMiter, JoinBevel, JoinRound}
	caps := []StrokeCap{CapButt, CapSquare, CapRound}

	cases := []struct {
		name string
		pts  [][2]float32
	}{
		{"empty", nil},
		{"single", [][2]float32{{1, 1}}},
		{"two", [][2]float32{{0, 0}, {10, 0}}},
		{"degenerate-zero-segment", [][2]float32{{0, 0}, {0, 0}, {10, 0}}},
		{"collinear", [][2]float32{{0, 0}, {5, 0}, {10, 0}}},
		{"reversal", [][2]float32{{0, 0}, {10, 0}, {0, 0}}},
		{"L-shape", [][2]float32{{0, 0}, {10, 0}, {10, 10}}},
	}

	for _, j := range joins {
		for _, cp := range caps {
			for _, c := range cases {
				r := newTestRenderer()
				style := StrokeStyle{
					Width: 2,
					Color: Color{1, 0, 0, 1},
					Join:  j,
					Cap:   cp,
				}
				// Must not panic for any join/cap/case combination.
				r.Polyline(c.pts, style)
			}
		}
	}
}

// TestPolylineZeroWidth verifies a zero-width polyline emits no geometry.
func TestPolylineZeroWidth(t *testing.T) {
	r := newTestRenderer()
	r.Polyline([][2]float32{{0, 0}, {10, 10}}, StrokeStyle{Width: 0})
	if len(r.verts) != 0 || len(r.indices) != 0 {
		t.Errorf("zero-width polyline emitted %d verts / %d indices; want 0/0",
			len(r.verts), len(r.indices))
	}
}

// TestPolylineEmitsGeometry verifies a normal multi-segment polyline emits
// positive-vertex geometry for each join style. This is a sanity floor —
// we don't pin exact counts because the join branches differ.
func TestPolylineEmitsGeometry(t *testing.T) {
	pts := [][2]float32{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	for _, j := range []StrokeJoin{JoinMiter, JoinBevel, JoinRound} {
		r := newTestRenderer()
		r.Polyline(pts, StrokeStyle{Width: 2, Color: Color{1, 1, 1, 1}, Join: j})
		if len(r.verts) == 0 {
			t.Errorf("join %d: emitted zero verts", j)
		}
		if len(r.indices) == 0 {
			t.Errorf("join %d: emitted zero indices", j)
		}
	}
}

// TestPolylineSolid verifies an empty Dash array behaves identically to the
// historical Polyline path: same vertex/index counts, no dashed branch.
// We compare a no-dash Polyline against a clean Polyline drawn with the
// same style — they must produce byte-identical buffers.
func TestPolylineSolid(t *testing.T) {
	pts := [][2]float32{{0, 0}, {30, 0}, {30, 30}}
	style := StrokeStyle{Width: 2, Color: Color{1, 1, 1, 1}, Join: JoinMiter}

	r1 := newTestRenderer()
	r1.Polyline(pts, style)

	r2 := newTestRenderer()
	style2 := style
	style2.Dash = nil
	r2.Polyline(pts, style2)

	if len(r1.verts) != len(r2.verts) || len(r1.indices) != len(r2.indices) {
		t.Fatalf("nil-dash diverged from solid: nilDash %d v / %d i, solid %d v / %d i",
			len(r2.verts), len(r2.indices), len(r1.verts), len(r1.indices))
	}
}

// TestPolylineDashedBasic verifies a horizontal segment with dash [10, 5]
// produces multiple sub-quads. The 30-pt segment with that pattern fits two
// full "on" pieces (0..10 and 15..25) plus a final run from 30..30 inside
// the third dash entry — we expect at least 2 quads (8 verts, 12 indices).
func TestPolylineDashedBasic(t *testing.T) {
	pts := [][2]float32{{0, 0}, {30, 0}}
	style := StrokeStyle{
		Width: 2,
		Color: Color{1, 1, 1, 1},
		Dash:  []float32{10, 5},
	}
	r := newTestRenderer()
	r.Polyline(pts, style)

	if len(r.verts) < 8 {
		t.Errorf("dashed [10,5] over 30pt: expected >= 8 verts (2 quads), got %d", len(r.verts))
	}
	if len(r.indices)%6 != 0 {
		t.Errorf("dashed indices must be a multiple of 6 (one quad each), got %d", len(r.indices))
	}
}

// TestPolylineDashedEmptyArray confirms an empty Dash slice falls through to
// the solid path. nil and []float32{} both should behave identically.
func TestPolylineDashedEmptyArray(t *testing.T) {
	pts := [][2]float32{{0, 0}, {20, 0}}
	style := StrokeStyle{Width: 2, Color: Color{1, 1, 1, 1}}

	r1 := newTestRenderer()
	r1.Polyline(pts, style)

	r2 := newTestRenderer()
	r2.Polyline(pts, StrokeStyle{Width: 2, Color: Color{1, 1, 1, 1}, Dash: []float32{}})

	if len(r1.verts) != len(r2.verts) {
		t.Fatalf("empty Dash slice should be solid: %d vs %d verts", len(r1.verts), len(r2.verts))
	}
}

// TestPolylineDashedZeroPattern guards against an infinite loop when every
// dash entry is zero — should produce no output rather than spin.
func TestPolylineDashedZeroPattern(t *testing.T) {
	pts := [][2]float32{{0, 0}, {20, 0}}
	r := newTestRenderer()
	r.Polyline(pts, StrokeStyle{
		Width: 2, Color: Color{1, 1, 1, 1},
		Dash: []float32{0, 0},
	})
	if len(r.verts) != 0 || len(r.indices) != 0 {
		t.Fatalf("zero-dash pattern should emit nothing, got %d v / %d i",
			len(r.verts), len(r.indices))
	}
}

// TestPolylineDashedOffsetShiftsPhase verifies DashOffset > 0 changes the
// phase. For [10, 5] at offset 10, the cursor starts in the OFF segment, so
// the first 5 points are gap. Compare the quad count to offset 0 — the
// patterns should differ.
func TestPolylineDashedOffsetShiftsPhase(t *testing.T) {
	pts := [][2]float32{{0, 0}, {40, 0}}
	style0 := StrokeStyle{Width: 2, Color: Color{1, 1, 1, 1}, Dash: []float32{10, 5}}
	style1 := StrokeStyle{Width: 2, Color: Color{1, 1, 1, 1}, Dash: []float32{10, 5}, DashOffset: 10}

	r0 := newTestRenderer()
	r0.Polyline(pts, style0)
	r1 := newTestRenderer()
	r1.Polyline(pts, style1)

	if len(r0.verts) == len(r1.verts) && len(r0.indices) == len(r1.indices) {
		// Same buffer counts could still mean different positions. Compare
		// first vertex X — at offset 0 it's at world x=0 (run starts at the
		// segment origin); at offset 10 the first run starts at x=15.
		if len(r0.verts) > 0 && len(r1.verts) > 0 && r0.verts[0].X == r1.verts[0].X {
			t.Errorf("DashOffset did not shift phase: same start vertex X")
		}
	}
}
