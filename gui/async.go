package gui

import (
	"context"
	"github.com/uk0/silk/async"
)

// RunAsync starts bounded background work and posts completion to the UI queue.
// Work must not access widgets. Callers own lifecycle: cancel on close and guard
// stale completions, which may already be queued when Cancel is called. A nil
// completion is allowed when the caller only needs the Future.
func RunAsync[T any](ctx context.Context, executor *async.Executor, work func(context.Context) (T, error), complete func(T, error)) (*async.Future[T], error) {
	f, err := async.Submit(ctx, executor, work)
	if err != nil {
		return nil, err
	}
	if complete != nil {
		_ = f.OnComplete(Post, complete)
	}
	return f, nil
}
