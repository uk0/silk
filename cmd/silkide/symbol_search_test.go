package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

func receiveSymbolTest[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("symbol test wait timed out")
		var z T
		return z
	}
}
func closeSymbolWorker(t *testing.T, w *symbolSearchWorker) {
	t.Helper()
	w.close()
	receiveSymbolTest(t, w.done)
}
func waitSymbolResult(t *testing.T, w *symbolSearchWorker, q string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		w.mu.Lock()
		ready := w.result != nil && w.result.query.text == q
		w.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("no result for %q", q)
		}
	}
}

func TestSymbolSearchBurstCancelsWithSingleActiveRequest(t *testing.T) {
	client := new(core.LSPClient)
	started := make(chan symbolSearchQuery, 4)
	cancelled := make(chan struct{}, 1)
	release := make(chan struct{})
	posts := make(chan func(), 4)
	delivered := make(chan symbolSearchResult, 4)
	var active, maxActive atomic.Int32
	w := newSymbolSearchWorker(10*time.Millisecond, func(ctx context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		n := active.Add(1)
		for old := maxActive.Load(); n > old; old = maxActive.Load() {
			if maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		defer active.Add(-1)
		started <- q
		if q.text == "old" {
			<-ctx.Done()
			cancelled <- struct{}{}
			<-release
		}
		return []core.LSPWorkspaceSymbol{{Name: q.text}}, nil
	}, func(fn func()) { posts <- fn }, func(r symbolSearchResult) { delivered <- r })
	t.Cleanup(func() { closeSymbolWorker(t, w) })
	w.update(client, "old")
	receiveSymbolTest(t, started)
	for i := 0; i < 10000; i++ {
		w.update(client, fmt.Sprintf("q%d", i))
	}
	receiveSymbolTest(t, cancelled)
	select {
	case q := <-started:
		t.Fatalf("concurrent RPC started: %+v", q)
	default:
	}
	close(release)
	if q := receiveSymbolTest(t, started); q.text != "q9999" {
		t.Fatalf("queued intermediate query: %+v", q)
	}
	receiveSymbolTest(t, posts)()
	if r := receiveSymbolTest(t, delivered); r.query.text != "q9999" {
		t.Fatal(r.query)
	}
	if maxActive.Load() != 1 {
		t.Fatalf("active RPCs=%d", maxActive.Load())
	}
	select {
	case q := <-started:
		t.Fatalf("extra RPC: %+v", q)
	default:
	}
}

func TestSymbolSearchDebouncesAndCoalescesUIDelivery(t *testing.T) {
	client := new(core.LSPClient)
	posts := make(chan func(), 4)
	var calls atomic.Int32
	var delivered []symbolSearchResult
	w := newSymbolSearchWorker(10*time.Millisecond, func(_ context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		calls.Add(1)
		return []core.LSPWorkspaceSymbol{{Name: q.text}}, nil
	}, func(fn func()) { posts <- fn }, func(r symbolSearchResult) { delivered = append(delivered, r) })
	defer closeSymbolWorker(t, w)
	for i := 0; i < 100; i++ {
		w.update(client, fmt.Sprint(i))
	}
	waitSymbolResult(t, w, "99")
	if calls.Load() != 1 {
		t.Fatalf("burst was not debounced: %d", calls.Load())
	}
	// Deliberately do not service UI callbacks while results keep arriving.
	for i := 100; i < 120; i++ {
		q := fmt.Sprint(i)
		w.update(client, q)
		waitSymbolResult(t, w, q)
	}
	if len(posts) != 1 {
		t.Fatalf("unbounded UI queue: %d", len(posts))
	}
	receiveSymbolTest(t, posts)()
	if len(delivered) != 1 || delivered[0].query.text != "119" {
		t.Fatalf("delivery=%+v", delivered)
	}
	w.update(client, "obsolete")
	waitSymbolResult(t, w, "obsolete")
	w.update(nil, "server stopped")
	receiveSymbolTest(t, posts)()
	if len(delivered) != 1 {
		t.Fatal("invalidated queued result delivered")
	}
}

func TestSymbolSearchCloseCancelsAndRetiresQueuedDelivery(t *testing.T) {
	client := new(core.LSPClient)
	posts := make(chan func(), 2)
	started := make(chan struct{}, 1)
	delivered := 0
	w := newSymbolSearchWorker(0, func(ctx context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		if q.text == "blocked" {
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return nil, nil
	}, func(fn func()) { posts <- fn }, func(symbolSearchResult) { delivered++ })
	w.update(client, "ready")
	waitSymbolResult(t, w, "ready")
	w.update(client, "blocked")
	receiveSymbolTest(t, started)
	closeSymbolWorker(t, w)
	w.close()
	w.update(client, "ignored")
	receiveSymbolTest(t, posts)()
	if delivered != 0 {
		t.Fatal("closed dialog received callback")
	}
}
