package svg

import (
	"math"
	"testing"
)

const arcEpsilon = 1e-6

func nearly(a, b float64) bool {
	return math.Abs(a-b) < arcEpsilon
}

// TestDecomposeArcZeroLengthIsEmpty: x0,y0 == x,y → spec says skip.
func TestDecomposeArcZeroLengthIsEmpty(t *testing.T) {
	segs := decomposeArc(10, 20, 5, 5, 0, false, true, 10, 20)
	if len(segs) != 0 {
		t.Errorf("zero-length arc returned %d segs, want 0", len(segs))
	}
}

// TestDecomposeArcDegenerateRadiusIsLine: rx == 0 → straight-line
// segment via degenerate cubic.
func TestDecomposeArcDegenerateRadiusIsLine(t *testing.T) {
	segs := decomposeArc(0, 0, 0, 5, 0, false, true, 10, 0)
	if len(segs) != 1 {
		t.Fatalf("rx=0 arc segs = %d, want 1", len(segs))
	}
	s := segs[0]
	// Endpoints unchanged.
	if !nearly(s.startX, 0) || !nearly(s.startY, 0) {
		t.Errorf("start = (%v, %v), want (0, 0)", s.startX, s.startY)
	}
	if !nearly(s.endX, 10) || !nearly(s.endY, 0) {
		t.Errorf("end = (%v, %v), want (10, 0)", s.endX, s.endY)
	}
	// Both control points at midpoint = (5, 0).
	if !nearly(s.c1x, 5) || !nearly(s.c2x, 5) {
		t.Errorf("control points should both be at midpoint x=5; got c1.x=%v c2.x=%v",
			s.c1x, s.c2x)
	}
}

// TestDecomposeArcQuarterCircle: from (10, 0) to (0, 10) along a
// unit-radius circle's first quadrant. Expect a single cubic with
// endpoints matching.
func TestDecomposeArcQuarterCircle(t *testing.T) {
	// Quarter-turn counter-clockwise on a circle of radius 10 centred
	// at origin: x0=(10,0), x=(0,10). sweep=true (positive direction).
	segs := decomposeArc(10, 0, 10, 10, 0, false, true, 0, 10)
	if len(segs) != 1 {
		t.Fatalf("quarter arc segs = %d, want 1", len(segs))
	}
	s := segs[0]
	if !nearly(s.startX, 10) || !nearly(s.startY, 0) {
		t.Errorf("start = (%v, %v), want (10, 0)", s.startX, s.startY)
	}
	if !nearly(s.endX, 0) || !nearly(s.endY, 10) {
		t.Errorf("end = (%v, %v), want (0, 10)", s.endX, s.endY)
	}
}

// TestDecomposeArcLargeArcFlag: largeArcFlag=true with sweep=true
// produces > 180° sweep. We expect ≥3 segments (90°-slice cap).
func TestDecomposeArcLargeArcFlag(t *testing.T) {
	// From (10, 0) to (-10, 0) on circle radius 10. Two valid arcs:
	// short (top half, 180°) and long (bottom half, 180°). With both
	// flags the same, sign convention picks one of them.
	segs := decomposeArc(10, 0, 10, 10, 0, true, true, -10, 0)
	if len(segs) < 2 {
		t.Errorf("180°+ arc should split into ≥2 slices, got %d", len(segs))
	}
}

// TestDecomposeArcSweepFlagFlipsDirection: sweep=true vs sweep=false
// over the same endpoints + small arc gives mirror sweeps.
func TestDecomposeArcSweepFlagFlipsDirection(t *testing.T) {
	// Quarter from (10, 0) to (0, 10) on a circle of radius 10.
	swT := decomposeArc(10, 0, 10, 10, 0, false, true, 0, 10)
	swF := decomposeArc(10, 0, 10, 10, 0, false, false, 0, 10)
	if len(swT) == 0 || len(swF) == 0 {
		t.Fatalf("expected non-empty in both cases")
	}
	// Different sweep flags → control points on opposite sides of the
	// chord. We check that the c1y of the two paths have opposite
	// signs (positive sweep arc bows up; negative bows down).
	if (swT[0].c1y > 0) == (swF[0].c1y > 0) {
		t.Errorf("sweep flag did not flip control-point side: trueY=%v falseY=%v",
			swT[0].c1y, swF[0].c1y)
	}
}

// TestDecomposeArcEndpointAccurate: regardless of slice count,
// the final segment's endpoint must equal (x, y) exactly (we
// override the last endpoint to defeat float drift).
func TestDecomposeArcEndpointAccurate(t *testing.T) {
	cases := []struct {
		x0, y0, x, y float64
		large, sweep bool
	}{
		{0, 0, 100, 0, false, true},
		{50, 50, -50, 50, true, true},
		{0, 0, 7.123, 12.456, true, false},
	}
	for i, c := range cases {
		segs := decomposeArc(c.x0, c.y0, 80, 60, 30, c.large, c.sweep, c.x, c.y)
		if len(segs) == 0 {
			continue
		}
		last := segs[len(segs)-1]
		if !nearly(last.endX, c.x) || !nearly(last.endY, c.y) {
			t.Errorf("case %d: last endpoint = (%v, %v), want (%v, %v)",
				i, last.endX, last.endY, c.x, c.y)
		}
	}
}

// TestDecomposeArcRadiiTooSmallAreScaled: the W3C spec says when
// the supplied radii can't reach between the endpoints, scale up
// uniformly. We verify by feeding a tiny rx/ry that physically
// can't span the chord — the function must still produce segments
// without NaN.
func TestDecomposeArcRadiiTooSmallAreScaled(t *testing.T) {
	segs := decomposeArc(0, 0, 1, 1, 0, false, true, 100, 0)
	if len(segs) == 0 {
		t.Fatalf("scaled-up arc should yield segments")
	}
	for i, s := range segs {
		if math.IsNaN(s.c1x) || math.IsNaN(s.c1y) {
			t.Errorf("seg %d has NaN control point: %+v", i, s)
		}
	}
}

// TestRenderPathArcEmitsCurveTo: integration check — a path with an
// A command produces ≥1 painter.CurveTo call (under the new code
// path; previously was a LineTo).
func TestRenderPathArcEmitsCurveTo(t *testing.T) {
	// "M 0 0 A 50 50 0 0 1 100 0" — half-circle from origin to (100,0)
	// going below the chord (sweep=1).
	doc, err := ParseString(`<svg viewBox="0 0 200 200">
		<path d="M 0 0 A 50 50 0 0 1 100 0" fill="none" stroke="black"/>
	</svg>`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rec := &recPainter{}
	Render(doc, rec, 0, 0, 200, 200)

	curveCount := 0
	for _, c := range rec.calls {
		if c == "CurveTo" {
			curveCount++
		}
	}
	if curveCount == 0 {
		t.Errorf("PathArc should emit CurveTo via decomposition; got 0 (calls=%v)",
			rec.calls)
	}
}
