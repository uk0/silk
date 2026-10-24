package async

import "sync"

// Latest coalesces replaceable snapshots (progress, metrics, previews). At most
// one delivery is queued or running, regardless of the number of Publish calls.
// It is NOT an event log: intermediate values may be discarded.
//
// dispatch must enqueue each callback exactly once, must not panic, and should
// not block (gui.Post is suitable). consume runs on that dispatcher. No timers
// or goroutines are created. Values must be immutable or owned by the receiver.
// Do not copy Latest after use.
type Latest[T any] struct {
	mu       sync.Mutex
	dispatch func(func())
	consume  func(T)
	value    T
	pending  bool
	dirty    bool
	closed   bool
}

func NewLatest[T any](dispatch func(func()), consume func(T)) (*Latest[T], error) {
	if dispatch == nil || consume == nil {
		return nil, ErrInvalid
	}
	return &Latest[T]{dispatch: dispatch, consume: consume}, nil
}

// Publish replaces the pending value and reports whether it was accepted.
// The first value posts a delivery; further publishes reuse that slot.
func (l *Latest[T]) Publish(value T) bool {
	l.mu.Lock()
	if l.closed || l.dispatch == nil {
		l.mu.Unlock()
		return false
	}
	l.value, l.dirty = value, true
	if l.pending {
		l.mu.Unlock()
		return true
	}
	l.pending = true
	dispatch := l.dispatch
	l.mu.Unlock()
	dispatch(l.deliver)
	return true
}

func (l *Latest[T]) deliver() {
	l.mu.Lock()
	if l.closed {
		l.pending = false
		l.mu.Unlock()
		return
	}
	value, consume := l.value, l.consume
	var zero T
	l.value, l.dirty = zero, false
	l.mu.Unlock()
	defer l.delivered() // a panicking consumer must not wedge later deliveries
	consume(value)
}

func (l *Latest[T]) delivered() {
	l.mu.Lock()
	if l.closed || !l.dirty {
		l.pending = false
		l.mu.Unlock()
		return
	}
	dispatch := l.dispatch
	l.mu.Unlock()
	// A publish during consume gets a separate turn, rather than a draining
	// loop which could indefinitely starve input/painting.
	dispatch(l.deliver)
}

// Close rejects future publishes and suppresses not-yet-started deliveries.
// An already-started consume may finish; Close does not wait for it. For UI
// owners, close on the UI thread before disposal. Pending payload/callback
// references are released immediately. A closed Latest cannot be reused.
func (l *Latest[T]) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed, l.dirty = true, false
	var zero T
	l.value = zero
	l.consume, l.dispatch = nil, nil
}
