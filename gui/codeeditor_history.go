package gui

import "unicode/utf8"

// A committed multi-rune input (IME, paste or SDK call) is an atomic edit,
// not the beginning of a run that later keystrokes may silently absorb.
func editorTypingInput(text string) bool {
	r, width := utf8.DecodeRuneInString(text)
	return width > 0 && width == len(text) && r != '\r' && r != '\n'
}

// Command/navigation boundaries must not merge with adjacent typing. This is
// UI-thread state only: no timers, polling, document copies or notifications.
func (e *CodeEditor) breakTypingUndoGroup() {
	if n := len(e.undoStack); n > 0 {
		e.undoStack[n-1].typing = false
	}
}

// CanUndo and CanRedo query the edit history in constant time, independently of
// Modified (saved content can still have undo history). Read-only mode hides
// availability without clearing stored history. UI-thread-only.
func (e *CodeEditor) CanUndo() bool { return !e.readOnly && len(e.undoStack) != 0 }
func (e *CodeEditor) CanRedo() bool { return !e.readOnly && len(e.redoStack) != 0 }

type editorHistoryObserver struct{ notify func() }

// SubscribeHistoryChanged observes changes to CanUndo/CanRedo after a complete
// edit, Undo/Redo, SetText reset or read-only transition. It does not replace
// SigChanged or publish an initial event: read CanUndo/CanRedo when binding. Save/caret moves and edits
// that preserve both booleans are silent. Callbacks run on the UI thread and
// must be short and not synchronously mutate text. Multiple observers run in
// registration order; the returned unsubscribe is idempotent. Call it when the
// consumer detaches. Subscriptions and queries are UI-thread-only too.
func (e *CodeEditor) SubscribeHistoryChanged(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	o := &editorHistoryObserver{fn}
	e.historyObservers = append(e.historyObservers, o)
	return func() {
		if o.notify == nil {
			return
		}
		o.notify = nil
		for i, current := range e.historyObservers {
			if current == o {
				copy(e.historyObservers[i:], e.historyObservers[i+1:])
				last := len(e.historyObservers) - 1
				e.historyObservers[last] = nil
				e.historyObservers = e.historyObservers[:last]
				if last == 0 {
					e.historyObservers = nil
				}
				return
			}
		}
	}
}
func (e *CodeEditor) notifyHistoryChanged() {
	undo, redo := e.CanUndo(), e.CanRedo()
	if undo == e.historyHadUndo && redo == e.historyHadRedo {
		return
	}
	e.historyHadUndo, e.historyHadRedo = undo, redo
	// Snapshot identities, not callbacks: unsubscribed listeners are skipped and
	// new subscriptions begin at the next transition. No allocation on idle/typing
	// when availability is unchanged, or when no one observes this editor.
	observers := append([]*editorHistoryObserver(nil), e.historyObservers...)
	for _, o := range observers {
		if o.notify != nil {
			o.notify()
		}
	}
}
