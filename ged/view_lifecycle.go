package ged

// View lifecycle, like widget access, is UI-thread-only. A canceled handle must
// not keep a retired view or the workbench captured by its callback alive.
type viewCloseSubscription struct {
	view *GedView
	fn   func()
}

// IsClosed reports whether Close has begun. A closed view cannot be reopened;
// create a fresh GedView for a new document viewport.
func (v *GedView) IsClosed() bool { return v == nil || v.closed }

// SubscribeClosed observes the first Close before pending-editor teardown and
// the legacy DocClosedCallback. The returned cancel function is idempotent.
// Cancellation and Close sever both callback and owner references. Subscribing
// during/after Close is inert, not an immediate or queued callback. UI-only.
func (v *GedView) SubscribeClosed(fn func()) func() {
	if v.IsClosed() || fn == nil {
		return func() {}
	}
	s := &viewCloseSubscription{view: v, fn: fn}
	v.closeSubscriptions = append(v.closeSubscriptions, s)
	return s.cancel
}

func (s *viewCloseSubscription) cancel() {
	v := s.view
	s.view, s.fn = nil, nil
	if v == nil {
		return
	}
	for i, entry := range v.closeSubscriptions {
		if entry == s {
			copy(v.closeSubscriptions[i:], v.closeSubscriptions[i+1:])
			v.closeSubscriptions[len(v.closeSubscriptions)-1] = nil
			v.closeSubscriptions = v.closeSubscriptions[:len(v.closeSubscriptions)-1]
			return
		}
	}
}

func (v *GedView) beginClose() bool {
	if v.IsClosed() {
		return false
	}
	v.closed = true
	subs := v.closeSubscriptions
	v.closeSubscriptions = nil
	for _, s := range subs {
		fn := s.fn
		s.view, s.fn = nil, nil
		if fn != nil {
			fn()
		}
	}
	return true
}
