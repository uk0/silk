package ged

import (
	"fmt"
	"github.com/uk0/silk/core"
)

// PendingDesignEdit connects a UI-owned draft to its document. All methods run
// on the UI thread; no polling goroutine or timer is needed.
type PendingDesignEdit interface {
	HasPendingChanges() bool
	CommitPendingChanges() error
}

type pendingDesignEdit struct {
	id     int
	editor PendingDesignEdit
}

func (s *GedScene) AddPendingEditor(editor PendingDesignEdit) int {
	if editor == nil {
		return 0
	}
	s.pendingEditSeq++
	s.pendingEditors = append(s.pendingEditors, pendingDesignEdit{s.pendingEditSeq, editor})
	return s.pendingEditSeq
}

func (s *GedScene) RemovePendingEditor(id int) {
	for i, entry := range s.pendingEditors {
		if entry.id == id {
			entries := append([]pendingDesignEdit(nil), s.pendingEditors[:i]...)
			s.pendingEditors = append(entries, s.pendingEditors[i+1:]...)
			return
		}
	}
}

// IsClean includes drafts that have not yet reached the undo stack.
func (s *GedScene) IsClean() bool {
	if !s.SceneItem.IsClean() {
		return false
	}
	for _, entry := range s.pendingEditors {
		if entry.editor.HasPendingChanges() {
			return false
		}
	}
	return true
}

// CommitPendingEdits materializes drafts as undoable commands, not disk writes.
// Failed commits leave the document dirty and prevent save/export from claiming
// success. Re-entry from a draft's PushCommand is deliberately a no-op.
func (s *GedScene) CommitPendingEdits() error {
	if s.committingEdits {
		return nil
	}
	s.committingEdits = true
	defer func() { s.committingEdits = false }()
	for _, entry := range s.pendingEditors {
		if entry.editor.HasPendingChanges() {
			if err := entry.editor.CommitPendingChanges(); err != nil {
				return err
			}
			if entry.editor.HasPendingChanges() {
				return fmt.Errorf("design editor did not commit its pending changes")
			}
		}
	}
	return nil
}

// SaveDesignWithPendingEdits is the snapshot API for save/export/recovery.
// SaveDesign remains a side-effect-free snapshot of committed model state.
func (s *GedScene) SaveDesignWithPendingEdits() (*core.TDoc, error) {
	if err := s.CommitPendingEdits(); err != nil {
		return nil, err
	}
	return s.SaveDesign(), nil
}

func (s *GedScene) containsWidget(w *FakeWidget) bool {
	for item := w; item != nil; {
		parent := item.Parent()
		if parent == s {
			return true
		}
		item, _ = parent.(*FakeWidget)
	}
	return false
}

func (s *GedScene) Title() string {
	title := s.SceneItem.Title()
	if s.SceneItem.IsClean() && !s.IsClean() {
		title += " *"
	}
	return title
}

func (s *GedScene) DirtyList() []string {
	if s.IsClean() {
		return nil
	}
	return []string{s.Title()}
}

// DiscardPendingEdits is for an explicitly accepted discard/close transition.
func (s *GedScene) DiscardPendingEdits() {
	for _, entry := range s.pendingEditors {
		if editor, ok := entry.editor.(interface{ DiscardPendingChanges() }); ok {
			editor.DiscardPendingChanges()
		}
	}
}
