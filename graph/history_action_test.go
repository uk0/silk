package graph

import "testing"

type historyValueCommand struct{ value *int }

func (*historyValueCommand) Text() string { return "value" }
func (c *historyValueCommand) Undo()      { *c.value-- }
func (c *historyValueCommand) Redo()      { *c.value++ }

type historyOverrideView struct {
	GraphView
	undos, redos int
}

func (v *historyOverrideView) Undo() { v.undos++; v.Scene().UndoStack().Undo() }
func (v *historyOverrideView) Redo() { v.redos++; v.Scene().UndoStack().Redo() }

func TestViewHistoryActionUsesOverrideOnce(t *testing.T) {
	v := new(historyOverrideView)
	v.Init(v)
	s := new(SceneItem)
	s.Init(s)
	v.SetScene(s)
	value := 0
	s.PushCommand(&historyValueCommand{&value})
	v.redoAction.Trigger(v)
	if v.redos != 0 {
		t.Fatal("disabled redo executed")
	}
	before := v.undoAction.Rev()
	v.undoAction.Trigger(v)
	if value != 0 || v.undos != 1 || s.UndoStack().Current() != 0 {
		t.Fatal("undo bypassed or duplicated", value, v.undos)
	}
	if v.undoAction.IsEnabled() || !v.redoAction.IsEnabled() || before == v.undoAction.Rev() {
		t.Fatal("action state no longer follows stack")
	}
	v.redoAction.Trigger(v)
	if value != 1 || v.redos != 1 || s.UndoStack().Current() != 1 {
		t.Fatal("redo bypassed or duplicated")
	}
	old := &viewHistoryAction{IAction: s.UndoStack().UndoAction(), view: &v.GraphView, scene: s}
	replacement := new(SceneItem)
	replacement.Init(replacement)
	v.SetScene(replacement)
	old.Trigger(v)
	if value != 1 || v.undos != 1 {
		t.Fatal("stale action mutated old document")
	}
}
func TestViewHistoryActionPlainGraphFallback(t *testing.T) {
	v := NewView()
	s := new(SceneItem)
	s.Init(s)
	v.SetScene(s)
	value := 0
	s.PushCommand(&historyValueCommand{&value})
	v.undoAction.Trigger(v)
	if value != 0 {
		t.Fatal("plain graph undo changed")
	}
	v.redoAction.Trigger(v)
	if value != 1 {
		t.Fatal("plain graph redo changed")
	}
}
