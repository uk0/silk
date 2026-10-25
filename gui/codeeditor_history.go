package gui

// CanUndo and CanRedo query the edit history in constant time, independently of
// Modified (saved content can still have undo history). UI-thread-only.
func (e *CodeEditor) CanUndo() bool { return len(e.undoStack) != 0 }
func (e *CodeEditor) CanRedo() bool { return len(e.redoStack) != 0 }

type editorHistoryObserver struct{ notify func() }

// SubscribeHistoryChanged observes changes to CanUndo/CanRedo after a complete
// edit, Undo/Redo or SetText reset. It does not replace SigChanged or publish an
// initial event: read CanUndo/CanRedo when binding. Save/caret moves and edits
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
