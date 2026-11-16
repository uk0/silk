package hotreload

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/fswatch"
)

type watchRequest struct {
	path     string
	done     chan struct{}
	finished bool
	err      error
}
type reloadEntry struct {
	root   *watchRequest
	token  uint64
	due    time.Time
	active bool
}
type reloadJob struct {
	path     string
	entry    *reloadEntry
	token    uint64
	rejected uint64
}

// Called with r.mu held, including on Unwatch/Close. Waiting duplicate Watch
// callers wake immediately even if the original AddPath is parked in OS I/O.
func finishWatch(root *watchRequest, err error) {
	if !root.finished {
		root.finished = true
		root.err = err
		close(root.done)
	}
}
func (r *Reloader) watch(path string, register func(string) error) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fswatch.ErrClosed
	}
	if root := r.roots[abs]; root != nil {
		r.mu.Unlock()
		select {
		case <-root.done:
			return root.err
		case <-r.stop:
			return fswatch.ErrClosed
		}
	}
	root := &watchRequest{path: abs, done: make(chan struct{})}
	r.roots[abs] = root
	r.mu.Unlock()
	err = register(abs) // May block; never holds the reloader's control lock.
	r.mu.Lock()
	if r.closed {
		err = fswatch.ErrClosed
	} else if r.roots[abs] != root {
		err = fswatch.ErrSuperseded
	} else if err != nil {
		delete(r.roots, abs)
		for p, e := range r.pending {
			if e.root == root {
				delete(r.pending, p)
			}
		}
	}
	finishWatch(root, err)
	err = root.err
	r.mu.Unlock()
	r.signal()
	return err
}
func (r *Reloader) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
func (r *Reloader) runnerDone() {
	r.mu.Lock()
	r.live--
	if r.live == 0 {
		r.busy = false
		close(r.done)
	}
	r.mu.Unlock()
}

// Exactly one collector/timer and one worker. While the worker is busy the
// collector still consumes events and retains only each path's latest token.
func (r *Reloader) loop() {
	defer r.runnerDone()
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	for {
		delay := r.dispatchReady(time.Now())
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var tick <-chan time.Time
		if delay >= 0 {
			timer.Reset(delay)
			tick = timer.C
		}
		select {
		case <-r.stop:
			return
		case <-r.wake:
		case <-tick:
		case ev, ok := <-r.w.Events():
			if !ok {
				return
			}
			if ev.Kind == fswatch.Modified || ev.Kind == fswatch.Created {
				r.scheduleRebuild(ev.Path)
			}
		}
	}
}
func (r *Reloader) findRootLocked(path string) *watchRequest {
	var chosen *watchRequest
	for root, request := range r.roots {
		prefix := root
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		if path == root || strings.HasPrefix(path, prefix) {
			if chosen == nil || len(root) > len(chosen.path) {
				chosen = request
			}
		}
	}
	return chosen
}
func (r *Reloader) scheduleRebuild(path string) {
	abs, err := filepath.Abs(path)
	if err != nil || !r.extAllowed(abs) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	root := r.findRootLocked(abs)
	if root == nil {
		return
	}
	e := r.pending[abs]
	if e == nil || e.root != root {
		if e == nil && len(r.pending) >= r.limit {
			r.rejected++
			if r.reportRejected == 0 {
				r.reportDue = time.Now().Add(r.debounce)
			}
			r.reportRejected++
			r.signal()
			return
		}
		e = &reloadEntry{root: root}
		r.pending[abs] = e
	}
	r.sequence++
	e.token = r.sequence
	e.due = time.Now().Add(r.debounce)
	r.signal()
}

// Nonnegative result is time until the next deadline. -1 means no idle timer.
// Dispatch under mu into a single-slot channel so Close can revoke an unsent job
// without racing a later send, and at most one worker job can be admitted.
func (r *Reloader) dispatchReady(now time.Time) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.busy {
		return -1
	}
	var path string
	var entry *reloadEntry
	var due time.Time
	for p, e := range r.pending {
		if !e.root.finished || e.root.err != nil {
			continue
		}
		if !e.active && (entry == nil || e.due.Before(due) || (e.due.Equal(due) && e.token < entry.token)) {
			path, entry, due = p, e, e.due
		}
	}
	overflow := r.reportRejected > 0 && (entry == nil || r.reportDue.Before(due))
	if overflow {
		due = r.reportDue
	} else if entry == nil {
		return -1
	}
	if due.After(now) {
		return due.Sub(now)
	}
	job := reloadJob{}
	if overflow {
		job.rejected = r.reportRejected
		r.reportRejected = 0
	} else {
		entry.active = true
		job = reloadJob{path: path, entry: entry, token: entry.token}
	}
	r.busy = true
	r.jobs <- job
	return -1
}
func (r *Reloader) allowed(job reloadJob) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	if job.rejected > 0 {
		return true
	}
	return r.pending[job.path] == job.entry && job.entry.token == job.token && r.roots[job.entry.root.path] == job.entry.root && job.entry.root.finished && job.entry.root.err == nil
}
func (r *Reloader) finish(job reloadJob) {
	r.mu.Lock()
	if e := r.pending[job.path]; e != nil && e == job.entry {
		if e.token == job.token {
			delete(r.pending, job.path)
		} else {
			e.active = false
		}
	}
	r.busy = false
	r.mu.Unlock()
	r.signal()
}
func (r *Reloader) worker() {
	defer r.runnerDone()
	for {
		select {
		case <-r.stop:
			return
		case job := <-r.jobs:
			r.runJob(job)
			r.finish(job)
		}
	}
}
func (r *Reloader) runJob(job reloadJob) {
	if !r.allowed(job) {
		return
	}
	if job.rejected > 0 {
		r.fireError(job, fmt.Errorf("%w: %d events rejected", ErrPendingLimit, job.rejected))
		return
	}
	var tree *decl.Node
	var err error
	if r.onInvalidate == nil {
		tree, err = r.load(job.path)
		if err != nil {
			r.fireError(job, err)
			return
		}
	}
	if !r.allowed(job) {
		return
	}
	// Admission is linearized by allowed(). Close can revoke later admissions,
	// not preempt an already admitted callback. Wait proves it has returned.
	if r.onInvalidate != nil {
		err = invokeInvalidation(r.onInvalidate, job.path)
	} else {
		err = invokeReload(r.onReload, job.path, tree)
	}
	if err != nil {
		r.fireError(job, err)
	}
}
func invokeInvalidation(fn InvalidationFunc, path string) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("hotreload: callback panic: %v", p)
		}
	}()
	return fn(path)
}
func invokeReload(fn ReloadFunc, path string, tree *decl.Node) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("hotreload: callback panic: %v", p)
		}
	}()
	return fn(path, tree)
}
func (r *Reloader) fireError(job reloadJob, err error) {
	if r.onError == nil || !r.allowed(job) {
		return
	}
	// An error hook must not kill the worker and leave shutdown waiting forever.
	// Its panic is contained; there is no recursive second error-hook invocation.
	defer func() {
		if p := recover(); p != nil {
			core.Warn("hotreload error hook panic: ", p)
		}
	}()
	r.onError(job.path, err)
}
