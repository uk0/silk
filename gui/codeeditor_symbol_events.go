package gui

type editorSymbolObserver struct{ notify func() }

// SubscribeSymbolsChanged observes text/provider/index changes on the UI thread.
// Consumers should invalidate their own paint region, then read SymbolsSnapshot
// when drawing; repainting the editor does not invalidate independently cached
// sibling panels. The returned unsubscribe is idempotent and must be called when
// detaching a consumer. Multiple observers are supported in subscription order.
// Callbacks must be short and must not synchronously modify the editor text.
func (e *CodeEditor) SubscribeSymbolsChanged(fn func()) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	observer := &editorSymbolObserver{notify: fn}
	e.symbolObservers = append(e.symbolObservers, observer)
	return func() {
		if observer.notify == nil {
			return
		}
		observer.notify = nil
		for i, current := range e.symbolObservers {
			if current == observer {
				copy(e.symbolObservers[i:], e.symbolObservers[i+1:])
				last := len(e.symbolObservers) - 1
				e.symbolObservers[last] = nil
				e.symbolObservers = e.symbolObservers[:last]
				if last == 0 {
					e.symbolObservers = nil
				}
				return
			}
		}
	}
}

func (e *CodeEditor) notifySymbolsChanged() {
	// Snapshot identities: callbacks may unsubscribe themselves/others or attach
	// new observers. New observers start with the next event; removed ones do not
	// receive an already-captured event. Never retain consumer closures in a queue.
	observers := append([]*editorSymbolObserver(nil), e.symbolObservers...)
	for _, observer := range observers {
		if observer.notify != nil {
			observer.notify()
		}
	}
}
