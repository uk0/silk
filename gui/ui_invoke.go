package gui

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/uk0/silk/async"
)

var (
	ErrUIThread      = errors.New("gui: operation requires the UI thread; use gui.Post or gui.Invoke")
	ErrUIUnavailable = errors.New("gui: UI thread identity is unavailable")
	ErrUIInvalid     = errors.New("gui: nil context or callback")
)

// IsUIThread reports whether the caller owns the native event loop. It is safe
// to query from a worker; false also means no backend has installed an owner.
// This does not imply that an event loop is currently running.
func IsUIThread() bool {
	owner := uiThreadOwner
	return owner != nil && owner()
}

// UIInvocationPanicError reports a panic without unwinding the native event loop.
// It is returned to the Invoke caller, unlike fire-and-forget Post's log report.
type UIInvocationPanicError struct{ Value any }

func (e *UIInvocationPanicError) Error() string {
	return fmt.Sprintf("gui: invoked callback panicked: %v", e.Value)
}
func (e *UIInvocationPanicError) Unwrap() error { err, _ := e.Value.(error); return err }

type uiInvocationResult[T any] struct {
	value T
	err   error
}
type uiInvocation[T any] struct {
	mu   sync.Mutex
	fn   func() (T, error)
	ctx  context.Context
	done chan uiInvocationResult[T]
}

func invokeUIValue[T any](fn func() (T, error)) (value T, err error) {
	defer func() {
		if p := recover(); p != nil {
			var zero T
			value = zero
			err = &UIInvocationPanicError{Value: p}
		}
	}()
	return fn()
}
func (c *uiInvocation[T]) run() {
	c.mu.Lock()
	fn, ctx := c.fn, c.ctx
	c.fn, c.ctx = nil, nil // release captured UI objects before running user code
	c.mu.Unlock()
	if fn == nil {
		return
	}
	if err := ctx.Err(); err != nil {
		c.done <- uiInvocationResult[T]{err: err}
		return
	}
	value, err := invokeUIValue(fn)
	c.done <- uiInvocationResult[T]{value, err}
}
func (c *uiInvocation[T]) retire() {
	c.mu.Lock()
	c.fn, c.ctx = nil, nil
	c.mu.Unlock()
}

// InvokeValue synchronously marshals a short operation to the UI thread and
// returns a typed result. On the UI thread it runs inline, including reentrant
// calls. Workers enqueue once using Post; no polling or helper goroutine is used.
//
// Use a deadline: an unstarted/stopped event loop cannot service queued work.
// Cancellation retires work not yet claimed by the UI and releases its captured
// callback. Already-started code cannot be interrupted and may finish after this
// call returns ctx.Err(). It must own its inputs and must not write worker stack
// variables; return a value instead. Do not block the UI waiting for the calling
// worker, and do not call runtime.Goexit in callbacks.
func InvokeValue[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var zero T
	if ctx == nil || fn == nil {
		return zero, ErrUIInvalid
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if uiThreadOwner == nil {
		return zero, ErrUIUnavailable
	}
	if IsUIThread() {
		return invokeUIValue(fn)
	}
	call := &uiInvocation[T]{fn: fn, ctx: ctx, done: make(chan uiInvocationResult[T], 1)}
	Post(call.run)
	select {
	case result := <-call.done:
		return result.value, result.err
	case <-ctx.Done():
		call.retire()
		return zero, ctx.Err()
	}
}

// Invoke is InvokeValue for operations without a result. Callbacks must be
// short; put HTTP, disk IO and expensive computation in RunAsync instead.
func Invoke(ctx context.Context, fn func() error) error {
	if fn == nil {
		return ErrUIInvalid
	}
	_, err := InvokeValue(ctx, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

// PostContext queues fn for a future UI turn, even when called on the UI thread.
// A context cancelled before delivery suppresses it. No completion is returned;
// panics follow Post's logging policy. The queue retains the closure until its
// turn, so use Invoke with a deadline when prompt cancellation release matters.
func PostContext(ctx context.Context, fn func()) error {
	if ctx == nil || fn == nil {
		return ErrUIInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	Post(func() {
		if ctx.Err() == nil {
			fn()
		}
	})
	return nil
}

// NewLatestUI coalesces replaceable worker snapshots into UI deliveries. At most
// one callback is queued or running per instance. It adds no timer or goroutine.
// Close on the UI thread before disposing the receiving widget. Values must be
// immutable snapshots; use Post for events which must never be discarded.
func NewLatestUI[T any](consume func(T)) (*async.Latest[T], error) {
	return async.NewLatest(Post, consume)
}
