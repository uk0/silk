package gui

import (
	"context"
	"github.com/uk0/silk/async"
	"sync"
	"time"
)

var editorFindPermits = make(chan struct{}, 2)

type editorFindJob struct {
	request editorFindRequest
	ctx     context.Context
	cancel  context.CancelFunc
}
type editorFindWorker struct {
	mu              sync.Mutex
	latest          *editorFindJob
	activeCancel    context.CancelFunc
	running, closed bool
	done            chan struct{}
	debounce        time.Duration
	compute         func(context.Context, editorFindRequest) editorFindResult
	delivery        *async.Latest[editorFindResult]
}

func newEditorFindWorker(post func(func()), consume func(editorFindResult)) *editorFindWorker {
	delivery, _ := async.NewLatest(post, consume)
	return &editorFindWorker{delivery: delivery, compute: computeEditorFind, debounce: 75 * time.Millisecond}
}
func (w *editorFindWorker) cancelLocked() {
	if w.latest != nil {
		w.latest.cancel()
		w.latest = nil
	}
	if w.activeCancel != nil {
		w.activeCancel()
	}
}
func (w *editorFindWorker) request(r editorFindRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.cancelLocked()
	ctx, cancel := context.WithCancel(context.Background())
	w.latest = &editorFindJob{r, ctx, cancel}
	if !w.running {
		w.running = true
		w.done = make(chan struct{})
		go w.run(w.done)
	}
}
func (w *editorFindWorker) cancel() { w.mu.Lock(); w.cancelLocked(); w.mu.Unlock() }
func (w *editorFindWorker) close() {
	w.mu.Lock()
	w.closed = true
	w.cancelLocked()
	w.mu.Unlock()
	w.delivery.Close()
}
func (w *editorFindWorker) run(done chan struct{}) {
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
		w.runJob(job)
		job.cancel()
	}
}
func (w *editorFindWorker) runJob(job *editorFindJob) {
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
	case editorFindPermits <- struct{}{}:
	}
	defer func() { <-editorFindPermits }()
	if job.ctx.Err() != nil {
		return
	}
	result := w.compute(job.ctx, job.request)
	if job.ctx.Err() == nil {
		w.delivery.Publish(result)
	}
}
