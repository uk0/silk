package ged

import (
	"github.com/uk0/silk/geom"
	"math"
	"testing"
)

// guideCanvas is the fixed page fixture shared by the guide cases: 800x600 mm
// with centre at (400, 300).
func guideCanvas() geom.Rect { return geom.Rect{X: 0, Y: 0, Width: 800, Height: 600} }

// hasVGuide reports whether gs contains a vertical guide (x1==x2) at x.
func hasVGuide(gs []alignGuide, x float64) bool {
	for _, g := range gs {
		if g.x1 == g.x2 && math.Abs(g.x1-x) <= 1e-9 {
			return true
		}
	}
	return false
}

// hasHGuide reports whether gs contains a horizontal guide (y1==y2) at y.
func hasHGuide(gs []alignGuide, y float64) bool {
	for _, g := range gs {
		if g.y1 == g.y2 && math.Abs(g.y1-y) <= 1e-9 {
			return true
		}
	}
	return false
}

// countVertical counts vertical guides (x1==x2) in gs.
func countVertical(gs []alignGuide) int {
	n := 0
	for _, g := range gs {
		if g.x1 == g.x2 {
			n++
		}
	}
	return n
}

// TestComputeAlignGuidesLeftEdge: a dragged left edge 3px from another rect's
// left edge (threshold 5) yields a vertical guide at that edge, and nothing on
// the horizontal axis.
func TestComputeAlignGuidesLeftEdge(t *testing.T) {
	other := geom.Rect{X: 100, Y: 100, Width: 50, Height: 20} // left = 100
	dragged := geom.Rect{X: 103, Y: 305, Width: 30, Height: 10}
	gs := computeAlignGuides(dragged, []geom.Rect{other}, guideCanvas(), 5)

	if !hasVGuide(gs, 100) {
		t.Fatalf("expected vertical guide at x=100 (left-to-left, 3px off), got %+v", gs)
	}
	if hasHGuide(gs, 100) {
		t.Errorf("did not expect a horizontal guide at y=100; axes crossed: %+v", gs)
	}
}

// TestComputeAlignGuidesCenterX: dragged horizontal centre 2px from another
// rect's centre yields a vertical guide at that centre.
func TestComputeAlignGuidesCenterX(t *testing.T) {
	other := geom.Rect{X: 100, Y: 100, Width: 60, Height: 20}   // centerX = 130
	dragged := geom.Rect{X: 108, Y: 400, Width: 40, Height: 10} // centerX = 128
	gs := computeAlignGuides(dragged, []geom.Rect{other}, guideCanvas(), 5)

	if !hasVGuide(gs, 130) {
		t.Fatalf("expected vertical guide at x=130 (centre-to-centre, 2px off), got %+v", gs)
	}
}

// TestComputeAlignGuidesRightEdge: dragged right edge 2px from another rect's
// right edge yields a vertical guide at that edge.
func TestComputeAlignGuidesRightEdge(t *testing.T) {
	other := geom.Rect{X: 100, Y: 100, Width: 50, Height: 20}   // right = 150
	dragged := geom.Rect{X: 132, Y: 400, Width: 20, Height: 10} // right = 152
	gs := computeAlignGuides(dragged, []geom.Rect{other}, guideCanvas(), 5)

	if !hasVGuide(gs, 150) {
		t.Fatalf("expected vertical guide at x=150 (right-to-right, 2px off), got %+v", gs)
	}
}

// TestComputeAlignGuidesTopBottom: dragged top edge aligning another rect's top
// (H guide at 100) and, separately, aligning another rect's bottom (H guide at
// 120). No vertical guide in either case (X held far from every x target).
func TestComputeAlignGuidesTopBottom(t *testing.T) {
	other := geom.Rect{X: 100, Y: 100, Width: 50, Height: 20} // top 100, bottom 120

	topAlign := geom.Rect{X: 500, Y: 102, Width: 40, Height: 10} // top 102 -> 100
	gs := computeAlignGuides(topAlign, []geom.Rect{other}, guideCanvas(), 5)
	if !hasHGuide(gs, 100) {
		t.Fatalf("expected horizontal guide at y=100 (top-to-top), got %+v", gs)
	}
	if countVertical(gs) != 0 {
		t.Errorf("expected no vertical guides for a pure top-align, got %+v", gs)
	}

	bottomAlign := geom.Rect{X: 500, Y: 121, Width: 40, Height: 10} // top 121 -> other bottom 120
	gs = computeAlignGuides(bottomAlign, []geom.Rect{other}, guideCanvas(), 5)
	if !hasHGuide(gs, 120) {
		t.Fatalf("expected horizontal guide at y=120 (top-to-bottom), got %+v", gs)
	}
}

// TestComputeAlignGuidesCanvasCenter: with no other rects, a rect centred on
// the page produces the canvas centre-X (vertical) and centre-Y (horizontal)
// guides.
func TestComputeAlignGuidesCanvasCenter(t *testing.T) {
	dragged := geom.Rect{X: 380, Y: 290, Width: 40, Height: 20} // centre (400, 300)
	gs := computeAlignGuides(dragged, nil, guideCanvas(), 5)

	if !hasVGuide(gs, 400) {
		t.Errorf("expected vertical guide at canvas centre x=400, got %+v", gs)
	}
	if !hasHGuide(gs, 300) {
		t.Errorf("expected horizontal guide at canvas centre y=300, got %+v", gs)
	}
}

// TestComputeAlignGuidesNoneWhenFar: a rect far from every edge/centre and from
// the canvas centre yields no guides.
func TestComputeAlignGuidesNoneWhenFar(t *testing.T) {
	other := geom.Rect{X: 100, Y: 100, Width: 50, Height: 20}
	dragged := geom.Rect{X: 500, Y: 500, Width: 30, Height: 30}
	gs := computeAlignGuides(dragged, []geom.Rect{other}, guideCanvas(), 5)

	if len(gs) != 0 {
		t.Fatalf("expected no guides when nothing is within threshold, got %+v", gs)
	}
}

// TestComputeAlignGuidesExactAlign: an exact edge match (0px off) still emits a
// guide.
func TestComputeAlignGuidesExactAlign(t *testing.T) {
	other := geom.Rect{X: 200, Y: 100, Width: 40, Height: 20} // left = 200
	dragged := geom.Rect{X: 200, Y: 400, Width: 30, Height: 10}
	gs := computeAlignGuides(dragged, []geom.Rect{other}, guideCanvas(), 5)

	if !hasVGuide(gs, 200) {
		t.Fatalf("expected vertical guide at x=200 for an exact left-edge match, got %+v", gs)
	}
}
