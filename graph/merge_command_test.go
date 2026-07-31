package graph

import (
	"testing"

	"github.com/uk0/silk/geom"
)

// redoneMove returns a MoveCommand tagged with token, already redone so it is
// in the state UndoStack.Push finds it in: sitting below the stack top with its
// records holding the pre-gesture positions.
func redoneMove(token uint64, items ...IItem) *MoveCommand {
	cmd := NewMoveCommand()
	for _, it := range items {
		x, y := it.Pos()
		cmd.AddItem(it, x+1, y)
	}
	cmd.SetMergeToken(token)
	cmd.Redo()
	return cmd
}

// pendingMove returns a MoveCommand tagged with token that has not been redone.
func pendingMove(token uint64, items ...IItem) *MoveCommand {
	cmd := NewMoveCommand()
	for _, it := range items {
		x, y := it.Pos()
		cmd.AddItem(it, x+1, y)
	}
	cmd.SetMergeToken(token)
	return cmd
}

// TestMoveCommandMergeWidthGating covers every reason a merge must be refused.
// The untagged cases are the important ones: a mouse drag pushes a MoveCommand
// with no token, and if that absorbed a following keyboard nudge then undoing
// the nudge would silently take the drag back with it. The item-list cases
// matter just as much, and they are asymmetric: a merge keeps only the lower
// command's records, so an item present only in the incoming command would end
// up with no undo record at all, while an item it has dropped still has one.
func TestMoveCommandMergeWidthGating(t *testing.T) {
	a, b := NewRectItem(), NewRectItem()
	a.SetBounds(0, 0, 10, 10)
	b.SetBounds(50, 50, 10, 10)

	cases := []struct {
		name string
		prev *MoveCommand
		next *MoveCommand
		want bool
	}{
		{"same gesture, same items", redoneMove(7, a, b), pendingMove(7, a, b), true},
		{"untagged prev never merges", redoneMove(0, a), pendingMove(0, a), false},
		{"untagged next never merges", redoneMove(7, a), pendingMove(0, a), false},
		{"different gestures", redoneMove(7, a), pendingMove(8, a), false},
		{"next covers more items", redoneMove(7, a), pendingMove(7, a, b), false},
		{"next covers fewer items", redoneMove(7, a, b), pendingMove(7, a), true},
		{"same count, different items", redoneMove(7, a), pendingMove(7, b), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.prev.MergeWidth(tc.next); got != tc.want {
				t.Errorf("MergeWidth() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMergeWidthRejectsOtherCommandType: the stack offers whichever command
// comes next, so a resize following a nudge (or the reverse) must be refused —
// the two record types carry different geometry and cannot stand in for each
// other's undo state, however well their gesture tokens line up.
func TestMergeWidthRejectsOtherCommandType(t *testing.T) {
	a := NewRectItem()
	a.SetBounds(0, 0, 10, 10)

	resize := NewResizeCommand()
	resize.AddItem(a, geom.Rect{X: 0, Y: 0, Width: 20, Height: 10})
	resize.SetMergeToken(7)

	if redoneMove(7, a).MergeWidth(resize) {
		t.Error("a MoveCommand merged a ResizeCommand")
	}
	if resize.MergeWidth(pendingMove(7, a)) {
		t.Error("a ResizeCommand merged a MoveCommand")
	}
}

// TestResizeCommandMergeWidthGating mirrors the MoveCommand table: the two
// implementations must stay in step, and this repo has already been bitten by a
// fix landing in one place and not its twin.
func TestResizeCommandMergeWidthGating(t *testing.T) {
	a, b := NewRectItem(), NewRectItem()
	a.SetBounds(0, 0, 10, 10)
	b.SetBounds(50, 50, 10, 10)

	redone := func(token uint64, items ...IItem) *ResizeCommand {
		cmd := NewResizeCommand()
		for _, it := range items {
			cmd.AddItem(it, resizeRectBy(it.Bounds1(), 1, 0, 1))
		}
		cmd.SetMergeToken(token)
		cmd.Redo()
		return cmd
	}
	pending := func(token uint64, items ...IItem) *ResizeCommand {
		cmd := NewResizeCommand()
		for _, it := range items {
			cmd.AddItem(it, resizeRectBy(it.Bounds1(), 1, 0, 1))
		}
		cmd.SetMergeToken(token)
		return cmd
	}

	cases := []struct {
		name string
		prev *ResizeCommand
		next *ResizeCommand
		want bool
	}{
		{"same gesture, same items", redone(7, a, b), pending(7, a, b), true},
		{"untagged prev never merges", redone(0, a), pending(0, a), false},
		{"different gestures", redone(7, a), pending(8, a), false},
		{"next covers more items", redone(7, a), pending(7, a, b), false},
		// The reachable one: a burst shrinking a mixed selection drops every
		// item that reaches the size floor, so from the second repeat onward
		// the incoming command drives a subset. Refusing that split one held
		// key into two undo steps.
		{"next covers fewer items", redone(7, a, b), pending(7, a), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.prev.MergeWidth(tc.next); got != tc.want {
				t.Errorf("MergeWidth() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestResizeRectBy pins the pure clamp. The floor cases are the reason it
// exists: a held Ctrl+Left repeats for as long as the key is down, and an
// unclamped width runs straight through zero into a negative extent.
func TestResizeRectBy(t *testing.T) {
	base := geom.Rect{X: 10, Y: 20, Width: 30, Height: 40}
	const minSize = 1.0

	cases := []struct {
		name   string
		dw, dh float64
		want   geom.Rect
	}{
		{"grow both", 5, 6, geom.Rect{X: 10, Y: 20, Width: 35, Height: 46}},
		{"shrink both", -5, -6, geom.Rect{X: 10, Y: 20, Width: 25, Height: 34}},
		{"width only", 5, 0, geom.Rect{X: 10, Y: 20, Width: 35, Height: 40}},
		{"height only", 0, 5, geom.Rect{X: 10, Y: 20, Width: 30, Height: 45}},
		{"floor width", -100, 0, geom.Rect{X: 10, Y: 20, Width: minSize, Height: 40}},
		{"floor both", -1000, -1000, geom.Rect{X: 10, Y: 20, Width: minSize, Height: minSize}},
		{"exactly at floor", -29, -39, geom.Rect{X: 10, Y: 20, Width: minSize, Height: minSize}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resizeRectBy(base, tc.dw, tc.dh, minSize); got != tc.want {
				t.Errorf("resizeRectBy(%+v, %g, %g, %g) = %+v, want %+v",
					base, tc.dw, tc.dh, minSize, got, tc.want)
			}
		})
	}
}

// TestResizeRectBySubFloorExtent: nothing stops a widget from already being
// smaller than the floor — the property sheet takes any size, and so does a
// loaded design file. Clamping such an extent up to minSize would let the
// shrink key double it, so the floor may only ever hold an extent back, never
// push it up. Growing still works from down there.
func TestResizeRectBySubFloorExtent(t *testing.T) {
	tiny := geom.Rect{X: 10, Y: 20, Width: 0.5, Height: 0.5}
	const minSize = 1.0

	if got := resizeRectBy(tiny, -1, -1, minSize); got != tiny {
		t.Errorf("shrinking a sub-floor rect gave %+v, want it left at %+v", got, tiny)
	}

	want := geom.Rect{X: 10, Y: 20, Width: 1.5, Height: 1.5}
	if got := resizeRectBy(tiny, 1, 1, minSize); got != want {
		t.Errorf("growing a sub-floor rect gave %+v, want %+v", got, want)
	}
}
