package ged

import (
	"reflect"

	"github.com/uk0/silk/gui"
)

// PendingEditVersion is an optional UI-only draft contract. The returned value
// must be comparable and change for every draft/binding revision (including
// Undo/Redo ABA). Unversioned dirty drafts cannot authorize delayed discard.
type PendingEditVersion interface{ PendingEditVersion() any }

type pendingEditCheckpoint struct {
	id        int
	pending   bool
	version   any
	versioned bool
}

// DesignEditCheckpoint is a UI-owned edit/draft identity, never worker data.
// Capturing and checking it does not commit/discard drafts or encode/read files.
type DesignEditCheckpoint struct {
	scene      *GedScene
	path       string
	revision   uint64
	pendingSeq int
	history    gui.UndoCheckpoint
	pending    []pendingEditCheckpoint
	valid      bool
}

func (s *GedScene) EditCheckpoint() DesignEditCheckpoint {
	c := DesignEditCheckpoint{scene: s, path: s.filename, revision: s.designRevision, pendingSeq: s.pendingEditSeq}
	stack, ok := s.UndoStack().(*gui.UndoStack)
	if !ok || s.saving || s.committingEdits {
		return c
	}
	c.history = stack.Checkpoint()
	c.valid = true
	for _, p := range s.pendingEditors {
		entry := pendingEditCheckpoint{id: p.id, pending: p.editor.HasPendingChanges()}
		if v, ok := p.editor.(PendingEditVersion); ok {
			entry.version = v.PendingEditVersion()
			entry.versioned = entry.version == nil || reflect.ValueOf(entry.version).Comparable()
		}
		c.pending = append(c.pending, entry)
	}
	c.valid = c.ModelCurrent(s)
	return c
}

// ModelCurrent ignores draft changes so an explicit Save choice may capture the
// latest draft independently. It never authorizes discarding changed drafts.
func (c DesignEditCheckpoint) ModelCurrent(s *GedScene) bool {
	return c.valid && s != nil && c.scene == s && !s.saving && !s.committingEdits && s.filename == c.path && s.designRevision == c.revision && c.history.IsCurrent(s.UndoStack())
}
func (c DesignEditCheckpoint) IsCurrent(s *GedScene) bool {
	if !c.ModelCurrent(s) || !c.draftsCurrent(s) {
		return false
	}
	return c.ModelCurrent(s)
}

func (c DesignEditCheckpoint) draftsCurrent(s *GedScene) bool {
	if c.pendingSeq != s.pendingEditSeq || len(c.pending) != len(s.pendingEditors) {
		return false
	}
	for i, entry := range c.pending {
		p := s.pendingEditors[i]
		if entry.id != p.id || entry.pending != p.editor.HasPendingChanges() {
			return false
		}
		if entry.versioned {
			v, ok := p.editor.(PendingEditVersion)
			if !ok {
				return false
			}
			now := v.PendingEditVersion()
			if now != nil && !reflect.ValueOf(now).Comparable() {
				return false
			}
			if entry.version != now {
				return false
			}
		} else if entry.pending {
			return false
		}
	}
	return true
}

// AfterSave rebases only the filename/clean marker of an actually adopted save.
// The host must call it only after CompleteSave succeeded for that output path.
// Every authored command, model notification and draft revision still refuses
// the rebase. This lets a waiting close resume without accepting later input.
func (c DesignEditCheckpoint) AfterSave(s *GedScene, path string) (DesignEditCheckpoint, bool) {
	if !c.valid || s == nil || c.scene != s || s.saving || s.committingEdits || s.filename != path || !s.IsClean() {
		return DesignEditCheckpoint{}, false
	}
	revision := c.revision
	if c.path != path {
		revision++
	}
	if s.designRevision != revision || !c.history.SameEdits(s.UndoStack()) || !c.draftsCurrent(s) {
		return DesignEditCheckpoint{}, false
	}
	c.path, c.revision = path, revision
	c.history = s.UndoStack().(*gui.UndoStack).Checkpoint()
	return c, c.IsCurrent(s)
}
