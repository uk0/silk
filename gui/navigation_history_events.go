package gui

type navigationObserver struct{ notify func() }

// Len reports the number of retained positions (at most 256), including the
// current entry. It does not inspect the filesystem. UI-thread-only.
func (ns *NavigationStack) Len() int { return len(ns.stack) }

// Clear releases all positions without changing editor text or caret state.
// Observers remain attached for subsequent jumps. Clearing an empty stack is
// silent. Like navigation and subscriptions, Clear is UI-thread-only.
func (ns *NavigationStack) Clear() {
	if len(ns.stack) == 0 {
		return
	}
	ns.stack = nil
	ns.current = -1
	ns.notifyChanged()
}

// SubscribeChanged observes completed history mutations in registration order.
// RecordJump and MapPaths each emit once; failed/boundary navigation, exact
// duplicate Push and empty Clear are silent. No initial event is sent: query
// Len/CanGoBack/CanGoForward when binding. Callback and subscription operations
// run on the UI thread; callbacks must be short and must not synchronously
// mutate the stack. Subscribe/unsubscribe during delivery is safe: removals are
// immediate, additions start with the next event. Unsubscribe is idempotent and
// should be called when the consumer detaches. There is no timer or polling.
func (ns *NavigationStack) SubscribeChanged(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	o := &navigationObserver{fn}
	// Copy membership only on bind/unbind, never on navigation. In-flight delivery
	// keeps its own slice while removals invalidate the observer identity.
	next := make([]*navigationObserver, len(ns.observers)+1)
	copy(next, ns.observers)
	next[len(ns.observers)] = o
	ns.observers = next
	return func() {
		if o.notify == nil {
			return
		}
		o.notify = nil
		for i, current := range ns.observers {
			if current == o {
				var next []*navigationObserver
				if len(ns.observers) > 1 {
					next = make([]*navigationObserver, 0, len(ns.observers)-1)
					next = append(next, ns.observers[:i]...)
					next = append(next, ns.observers[i+1:]...)
				}
				ns.observers = next
				return
			}
		}
	}
}

func (ns *NavigationStack) notifyChanged() {
	for _, o := range ns.observers {
		if o.notify != nil {
			o.notify()
		}
	}
}
