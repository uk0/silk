package async

import (
	"context"
	"errors"
	"sync"
)

var ErrSuperseded = errors.New("async: queued write superseded by a newer request")

type writeTask[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	work   func(context.Context) (T, error)
	future *Future[T]
}

// WriteQueue serializes physical side effects with one active task and one
// replaceable latest queued task. Use one shared queue for all writers that
// must be ordered (e.g. primary saves and recovery). Submit creates no unbounded
// waiting pool; superseded queued futures explicitly finish with ErrSuperseded.
//
// Unlike Executor/Run, cancellation AFTER work returns does not replace a real
// transaction result with context.Canceled: disk may already have committed.
// Cancellation is cooperative inside work and never frees an active slot until
// that work actually returns, panics or exits. A queued canceled task remains
// bounded and is skipped when reached, superseded or closed. UI hosts retire
// their own receipts immediately; they must not await this queue on UI.
//
// Work and results must contain immutable data, not widgets. OnComplete should
// dispatch via a revocable UI mailbox; callbacks/dispatchers must not block or
// panic. The queue owns no UI, starts no idle polling and creates no goroutine
// while idle. Do not copy a queue after use.
// The zero value is ready for use; NewWriteQueue is a convenience constructor.
type WriteQueue[T any] struct {
	mu             sync.Mutex
	active, queued *writeTask[T]
	idle           chan struct{}
	closed         bool
}

type WriteQueueStats struct{ Active, Queued, Closed bool }

func NewWriteQueue[T any]() *WriteQueue[T] {
	idle := make(chan struct{})
	close(idle)
	return &WriteQueue[T]{idle: idle}
}

func (q *WriteQueue[T]) Stats() WriteQueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()
	return WriteQueueStats{q.active != nil, q.queued != nil, q.closed}
}

func (q *WriteQueue[T]) Submit(ctx context.Context, work func(context.Context) (T, error)) (*Future[T], error) {
	if ctx == nil || work == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	child, cancel := context.WithCancel(ctx)
	t := &writeTask[T]{ctx: child, cancel: cancel, work: work}
	t.future = &Future[T]{done: make(chan struct{}), cancel: cancel}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		cancel()
		return nil, ErrClosed
	}
	var replaced *writeTask[T]
	start := q.active == nil
	if start {
		q.active = t
		q.idle = make(chan struct{})
	} else {
		replaced, q.queued = q.queued, t
	}
	q.mu.Unlock()
	// Complete outside the lock: dispatch may submit another task or inspect
	// Stats. Even supersession never calls user callbacks under the queue lock.
	if replaced != nil {
		replaced.cancel()
		var zero T
		completeWriteFuture(replaced.future, zero, ErrSuperseded)
	}
	if start {
		go q.run(t)
	}
	return t.future, nil
}

func (q *WriteQueue[T]) run(t *writeTask[T]) {
	var value T
	err := ErrAborted // also retained when work calls runtime.Goexit
	defer func() {
		if p := recover(); p != nil {
			var zero T
			value = zero
			err = &PanicError{Value: p}
		}
		t.cancel()
		// Keep the same busy interval through completion dispatch. A submit
		// from an observer must join this interval, not replace its idle channel
		// and strand waiters which captured the old one.
		defer q.advance(t)
		completeWriteFuture(t.future, value, err)
	}()
	if e := t.ctx.Err(); e != nil {
		err = e
		return
	}
	value, err = t.work(t.ctx)
}

func (q *WriteQueue[T]) advance(t *writeTask[T]) {
	q.mu.Lock()
	if q.active != t {
		q.mu.Unlock()
		return
	}
	next := q.queued
	q.queued, q.active = nil, next
	if next == nil {
		close(q.idle)
	}
	q.mu.Unlock()
	if next != nil {
		go q.run(next)
	}
}

func completeWriteFuture[T any](f *Future[T], value T, err error) {
	f.mu.Lock()
	if f.ready {
		f.mu.Unlock()
		return
	}
	f.value, f.err, f.ready = value, err, true
	observers := f.observers
	f.observers = nil
	close(f.done)
	f.mu.Unlock()
	for _, observe := range observers {
		observe(value, err)
	}
}

// Close permanently rejects new work, drops an unstarted queued task and signals
// active cancellation, but never waits or discards the active transaction result.
// Call Wait outside UI if shutdown must join physical work and retain failures.
func (q *WriteQueue[T]) Close() {
	q.mu.Lock()
	q.closed = true
	active, queued := q.active, q.queued
	q.queued = nil
	q.mu.Unlock()
	if active != nil {
		active.cancel()
	}
	if queued != nil {
		queued.cancel()
		var zero T
		completeWriteFuture(queued.future, zero, ErrClosed)
	}
}

// Wait observes the busy interval at entry; Close first to stop new admissions.
// It does not run GUI callbacks or cancel a physically blocked OS operation.
func (q *WriteQueue[T]) Wait(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	q.mu.Lock()
	idle := q.idle
	q.mu.Unlock()
	if idle == nil { // zero value, never admitted a task
		return nil
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
