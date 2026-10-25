package gui

type editorLineEndingObserver struct{ notify func() }

// SubscribeLineEndingsChanged observes actual separator counts and the preferred
// Enter style after a complete edit, load, conversion or history replay. It does
// not replace SigChanged, emit an initial notification, or turn a preference-only
// change into a text edit. Read LineEndings when binding. UI-thread-only; callbacks
// must be short and must not synchronously mutate the document. Unsubscribe when
// detaching; cancellation is idempotent and can safely occur inside a callback.
func (e *CodeEditor) SubscribeLineEndingsChanged(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	o := &editorLineEndingObserver{fn}
	e.lineEndingObservers = append(e.lineEndingObservers, o)
	return func() {
		if o.notify == nil {
			return
		}
		o.notify = nil
		for i, current := range e.lineEndingObservers {
			if current == o {
				copy(e.lineEndingObservers[i:], e.lineEndingObservers[i+1:])
				last := len(e.lineEndingObservers) - 1
				e.lineEndingObservers[last] = nil
				e.lineEndingObservers = e.lineEndingObservers[:last]
				if last == 0 {
					e.lineEndingObservers = nil
				}
				return
			}
		}
	}
}
func (e *CodeEditor) notifyLineEndingsChanged() {
	state := e.LineEndings()
	if state == e.lastLineEndingInfo {
		return
	}
	e.lastLineEndingInfo = state
	for _, o := range append([]*editorLineEndingObserver(nil), e.lineEndingObservers...) {
		if o.notify != nil {
			o.notify()
		}
	}
}
