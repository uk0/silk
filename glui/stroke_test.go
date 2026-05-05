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
