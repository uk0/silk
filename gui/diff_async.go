package gui

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/i18n"
)

// DiffSummary counts displayed text rows (patch separators are not text rows).
// Modified rows count once, although both columns have a changed line.
type DiffSummary struct{ Rows, Same, Added, Removed, Modified int }

func (s DiffSummary) ChangedRows() int { return s.Added + s.Removed + s.Modified }
func (s *DiffSummary) add(status DiffRowStatus) {
	switch status {
	case DiffSame:
		s.Same++
	case DiffAdded:
		s.Added++
	case DiffRemoved:
		s.Removed++
	case DiffModified:
		s.Modified++
	default:
		return
	}
	s.Rows++
}

type diffSnapshot struct {
	oldLines, newLines []string
	rows               []DiffRow
	left, right        []int
	summary            DiffSummary
}

func splitDiffLinesContext(ctx context.Context, text string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}
	var lines []string
	for {
		if len(lines)&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			lines = append(lines, text)
			return lines, ctx.Err()
		}
		lines = append(lines, text[:i])
		text = text[i+1:]
	}
}
func computeDiffSnapshot(ctx context.Context, oldText, newText string) (diffSnapshot, error) {
	s := diffSnapshot{}
	var err error
	s.oldLines, err = splitDiffLinesContext(ctx, oldText)
	if err != nil {
		return diffSnapshot{}, err
	}
	s.newLines, err = splitDiffLinesContext(ctx, newText)
	if err != nil {
		return diffSnapshot{}, err
	}
	s.rows, err = lineDiffContext(ctx, s.oldLines, s.newLines)
	if err != nil {
		return diffSnapshot{}, err
	}
	s.left, s.right = make([]int, len(s.rows)), make([]int, len(s.rows))
	lc, rc := 0, 0
	for i, r := range s.rows {
		if i&255 == 0 {
			if err := ctx.Err(); err != nil {
				return diffSnapshot{}, err
			}
		}
		switch r.Status {
		case DiffSame, DiffModified:
			lc++
			rc++
			s.left[i], s.right[i] = lc, rc
		case DiffAdded:
			rc++
			s.right[i] = rc
		case DiffRemoved:
			lc++
			s.left[i] = lc
		}
		s.summary.add(r.Status)
	}
	if err := ctx.Err(); err != nil {
		return diffSnapshot{}, err
	}
	return s, nil
}

type diffJob struct {
	generation       uint64
	oldText, newText string
	ctx              context.Context
	cancel           context.CancelFunc
}
type diffCompletion struct {
	generation uint64
	snapshot   diffSnapshot
	err        error
}

var diffComputePermits = make(chan struct{}, 2)

// One worker and one replaceable pending request per view, two computations
// process-wide. No idle timer/goroutine. Worker payloads contain immutable text,
// never widget pointers/fields. UI delivery is coalesced and generation checked.
type diffWorker struct {
	mu              sync.Mutex
	latest          *diffJob
	activeCancel    context.CancelFunc
	running, closed bool
	done            chan struct{}
	delivery        *async.Latest[diffCompletion]
	compute         func(context.Context, string, string) (diffSnapshot, error)
}

func newDiffWorker(post func(func()), consume func(diffCompletion)) *diffWorker {
	delivery, _ := async.NewLatest(post, consume)
	return &diffWorker{delivery: delivery, compute: computeDiffSnapshot}
}
func (w *diffWorker) cancelLocked() {
	if w.latest != nil {
		w.latest.cancel()
		w.latest = nil
	}
	if w.activeCancel != nil {
		w.activeCancel()
	}
}
func (w *diffWorker) request(ctx context.Context, generation uint64, oldText, newText string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.cancelLocked()
	ctx, cancel := context.WithCancel(ctx)
	w.latest = &diffJob{generation, oldText, newText, ctx, cancel}
	if !w.running {
		w.running = true
		w.done = make(chan struct{})
		go w.run(w.done)
	}
}
func (w *diffWorker) close() {
	w.mu.Lock()
	w.closed = true
	w.cancelLocked()
	w.mu.Unlock()
	w.delivery.Close()
}
func (w *diffWorker) run(done chan struct{}) {
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
		result := diffCompletion{generation: job.generation}
		select {
		case <-job.ctx.Done():
			result.err = job.ctx.Err()
		case diffComputePermits <- struct{}{}:
			result.snapshot, result.err = async.Run(job.ctx, func(ctx context.Context) (diffSnapshot, error) { return w.compute(ctx, job.oldText, job.newText) })
			<-diffComputePermits
		}
		job.cancel()
		w.delivery.Publish(result)
	}
}

// SetTextsAsync replaces both snapshots and computes off the UI thread. Call
// this and all other widget methods on the UI thread. The result includes line
// numbers/statistics, so applying it is O(1). No partial/stale rows are displayed.
// Nil/already-cancelled contexts fail without changing the current comparison.
// Hosts must call CancelDiff when disposing the view (including an ancestor
// dialog closing); OnHide also cancels when the view itself is hidden.
func (v *DiffView) SetTextsAsync(ctx context.Context, oldText, newText string) error {
	if ctx == nil {
		return async.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	v.diffGeneration++
	v.oldText, v.newText = oldText, newText
	v.oldLines, v.newLines, v.diffRows = nil, nil, nil
	v.leftNumbers, v.rightNumbers = nil, nil
	v.patchMode = false
	v.patchFile = DiffPatchFile{}
	v.rowHunk = nil
	v.hunkHeaderRows = nil
	v.scrollY = 0
	v.activeChangeRow = -1
	v.diffSummary = DiffSummary{}
	v.diffErr = nil
	v.diffPending = true
	if v.diffWorker == nil {
		post := v.diffPost
		if post == nil {
			post = Post
		}
		var worker *diffWorker
		worker = newDiffWorker(post, func(result diffCompletion) {
			if v.diffWorker != worker || !v.diffPending || v.diffGeneration != result.generation {
				return
			}
			v.diffPending = false
			v.diffErr = result.err
			if result.err == nil {
				s := result.snapshot
				v.oldLines, v.newLines = s.oldLines, s.newLines
				v.diffRows = s.rows
				v.leftNumbers, v.rightNumbers = s.left, s.right
				v.diffSummary = s.summary
			}
			v.Self().Update()
			v.notifyDiffState()
		})
		v.diffWorker = worker
	}
	v.diffWorker.request(ctx, v.diffGeneration, oldText, newText)
	v.Self().Update()
	v.notifyDiffState()
	return nil
}

// DiffPending, DiffError and DiffSummary are UI-thread status accessors.
func (v *DiffView) DiffPending() bool        { return v.diffPending }
func (v *DiffView) DiffError() error         { return v.diffErr }
func (v *DiffView) DiffSummary() DiffSummary { return v.diffSummary }

// SetDiffStateChanged registers a UI-thread notification on start, completion,
// cancellation and synchronous replacement. No callback runs on a worker.
func (v *DiffView) SetDiffStateChanged(fn func(*DiffView)) { v.diffStateChanged = fn }
func (v *DiffView) notifyDiffState() {
	if v.diffStateChanged != nil {
		v.diffStateChanged(v)
	}
}
func (v *DiffView) stopDiff() {
	v.diffGeneration++
	if v.diffWorker != nil {
		v.diffWorker.close()
		v.diffWorker = nil
	}
	v.diffPending = false
}

// CancelDiff is idempotent and never waits for the worker/UI queue. Queued
// completions are suppressed, and a future SetTextsAsync can start a new job.
func (v *DiffView) CancelDiff() {
	pending := v.diffPending
	v.stopDiff()
	if pending {
		v.diffErr = context.Canceled
		v.Self().Update()
		v.notifyDiffState()
	}
}
func (v *DiffView) OnHide() { v.CancelDiff() }
func (v *DiffView) diffMessage() string {
	if v.diffPending {
		return i18n.T("Computing comparison...")
	}
	if errors.Is(v.diffErr, context.Canceled) {
		return i18n.T("Comparison cancelled")
	}
	if v.diffErr != nil {
		return i18n.Tf("Comparison failed: %v", v.diffErr)
	}
	return ""
}
