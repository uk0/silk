package gui

import "testing"

type checkpointCommand struct {
	redo, undo func()
	merge      bool
}

func (c *checkpointCommand) Redo() {
	if c.redo != nil {
		c.redo()
	}
}
func (c *checkpointCommand) Undo() {
	if c.undo != nil {
		c.undo()
	}
}
func (c *checkpointCommand) Text() string             { return "checkpoint" }
func (c *checkpointCommand) MergeWidth(ICommand) bool { return c.merge }

func TestUndoCheckpointRejectsReusedPositionsAndOtherStacks(t *testing.T) {
	for _, action := range []string{"undo-redo", "branch", "merge", "clear", "clean"} {
		t.Run(action, func(t *testing.T) {
			s := NewUndoStack("")
			s.Push(&checkpointCommand{merge: action == "merge"})
			p := s.Checkpoint()
			if !p.IsCurrent(s) || p.IsCurrent(NewUndoStack("")) || (UndoCheckpoint{}).IsCurrent(s) {
				t.Fatal("invalid initial checkpoint ownership")
			}
			switch action {
			case "undo-redo":
				s.Undo()
				s.Redo()
			case "branch":
				s.Undo()
				s.Push(&checkpointCommand{})
			case "merge":
				s.Push(&checkpointCommand{})
			case "clear":
				s.Clear()
			case "clean":
				s.SetClean()
			}
			if p.IsCurrent(s) {
				t.Fatal("checkpoint survived a history identity change")
			}
			if p.SameEdits(s) != (action == "clean") || p.SameEdits(NewUndoStack("")) || (UndoCheckpoint{}).SameEdits(s) {
				t.Fatal("command identity did not isolate clean-marker changes")
			}
			if action == "merge" && s.Count() != 1 {
				t.Fatal("test did not exercise unchanged numeric position")
			}
		})
	}
}

func TestUndoCheckpointNeverCapturesPartialCommand(t *testing.T) {
	s := NewUndoStack("")
	var during UndoCheckpoint
	s.Push(&checkpointCommand{redo: func() {
		during = s.Checkpoint()
		if during.IsCurrent(s) {
			t.Fatal("checkpoint valid inside Redo")
		}
		if during.SameEdits(s) {
			t.Fatal("command identity captured partial Redo")
		}
	}})
	if during.IsCurrent(s) || !s.Checkpoint().IsCurrent(s) {
		t.Fatal("partial-command checkpoint became valid")
	}
}

func TestEditorMarkSnapshotSavedRetainsNewInputAndHistory(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("loaded\r\n")
	e.MarkSaved()
	e.ReplaceAllText("captured 中文😀\r\n")
	captured := e.Text()
	e.ReplaceAllText("newer\r\n")
	e.NavigateToLineCol(0, 2)
	revision, line, column := e.TextRevision(), e.CursorLine(), e.CursorCol()
	e.MarkSnapshotSaved(captured)
	if e.Text() != "newer\r\n" || !e.Modified() || e.SavedTextSnapshot() != captured ||
		e.TextRevision() != revision || e.CursorLine() != line || e.CursorCol() != column {
		t.Fatal("saved snapshot overwrote newer buffer, caret or revision")
	}
	e.Undo()
	if e.Text() != captured || e.Modified() {
		t.Fatal("Undo to physically saved bytes is not clean")
	}
	e.Redo()
	if e.Text() != "newer\r\n" || !e.Modified() {
		t.Fatal("saved snapshot destroyed Redo or dirty state")
	}
}
