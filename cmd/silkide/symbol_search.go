package main

import (
	"context"
	"sync"
	"time"

	"github.com/uk0/silk/core"
)

type symbolSearchQuery struct {
	client     *core.LSPClient
	text       string
	generation uint64
}
type symbolSearchResult struct {
	query   symbolSearchQuery
	symbols []core.LSPWorkspaceSymbol
	err     error
}
type symbolSearchRequest func(context.Context, symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error)

// A dialog owns one worker, one latest pending query and one posted UI delivery.
// update/close never wait for RPC or touch the transport. request runs only on
// the worker; post must schedule delivery on the GUI thread (not run it inline).
// A non-cooperative transport cannot create an unbounded worker/request backlog.
// Native LSP writes honor cancellation; arbitrary injected request functions
// may still require their owner to release non-cooperative I/O.
type symbolSearchWorker struct {
	mu               sync.Mutex
	latest           symbolSearchQuery
	changed          time.Time
	pending, closed  bool
	cancel           context.CancelFunc
	result           *symbolSearchResult
	posted           bool
	wake, stop, done chan struct{}
	debounce         time.Duration
	request          symbolSearchRequest
	post             func(func())
	deliver          func(symbolSearchResult)
}

func newSymbolSearchWorker(debounce time.Duration, request symbolSearchRequest, post func(func()), deliver func(symbolSearchResult)) *symbolSearchWorker {
	w := &symbolSearchWorker{debounce: debounce, request: request, post: post, deliver: deliver,
		wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go w.run()
	return w
}

func (w *symbolSearchWorker) update(client *core.LSPClient, query string) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.latest = symbolSearchQuery{client: client, text: query, generation: w.latest.generation + 1}
	w.changed, w.pending, w.result = time.Now(), client != nil, nil
	cancel := w.cancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *symbolSearchWorker) close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed, w.pending, w.result = true, false, nil
	w.latest = symbolSearchQuery{}
	w.deliver = nil
	cancel := w.cancel
	close(w.stop)
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (w *symbolSearchWorker) run() {
	defer close(w.done)
	for {
		select {
		case <-w.stop:
			return
		case <-w.wake:
		}
		for {
			w.mu.Lock()
			if w.closed {
				w.mu.Unlock()
				return
			}
			if !w.pending {
				w.mu.Unlock()
				break
			}
			remaining := w.debounce - time.Since(w.changed)
			if remaining > 0 {
				w.mu.Unlock()
				timer := time.NewTimer(remaining)
				select {
				case <-w.stop:
					timer.Stop()
					return
				case <-w.wake:
					timer.Stop()
				case <-timer.C:
				}
				continue
			}
			q := w.latest
			w.pending = false
			ctx, cancel := context.WithCancel(context.Background())
			w.cancel = cancel
			w.mu.Unlock()
			symbols, err := w.request(ctx, q)
			cancel()
			w.mu.Lock()
			w.cancel = nil
			post := false
			if !w.closed && q.generation == w.latest.generation {
				w.result = &symbolSearchResult{query: q, symbols: symbols, err: err}
				if !w.posted {
					w.posted, post = true, true
				}
			}
			w.mu.Unlock()
			if post {
				w.post(w.flush)
			}
		}
	}
}

// A single queued callback drains the newest result. A query or close can
// invalidate the result while the GUI event loop is blocked by another modal.
func (w *symbolSearchWorker) flush() {
	w.mu.Lock()
	r, deliver := w.result, w.deliver
	w.result, w.posted = nil, false
	valid := !w.closed && r != nil && r.query.generation == w.latest.generation
	w.mu.Unlock()
	if valid && deliver != nil {
		deliver(*r)
	}
}
