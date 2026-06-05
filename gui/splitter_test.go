package gui

import (
	"math"
	"testing"
)

// buildSplitter creates a splitter of the given orientation and size with n
// equally-sized panes, ready for handle hit-testing.
func buildSplitter(vertical bool, w, h float64, n int) *Splitter {
	s := NewSplitter(vertical)
	s.SetHandleSize(10)
	s.SetSize(w, h)
	for i := 0; i < n; i++ {
		s.AddWidget(NewLabel("pane"))
	}
	return s
}

// With handleSize 10 and two equal panes across 200px, the single gap sits at
// [95, 105): a point inside the gap maps to handle 0, points inside either
// pane map to -1.
func TestSplitterHandleAtPosHorizontal(t *testing.T) {
	s := buildSplitter(false, 200, 50, 2)

	if got := s.handleAtPos(100, 25); got != 0 {
		t.Errorf("mid-handle (100,25): got %d, want 0", got)
	}
	if got := s.handleAtPos(40, 25); got != -1 {
		t.Errorf("inside left pane (40,25): got %d, want -1", got)
	}
	if got := s.handleAtPos(160, 25); got != -1 {
		t.Errorf("inside right pane (160,25): got %d, want -1", got)
	}
}

// Vertical splitter: the gap runs horizontally at y in [95, 105).
func TestSplitterHandleAtPosVertical(t *testing.T) {
	s := buildSplitter(true, 50, 200, 2)

	if got := s.handleAtPos(25, 100); got != 0 {
		t.Errorf("mid-handle (25,100): got %d, want 0", got)
	}
	if got := s.handleAtPos(25, 40); got != -1 {
		t.Errorf("inside top pane (25,40): got %d, want -1", got)
	}
	if got := s.handleAtPos(25, 160); got != -1 {
		t.Errorf("inside bottom pane (25,160): got %d, want -1", got)
	}
}

// Three equal panes across 320px with handleSize 10: available = 300, each
// pane = 100, so handle 0 sits at [100,110) and handle 1 at [210,220).
func TestSplitterHandleAtPosThreePanes(t *testing.T) {
	s := buildSplitter(false, 320, 50, 3)

	if got := s.handleAtPos(105, 25); got != 0 {
		t.Errorf("first gap (105,25): got %d, want 0", got)
	}
	if got := s.handleAtPos(215, 25); got != 1 {
		t.Errorf("second gap (215,25): got %d, want 1", got)
	}
	if got := s.handleAtPos(160, 25); got != -1 {
		t.Errorf("middle pane (160,25): got %d, want -1", got)
	}
}

// A point outside the cross-axis extent of the handle is not a hit, and a
// splitter with fewer than two panes has no handles at all.
func TestSplitterHandleAtPosEdges(t *testing.T) {
	s := buildSplitter(false, 200, 50, 2)
	if got := s.handleAtPos(100, 80); got != -1 {
		t.Errorf("below widget (100,80): got %d, want -1", got)
	}

	single := buildSplitter(false, 200, 50, 1)
	if got := single.handleAtPos(100, 25); got != -1 {
		t.Errorf("single pane has no handle: got %d, want -1", got)
	}
}

// paneExtents returns each child's main-axis length after the most recent
// Layout: width for a horizontal splitter, height for a vertical one.
func paneExtents(s *Splitter) []float64 {
	var out []float64
	s.NakedWidget().eachChild(func(_ int, c IWidget) bool {
		_, _, w, h := c.Bounds()
		if s.Vertical() {
			out = append(out, h)
		} else {
			out = append(out, w)
		}
		return true
	})
	return out
}

const splitterEps = 1e-6

// paneToCollapse picks the smaller adjacent pane and, on a tie, the pane before
// the handle (index h).
func TestSplitterPaneToCollapse(t *testing.T) {
	s := buildSplitter(false, 300, 50, 3)

	// Pane 1 smaller than pane 0 -> handle 0 collapses pane 1.
	s.SetSizes([]float64{3, 1, 2})
	if got := s.paneToCollapse(0); got != 1 {
		t.Errorf("handle 0 with sizes [3,1,2]: got pane %d, want 1", got)
	}
	// Pane 1 smaller than pane 2 -> handle 1 collapses pane 1.
	if got := s.paneToCollapse(1); got != 1 {
		t.Errorf("handle 1 with sizes [3,1,2]: got pane %d, want 1", got)
	}
	// Equal neighbours -> default to the pane before the handle.
	s.SetSizes([]float64{1, 1, 1})
	if got := s.paneToCollapse(1); got != 1 {
		t.Errorf("handle 1 with equal sizes: got pane %d, want 1", got)
	}
	// Out-of-range handle has no pane to collapse.
	if got := s.paneToCollapse(5); got != -1 {
		t.Errorf("out-of-range handle: got pane %d, want -1", got)
	}
}

// Double-clicking a handle in a 2-pane splitter collapses the smaller pane to
// zero and the survivor absorbs the freed space; a second double-click
// restores the prior size. Drives toggleHandleCollapse, the method the
// double-click path calls.
func TestSplitterToggleCollapseTwoPanes(t *testing.T) {
	s := buildSplitter(false, 200, 50, 2) // handleSize 10 -> available 190
	s.SetSizes([]float64{3, 1})           // pane 1 is smaller
	s.Layout()

	// Collapse: handle 0 targets the smaller pane (1).
	s.toggleHandleCollapse(0)
	s.Layout()

	if got := s.Sizes()[1]; got != 0 {
		t.Fatalf("after collapse: pane 1 size got %g, want 0", got)
	}
	ext := paneExtents(s)
	if math.Abs(ext[1]) > splitterEps {
		t.Errorf("after collapse: pane 1 extent got %g, want 0", ext[1])
	}
	// Survivor absorbs everything except the handle.
	if want := 200.0 - s.HandleSize(); math.Abs(ext[0]-want) > splitterEps {
		t.Errorf("after collapse: pane 0 extent got %g, want %g", ext[0], want)
	}

	// Restore: same handle double-clicked again.
	s.toggleHandleCollapse(0)
	s.Layout()

	if got := s.Sizes()[1]; math.Abs(got-1) > splitterEps {
		t.Errorf("after restore: pane 1 size got %g, want 1", got)
	}
	ext = paneExtents(s)
	// Back to the original 3:1 split of the 190px available space.
	if want := 190.0 * 3.0 / 4.0; math.Abs(ext[0]-want) > splitterEps {
		t.Errorf("after restore: pane 0 extent got %g, want %g", ext[0], want)
	}
	if want := 190.0 * 1.0 / 4.0; math.Abs(ext[1]-want) > splitterEps {
		t.Errorf("after restore: pane 1 extent got %g, want %g", ext[1], want)
	}
}

// In a 3-pane splitter, collapsing the middle pane via either adjacent handle
// drives it to zero while the outer panes keep their relative split; a second
// double-click on the same handle restores it.
func TestSplitterToggleCollapseThreePanes(t *testing.T) {
	s := buildSplitter(false, 320, 50, 3) // handleSize 10 -> available 300
	s.SetSizes([]float64{2, 1, 2})        // middle pane is the smallest
	s.Layout()

	// Handle 1 sits between panes 1 and 2; pane 1 is smaller, so it collapses.
	s.toggleHandleCollapse(1)
	s.Layout()

	if got := s.Sizes()[1]; got != 0 {
		t.Fatalf("after collapse: middle pane size got %g, want 0", got)
	}
	ext := paneExtents(s)
	if math.Abs(ext[1]) > splitterEps {
		t.Errorf("after collapse: middle extent got %g, want 0", ext[1])
	}
	// available now 300 (320 - 2 handles); outer panes split it 2:2 = 150 each.
	if math.Abs(ext[0]-150) > splitterEps || math.Abs(ext[2]-150) > splitterEps {
		t.Errorf("after collapse: outer extents got %g/%g, want 150/150", ext[0], ext[2])
	}

	// Restore via the same handle.
	s.toggleHandleCollapse(1)
	s.Layout()

	if got := s.Sizes()[1]; math.Abs(got-1) > splitterEps {
		t.Errorf("after restore: middle pane size got %g, want 1", got)
	}
	ext = paneExtents(s)
	// Original 2:1:2 over 300px available -> 120 / 60 / 120.
	if math.Abs(ext[0]-120) > splitterEps || math.Abs(ext[1]-60) > splitterEps || math.Abs(ext[2]-120) > splitterEps {
		t.Errorf("after restore: extents got %g/%g/%g, want 120/60/120", ext[0], ext[1], ext[2])
	}
}

// A vertical splitter collapses along the main (height) axis the same way.
func TestSplitterToggleCollapseVertical(t *testing.T) {
	s := buildSplitter(true, 50, 200, 2) // handleSize 10 -> available 190
	s.SetSizes([]float64{1, 3})          // top pane is smaller
	s.Layout()

	s.toggleHandleCollapse(0)
	s.Layout()

	if got := s.Sizes()[0]; got != 0 {
		t.Fatalf("after collapse: top pane size got %g, want 0", got)
	}
	ext := paneExtents(s)
	if math.Abs(ext[0]) > splitterEps {
		t.Errorf("after collapse: top extent got %g, want 0", ext[0])
	}
	if want := 200.0 - s.HandleSize(); math.Abs(ext[1]-want) > splitterEps {
		t.Errorf("after collapse: bottom extent got %g, want %g", ext[1], want)
	}

	s.toggleHandleCollapse(0)
	s.Layout()
	if got := s.Sizes()[0]; math.Abs(got-1) > splitterEps {
		t.Errorf("after restore: top pane size got %g, want 1", got)
	}
}
