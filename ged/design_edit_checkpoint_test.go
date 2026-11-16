package ged

import (
	"github.com/uk0/silk/gui"
	"path/filepath"
	"testing"
)

func TestDesignEditCheckpointPreservesAndDetectsDrafts(t *testing.T) {
	v, p, widget := draftPanel(t)
	s := v.GedScene()
	p.HandlerEditor().ReplaceAllText("func onButtonClick(){println(1)}")
	c := s.EditCheckpoint()
	if !c.IsCurrent(s) || !p.HasPendingChanges() || widget.GetCode() != "" {
		t.Fatal("capture committed or lost pending draft")
	}
	p.HandlerEditor().ReplaceAllText("func onButtonClick(){println(2)}")
	if c.IsCurrent(s) || !c.ModelCurrent(s) {
		t.Fatal("draft revision not separated from model")
	}
	p.HandlerEditor().Undo()
	if c.IsCurrent(s) {
		t.Fatal("draft Undo ABA reused checkpoint")
	}
	current := s.EditCheckpoint()
	s.NotifyDesignChanged()
	if current.ModelCurrent(s) {
		t.Fatal("model notification not tracked")
	}
}

func TestDesignEditCheckpointRebaseOnlyAdoptedSaveChanges(t *testing.T) {
	for _, change := range []string{"none", "notify", "undo-redo", "draft"} {
		t.Run(change, func(t *testing.T) {
			v, p, _ := draftPanel(t)
			s := v.GedScene()
			s.PushCommand(gui.NewCompoundCommand())
			path := filepath.Join(t.TempDir(), "saved.silkui")
			receipt, err := s.PrepareSave(path)
			if err != nil {
				t.Fatal(err)
			}
			c := s.EditCheckpoint()
			if err := receipt.Writer().Write(); err != nil {
				t.Fatal(err)
			}
			if !s.CompleteSave(receipt) {
				t.Fatal("save not adopted")
			}
			switch change {
			case "notify":
				s.NotifyDesignChanged()
			case "undo-redo":
				s.UndoStack().Undo()
				s.UndoStack().Redo()
			case "draft":
				p.HandlerEditor().ReplaceAllText("func onButtonClick(){println(9)}")
			}
			next, ok := c.AfterSave(s, path)
			if ok != (change == "none") {
				t.Fatal("wrong rebase result", change, ok)
			}
			if ok && !next.IsCurrent(s) {
				t.Fatal("rebased checkpoint is stale")
			}
		})
	}
}

type checkpointUnversionedDraft struct{ pending bool }

func (d *checkpointUnversionedDraft) HasPendingChanges() bool     { return d.pending }
func (d *checkpointUnversionedDraft) CommitPendingChanges() error { d.pending = false; return nil }
func TestDesignEditCheckpointUnknownDirtyDraftFailsClosedForDiscard(t *testing.T) {
	s := NewGedScene()
	d := &checkpointUnversionedDraft{true}
	s.AddPendingEditor(d)
	c := s.EditCheckpoint()
	if !c.ModelCurrent(s) || c.IsCurrent(s) || !d.pending {
		t.Fatal("unknown draft authorized delayed discard or was committed")
	}
	if err := s.CommitPendingEdits(); err != nil {
		t.Fatal(err)
	}
	if !s.EditCheckpoint().IsCurrent(s) {
		t.Fatal("clean legacy provider blocked")
	}
}

type nestedCheckpointDraft struct{ version any }

func (*nestedCheckpointDraft) HasPendingChanges() bool     { return true }
func (*nestedCheckpointDraft) CommitPendingChanges() error { return nil }
func (d *nestedCheckpointDraft) PendingEditVersion() any   { return d.version }

func TestDesignEditCheckpointNestedNoncomparableDraftFailsClosed(t *testing.T) {
	s := NewGedScene()
	// A struct containing any is a comparable TYPE, but this VALUE cannot be
	// compared when its interface field contains a slice. Reject without panic.
	d := &nestedCheckpointDraft{version: struct{ Value any }{[]int{1}}}
	s.AddPendingEditor(d)
	if c := s.EditCheckpoint(); !c.ModelCurrent(s) || c.IsCurrent(s) {
		t.Fatal("invalid draft version authorized delayed discard")
	}
	d.version = struct{ Value any }{"valid"}
	c := s.EditCheckpoint()
	if !c.IsCurrent(s) {
		t.Fatal("valid comparable value refused")
	}
	d.version = struct{ Value any }{[]int{1}}
	if c.IsCurrent(s) {
		t.Fatal("changed invalid token accepted")
	}
}
