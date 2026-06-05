package gui

import (
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
