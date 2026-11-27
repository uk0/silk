package ged

import (
	"errors"
	"testing"

	"github.com/uk0/silk/graph"
)

func TestCanvasMoveUsesActualReleaseAndNeverCommitsOutsideForm(t *testing.T) {
	v := NewGedView()
	s := v.GedScene()
	s.SetSize(200, 140)
	item := sceneWidget(t, s, "gui.Button", "move", 10, 10, 25, 7)
	first := sceneWidget(t, s, "gui.VBox", "first", 60, 10, 50, 75)
	second := sceneWidget(t, s, "gui.VBox", "second", 115, 10, 65, 75)
	arm := func() {
		t.Helper()
		clearClickHistory()
		v.Selection().Clear()
		v.Selection().Add(item)
		v.OnLeftDown(v.MapFromScene(12, 12))
		v.OnMouseMove(v.MapFromScene(75, 45))
		if v.dropContainer != graph.IItem(first) || s.UndoStack().Current() != 0 {
			t.Fatalf("expected preview on first without an applied drag: %+v", v.dropContainer)
		}
	}
	arm()
	v.OnLeftUp(v.MapFromScene(140, 45))
	if item.Parent() != graph.IItem(second) {
		t.Fatal("release used the old hover or failed to move")
	}
	v.Undo()
	arm()
	old := item.Bounds1()
	v.OnLeftUp(v.MapFromScene(220, 45))
	if item.Bounds1() != old || item.Parent() != graph.IItem(s) || s.UndoStack().CanUndo() || !s.UndoStack().CanRedo() {
		t.Fatal("outside release mutated geometry, parent, or redo history")
	}
	v.Redo()
	if item.Parent() != graph.IItem(second) {
		t.Fatal("cancelled drag damaged the earlier redo")
	}
}

type failingCanvasEdit struct {
	attempts int
}

func (*failingCanvasEdit) HasPendingChanges() bool { return true }

func (draft *failingCanvasEdit) CommitPendingChanges() error {
	draft.attempts++
	return errors.New("uncommitted fixture")
}

func TestCanvasMoveWillNotPartiallyApplyWhenDraftCommitFails(t *testing.T) {
	v := NewGedView()
	s := v.GedScene()
	s.SetSize(200, 140)
	item := sceneWidget(t, s, "gui.Button", "move", 10, 10, 25, 7)
	sceneWidget(t, s, "gui.VBox", "dest", 60, 10, 65, 75)
	v.Selection().Add(item)
	start := item.Bounds1()
	draft := &failingCanvasEdit{}
	s.AddPendingEditor(draft)
	changes := 0
	s.SigDesignChanged(func() { changes++ })
	dragOnCanvas(v, 12, 12, 80, 40)
	if draft.attempts != 1 || changes != 0 || s.UndoStack().CanUndo() || item.Parent() != graph.IItem(s) || item.Bounds1() != start {
		t.Fatal("failed property commit leaked part of the canvas edit")
	}
}

func TestCanvasMoveClampedByNestedOwnerStaysVisible(t *testing.T) {
	for _, sameOwner := range []bool{false, true} {
		name := "reparented"
		if sameOwner {
			name = "same-owner"
		}
		t.Run(name, func(t *testing.T) {
			v := NewGedView()
			s := v.GedScene()
			s.SetSize(200, 140)
			outer := sceneWidget(t, s, "gui.VBox", "outer", 70, 10, 50, 80)
			inner := sceneWidget(t, s, "gui.VBox", "inner", 44, 26, 64, 47)
			inner.SetParent(outer)
			item := sceneWidget(t, s, "gui.Button", "move", 10, 32, 25, 7)
			grabX := 34.0
			if sameOwner {
				item.SetPos(76, 32)
				item.SetParent(inner)
				grabX = 100
			}
			old, parent := item.Bounds1(), item.Parent()
			v.Selection().Add(item)
			// Button's right edge would touch x=70 after snap, overlapping
			// inner's declared box but fully hidden by the outer clip.
			dragOnCanvas(v, grabX, 33.75, 70.4, 45)
			if item.Parent() != graph.IItem(inner) || item.X() != 72 || item.Y() != 45 || drawnRect(item).IsEmpty() {
				t.Fatalf("drop did not park inside the visible ancestor intersection: parent=%v bounds=%v", item.Parent(), item.Bounds1())
			}
			after := item.Bounds1()
			v.Undo()
			if s.UndoStack().CanUndo() || item.Bounds1() != old || item.Parent() != parent {
				t.Fatal("undo did not restore the pre-drag model")
			}
			v.Redo()
			if item.Bounds1() != after || drawnRect(item).IsEmpty() || item.Parent() != graph.IItem(inner) {
				t.Fatal("redo did not repeat the visible placement")
			}
		})
	}
}
