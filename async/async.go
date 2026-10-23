// Package async provides bounded, cancellable work without a GUI dependency.
package async

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrBusy    = errors.New("async: executor capacity exhausted")
	ErrClosed  = errors.New("async: executor closed")
	ErrAborted = errors.New("async: work exited without returning")
	ErrInvalid = errors.New("async: context, work and completion dispatcher must not be nil")
)

// PanicError turns a work-function panic into a failed result. Completion
// dispatchers/callbacks remain caller-owned and must not panic or block.
type PanicError struct{ Value any }

func (e *PanicError) Error() string { return fmt.Sprintf("async: work panicked: %v", e.Value) }

// Executor admits at most limit jobs. There is no unbounded waiting queue.
// Close cancels admitted work; Wait waits for cooperative work to return.
type Executor struct {
	mu     sync.Mutex
	limit  int
	closed bool
	next   uint64
	active map[uint64]context.CancelFunc
	idle   chan struct{}
}

func NewExecutor(limit int) *Executor {
	if limit < 1 {
		panic("async: concurrency limit must be positive")
	}
	idle := make(chan struct{})
	close(idle)
	return &Executor{limit: limit, active: make(map[uint64]context.CancelFunc), idle: idle}
}

var defaultExecutor = NewExecutor(8)

// Run executes synchronously. Cancellation is cooperative: Go cannot forcibly
// stop a function which ignores its context. No goroutine is created here.
func Run[T any](ctx context.Context, work func(context.Context) (T, error)) (value T, err error) {
	if ctx == nil || work == nil {
		return value, ErrInvalid
	}
	if err = ctx.Err(); err != nil {
		return value, err
	}
	defer func() {
		if p := recover(); p != nil {
			var zero T
			value = zero
			err = &PanicError{Value: p}
		}
	}()
	value, err = work(ctx)
	if err == nil && ctx.Err() != nil {
		var zero T
		return zero, ctx.Err()
	}
	return
}

// Future is safe for concurrent Await, Cancel and OnComplete calls. Done means
// work returned, not that GUI callbacks have been dispatched. Values containing
// maps/slices/pointers must be treated as immutable or synchronized by the caller.
type Future[T any] struct {
	mu        sync.Mutex
	done      chan struct{}
	cancel    context.CancelFunc
	value     T
	err       error
	ready     bool
	observers []func(T, error)
}

// Submit snapshots no caller data: capture immutable values in work. A nil
// executor uses the shared eight-job executor. Rejection creates no goroutine.
func Submit[T any](ctx context.Context, executor *Executor, work func(context.Context) (T, error)) (*Future[T], error) {
	if ctx == nil || work == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if executor == nil {
		executor = defaultExecutor
	}
	executor.mu.Lock()
	if executor.closed {
		executor.mu.Unlock()
		return nil, ErrClosed
	}
	if len(executor.active) >= executor.limit {
		executor.mu.Unlock()
		return nil, ErrBusy
	}
	child, cancel := context.WithCancel(ctx)
	executor.next++
	id := executor.next
	if len(executor.active) == 0 {
		executor.idle = make(chan struct{})
	}
	executor.active[id] = cancel
	executor.mu.Unlock()
	f := &Future[T]{done: make(chan struct{}), cancel: cancel}
	go func() {
		var value T
		err := ErrAborted
		defer func() {
			cancel()
			executor.mu.Lock()
			delete(executor.active, id)
			if len(executor.active) == 0 {
				close(executor.idle)
			}
			executor.mu.Unlock()
			f.mu.Lock()
			f.value, f.err, f.ready = value, err, true
			observers := f.observers
			f.observers = nil
			close(f.done)
			f.mu.Unlock()
			for _, observe := range observers {
				observe(value, err)
			}
		}()
		value, err = Run(child, work)
	}()
	return f, nil
}
func (f *Future[T]) Done() <-chan struct{} { return f.done }
func (f *Future[T]) Cancel()               { f.cancel() }

// Await timing out does not cancel the underlying job; call Cancel explicitly.
func (f *Future[T]) Await(ctx context.Context) (T, error) {
	var zero T
	if ctx == nil {
		return zero, ErrInvalid
	}
	select {
	case <-f.done:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.value, f.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// OnComplete registers one exactly-once notification, without a watcher
// goroutine. Use gui.Post as dispatcher for UI callbacks. An already-complete
// future dispatches immediately; callback execution order is dispatcher-owned.
func (f *Future[T]) OnComplete(dispatch func(func()), callback func(T, error)) error {
	if dispatch == nil || callback == nil {
		return ErrInvalid
	}
	observe := func(value T, err error) { dispatch(func() { callback(value, err) }) }
	f.mu.Lock()
	if !f.ready {
		f.observers = append(f.observers, observe)
		f.mu.Unlock()
		return nil
	}
	value, err := f.value, f.err
	f.mu.Unlock()
	observe(value, err)
	return nil
}
func (e *Executor) Close() {
	e.mu.Lock()
	e.closed = true
	cancel := make([]context.CancelFunc, 0, len(e.active))
	for _, fn := range e.active {
		cancel = append(cancel, fn)
	}
	e.mu.Unlock()
	for _, fn := range cancel {
		fn()
	}
}

// Wait observes the current busy interval. Close first to prevent new admissions.
func (e *Executor) Wait(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	e.mu.Lock()
	idle := e.idle
	e.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
