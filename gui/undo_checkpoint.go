package gui

// UndoCheckpoint is an opaque UI-thread-only history identity. It must NOT be
// passed to a worker: it retains the stack and its live commands/actions. Capture
// immutable work separately, then check the checkpoint on UI before publishing.
// Numeric Count/Current alone can be reused by Undo/Redo, branch and merge.
type UndoCheckpoint struct {
	stack        *UndoStack
	revision     uint64
	editRevision uint64
	stable       bool
}

// Checkpoint invalidates on effective Push (including merge), Undo, Redo, Clear
// and clean-marker changes. A checkpoint taken inside a command is never valid.
// Revision values are opaque and need not advance by exactly one per command.
func (s *UndoStack) Checkpoint() UndoCheckpoint {
	return UndoCheckpoint{stack: s, revision: s.revision, editRevision: s.editRevision, stable: s.executing == 0}
}

// SameEdits ignores saved-position changes only. Push/merge, Undo/Redo ABA,
// Clear, another stack and an executing command still invalidate it. UI only;
// this is not a substitute for IsCurrent when authorizing delayed discard.
func (c UndoCheckpoint) SameEdits(stack IUndoStack) bool {
	s, ok := stack.(*UndoStack)
	return ok && s != nil && c.stable && c.stack == s && c.editRevision == s.editRevision && s.executing == 0
}

// IsCurrent is UI-thread-only and never waits for a worker. Other stack
// implementations are deliberately not assumed to share this identity.
func (c UndoCheckpoint) IsCurrent(stack IUndoStack) bool {
	s, ok := stack.(*UndoStack)
	return ok && s != nil && c.stable && c.stack == s && c.revision == s.revision && s.executing == 0
}
