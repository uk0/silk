package jsonx

import (
	"bytes"
	"context"
	"fmt"

	"github.com/uk0/silk/async"
)

// ParseAsync snapshots bounded input before submitting to the bounded executor.
// Cancellation is checked before/after the bounded parse, not per JSON token.
// Use Future.OnComplete with an explicit dispatcher (gui.Post for UI callbacks).
func ParseAsync(ctx context.Context, executor *async.Executor, data []byte, options Options) (*async.Future[*Document], error) {
	if options.MaxBytes < 0 || options.MaxNodes < 0 || options.MaxDepth < 0 {
		return nil, fmt.Errorf("jsonx: limits must be nonnegative")
	}
	maxBytes := options.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}
	if len(data) > maxBytes {
		return nil, ErrLimit
	}
	snapshot := bytes.Clone(data)
	return async.Submit(ctx, executor, func(ctx context.Context) (*Document, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d, err := ParseWithOptions(snapshot, options)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return d, err
	})
}
