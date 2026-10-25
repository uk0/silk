package gui

import (
	"context"
	"sync"
	"time"

	"github.com/uk0/silk/async"
)

const symbolAsyncBytes = 32 << 10

var symbolIndexPermits = make(chan struct{}, 2)

type symbolIndexRequest struct {
	key        codeSymbolCacheKey
	generation uint64
	text       string
	additional []CodeSymbol
}
type symbolIndexResult struct {
	request symbolIndexRequest
	cache   codeSymbolCache
}
type symbolIndexJob struct {
	request symbolIndexRequest
	ctx     context.Context
	cancel  context.CancelFunc
}

// One worker and one latest request per editor; at most two active parsers
// process-wide. No idle goroutine/timer. The Go parser is not preemptible: an
// obsolete parse retains its permit until it returns, then its result is dropped.
// Workers never access editor fields or mutable editor line slices.
type symbolIndexWorker struct {
	mu              sync.Mutex
	latest          *symbolIndexJob
	activeCancel    context.CancelFunc
	running, closed bool
	done            chan struct{}
	debounce        time.Duration
	parse           func(symbolIndexRequest) codeSymbolCache
	delivery        *async.Latest[symbolIndexResult]
}

func newSymbolIndexWorker(post func(func()), consume func(symbolIndexResult)) *symbolIndexWorker {
	delivery, _ := async.NewLatest(post, consume)
	return &symbolIndexWorker{delivery: delivery, debounce: 90 * time.Millisecond, parse: func(r symbolIndexRequest) codeSymbolCache {
		var c codeSymbolCache
		c.store(r.key, mergeCodeSymbols(r.text, goSyntaxLanguage(r.key.language), r.additional))
		return c
	}}
}
func (w *symbolIndexWorker) cancelLocked() {
	if w.latest != nil {
		w.latest.cancel()
		w.latest = nil
	}
	if w.activeCancel != nil {
		w.activeCancel()
	}
}
func (w *symbolIndexWorker) request(r symbolIndexRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.cancelLocked()
	ctx, cancel := context.WithCancel(context.Background())
	w.latest = &symbolIndexJob{r, ctx, cancel}
	if !w.running {
		w.running = true
		w.done = make(chan struct{})
		go w.run(w.done)
	}
}
func (w *symbolIndexWorker) cancel() { w.mu.Lock(); defer w.mu.Unlock(); w.cancelLocked() }
func (w *symbolIndexWorker) close() {
	w.mu.Lock()
	w.closed = true
	w.cancelLocked()
	w.mu.Unlock()
	w.delivery.Close()
}
func (w *symbolIndexWorker) run(done chan struct{}) {
	defer close(done)
	for {
		w.mu.Lock()
		job := w.latest
		w.latest = nil
		if w.closed || job == nil {
			w.running = false
			w.activeCancel = nil
			w.mu.Unlock()
			return
		}
		w.activeCancel = job.cancel
		w.mu.Unlock()
		w.compute(job)
		job.cancel()
	}
}
func (w *symbolIndexWorker) compute(job *symbolIndexJob) {
	timer := time.NewTimer(w.debounce)
	defer timer.Stop()
	select {
	case <-job.ctx.Done():
		return
	case <-timer.C:
	}
	select {
	case <-job.ctx.Done():
		return
	case symbolIndexPermits <- struct{}{}:
	}
	defer func() { <-symbolIndexPermits }()
	if job.ctx.Err() != nil {
		return
	}
	cache := w.parse(job.request)
	if job.ctx.Err() != nil {
		return
	}
	r := job.request
	r.text = ""
	r.additional = nil
	w.delivery.Publish(symbolIndexResult{r, cache})
}

// ensureSymbols never parses a large Go document on the UI thread. While a
// newer revision is pending, do not display/click stale line-numbered markers.
func (e *CodeEditor) ensureSymbols() bool {
	key := codeSymbolCacheKey{revision: e.TextRevision(), language: e.Language()}
	if e.symbolCache.valid && e.symbolCache.key == key {
		return true
	}
	if len(e.text) < symbolAsyncBytes {
		e.cachedSymbols()
		return true
	}
	if e.symbolPending && e.symbolRequest.key == key && e.symbolRequest.generation == e.symbolsRevision {
		return false
	}
	if e.symbolWorker == nil {
		post := e.symbolPost
		if post == nil {
			post = Post
		}
		var worker *symbolIndexWorker
		worker = newSymbolIndexWorker(post, func(r symbolIndexResult) {
			if e.symbolWorker != worker || !e.symbolPending || e.TextRevision() != r.request.key.revision || e.Language() != r.request.key.language || e.symbolsRevision != r.request.generation {
				return
			}
			e.symbolPending = false
			e.symbolCache = r.cache
			e.symbolsRevision++ // notify outline consumers even though text is unchanged
			e.Update()
			e.notifySymbolsChanged()
		})
		e.symbolWorker = worker
	}
	r := symbolIndexRequest{key: key, generation: e.symbolsRevision}
	e.symbolRequest = r
	e.symbolPending = true
	r.text = e.text
	r.additional = append([]CodeSymbol(nil), e.currentAdditionalSymbols()...)
	e.symbolWorker.request(r)
	return false
}

// SymbolsSnapshot returns an owned snapshot for nonblocking UI consumers.
// ready=false means large-file indexing is pending, not that the file has no
// symbols. Recheck when SymbolsRevision changes. ParseSymbols remains the
// synchronous, exact-revision API for explicit commands. UI-thread only.
func (e *CodeEditor) SymbolsSnapshot() (symbols []CodeSymbol, ready bool) {
	if !e.ensureSymbols() {
		return nil, false
	}
	return append([]CodeSymbol(nil), e.symbolCache.symbols...), true
}
func (e *CodeEditor) cancelSymbolRequest() {
	e.symbolPending = false
	if e.symbolWorker != nil {
		e.symbolWorker.cancel()
	}
}

// CancelSymbolIndex releases pending snapshots and suppresses queued deliveries.
// Hosts must call it on the UI thread when removing an editor or before stopping
// the event loop. A later symbol query may start fresh; cancellation never waits
// for a parser on the UI thread.
func (e *CodeEditor) CancelSymbolIndex() {
	e.symbolPending = false
	if e.symbolWorker != nil {
		e.symbolWorker.close()
		e.symbolWorker = nil
	}
}
