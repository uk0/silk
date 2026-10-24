package ged

import (
	"context"
	"github.com/uk0/silk/async"
	"sync"
	"time"
)

var globalSearchPermits = make(chan struct{}, 2)

type globalSearchJob struct {
	request globalSearchRequest
	ctx     context.Context
	cancel  context.CancelFunc
}
type globalSearchWorker struct {
	mu              sync.Mutex
	latest          *globalSearchJob
	activeCancel    context.CancelFunc
	running, closed bool
	done            chan struct{}
	debounce        time.Duration
	compute         func(context.Context, globalSearchRequest) globalSearchResult
	delivery        *async.Latest[globalSearchResult]
}

func newGlobalSearchWorker(post func(func()), consume func(globalSearchResult)) *globalSearchWorker {
	delivery, _ := async.NewLatest(post, consume)
	return &globalSearchWorker{delivery: delivery, compute: computeGlobalSearch, debounce: 75 * time.Millisecond}
}
func (w *globalSearchWorker) cancelLocked() {
	if w.latest != nil {
		w.latest.cancel()
		w.latest = nil
	}
	if w.activeCancel != nil {
		w.activeCancel()
	}
}
func (w *globalSearchWorker) request(r globalSearchRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.cancelLocked()
	ctx, cancel := context.WithCancel(context.Background())
	w.latest = &globalSearchJob{r, ctx, cancel}
	if !w.running {
		w.running = true
		w.done = make(chan struct{})
		go w.run(w.done)
	}
}
func (w *globalSearchWorker) cancel() { w.mu.Lock(); w.cancelLocked(); w.mu.Unlock() }
func (w *globalSearchWorker) close() {
	w.mu.Lock()
	w.closed = true
	w.cancelLocked()
	w.mu.Unlock()
	w.delivery.Close()
}
func (w *globalSearchWorker) run(done chan struct{}) {
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
func (w *globalSearchWorker) runJob(job *globalSearchJob) {
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
	case globalSearchPermits <- struct{}{}:
	}
	defer func() { <-globalSearchPermits }()
	if job.ctx.Err() != nil {
		return
	}
	result := w.compute(job.ctx, job.request)
	if job.ctx.Err() == nil {
		w.delivery.Publish(result)
	}
}
