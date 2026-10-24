package gui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type diffTestQueue struct {
	mu    sync.Mutex
	tasks []func()
	max   int
}

func (q *diffTestQueue) post(fn func()) {
	q.mu.Lock()
	q.tasks = append(q.tasks, fn)
	q.max = max(q.max, len(q.tasks))
	q.mu.Unlock()
}
func (q *diffTestQueue) drain() {
	q.mu.Lock()
	tasks := q.tasks
	q.tasks = nil
	q.mu.Unlock()
	for _, fn := range tasks {
		fn()
	}
}
func waitDiffWorker(t *testing.T, w *diffWorker) {
	t.Helper()
	w.mu.Lock()
	done := w.done
	w.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("diff worker did not retire")
	}
}
func pumpDiff(t *testing.T, q *diffTestQueue, v *DiffView) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for v.DiffPending() && time.Now().Before(deadline) {
		q.drain()
		time.Sleep(time.Millisecond)
	}
	q.drain()
	if v.DiffPending() {
		t.Fatal("diff stayed pending")
	}
}

func TestDiffAsyncSnapshotAndStatistics(t *testing.T) {
	q := new(diffTestQueue)
	v := NewDiffView()
	v.diffPost = q.post
	defer v.CancelDiff()
	changed := 0
	v.SetDiffStateChanged(func(*DiffView) { changed++ })
	old := "中文\nkeep\nold\n"
	next := "中文\nkeep\nnew\nextra\n"
	if err := v.SetTextsAsync(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	if !v.DiffPending() || len(v.DiffRows()) != 0 || v.OldText() != old || v.NewText() != next {
		t.Fatal("pending snapshots/rows invalid")
	}
	v.SetSize(400, 200)
	v.Draw(diffViewNopPainter{})
	pumpDiff(t, q, v)
	if v.DiffError() != nil || changed != 2 {
		t.Fatalf("err=%v notifications=%d", v.DiffError(), changed)
	}
	checkExactRows(t, splitDiffLines(old), splitDiffLines(next), v.DiffRows())
	if s := v.DiffSummary(); s.Rows != 5 || s.Same != 3 || s.Added != 1 || s.Modified != 1 || s.ChangedRows() != 2 {
		t.Fatalf("summary %+v", s)
	}
	l, r := gutterLineNumbers(v.DiffRows())
	if !equalIntSlice(l, v.leftNumbers) || !equalIntSlice(r, v.rightNumbers) {
		t.Fatal("background numbering invalid")
	}
}
func TestDiffAsyncStaleCompletionCannotReplaceSyncOrPatch(t *testing.T) {
	for _, patch := range []bool{false, true} {
		q := new(diffTestQueue)
		v := NewDiffView()
		v.diffPost = q.post
		if err := v.SetTextsAsync(context.Background(), "old", "async result"); err != nil {
			t.Fatal(err)
		}
		waitDiffWorker(t, v.diffWorker) // completed but not yet delivered
		if patch {
			v.SetPatchFile(patchFixture())
		} else {
			v.SetTexts("replacement", "replacement")
		}
		before := append([]DiffRow(nil), v.DiffRows()...)
		q.drain()
		if v.DiffPending() || v.DiffError() != nil || v.IsPatchMode() != patch || !reflect.DeepEqual(before, v.DiffRows()) {
			t.Fatal("stale async result overwrote a replacement")
		}
	}
}
func TestDiffAsyncContextCancelHideAndRestart(t *testing.T) {
	q := new(diffTestQueue)
	v := NewDiffView()
	v.diffPost = q.post
	v.SetTexts("a", "b")
	if err := v.SetTextsAsync(nil, "bad", "bad"); err == nil || v.OldText() != "a" {
		t.Fatal("invalid context mutated state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := v.SetTextsAsync(ctx, "bad", "bad"); !errors.Is(err, context.Canceled) || v.OldText() != "a" {
		t.Fatal("cancelled admission mutated state")
	}
	// Hold both computation permits; cancellation must retire a waiting job.
	diffComputePermits <- struct{}{}
	diffComputePermits <- struct{}{}
	ctx, cancel = context.WithCancel(context.Background())
	if err := v.SetTextsAsync(ctx, "a", "c"); err != nil {
		t.Fatal(err)
	}
	w := v.diffWorker
	cancel()
	waitDiffWorker(t, w)
	q.drain()
	<-diffComputePermits
	<-diffComputePermits
	if v.DiffPending() || !errors.Is(v.DiffError(), context.Canceled) {
		t.Fatal("external cancellation did not complete")
	}
	if err := v.SetTextsAsync(context.Background(), "a", "newest"); err != nil {
		t.Fatal(err)
	}
	w = v.diffWorker
	v.OnHide()
	waitDiffWorker(t, w)
	q.drain()
	if v.diffWorker != nil || v.DiffPending() || len(v.DiffRows()) != 0 {
		t.Fatal("hidden view retained work")
	}
	if err := v.SetTextsAsync(context.Background(), "a", "reopened"); err != nil {
		t.Fatal(err)
	}
	pumpDiff(t, q, v)
	if v.DiffRows()[0].NewLine != "reopened" {
		t.Fatal("could not restart after cancellation")
	}
	v.CancelDiff()
	v.CancelDiff()
}
func TestDiffWorkerCoalescesAndNeverOverlapsAnOwner(t *testing.T) {
	q := new(diffTestQueue)
	var got []diffCompletion
	w := newDiffWorker(q.post, func(r diffCompletion) { got = append(got, r) })
	defer w.close()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	w.compute = func(ctx context.Context, a, b string) (diffSnapshot, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		} // simulate a delayed cancelled computation
		return computeDiffSnapshot(ctx, a, b)
	}
	w.request(context.Background(), 1, "a", "old")
	<-entered
	for n := 2; n <= 200; n++ {
		w.request(context.Background(), uint64(n), "a", fmt.Sprint(n))
	}
	if calls.Load() != 1 {
		t.Fatal("more than one computation ran for one view")
	}
	close(release)
	waitDiffWorker(t, w)
	q.drain()
	if calls.Load() != 2 || len(got) != 1 || got[0].generation != 200 || got[0].err != nil || got[0].snapshot.rows[0].NewLine != "200" {
		t.Fatalf("not latest-only: calls=%d results=%+v", calls.Load(), got)
	}
	if q.max != 1 {
		t.Fatalf("UI deliveries grew to %d", q.max)
	}
}
func TestDiffWorkersBoundGlobalCPUAndReleaseOnClose(t *testing.T) {
	var active, maxActive atomic.Int32
	entered := make(chan struct{}, 8)
	var workers []*diffWorker
	q := new(diffTestQueue)
	for n := 0; n < 8; n++ {
		w := newDiffWorker(q.post, func(diffCompletion) { t.Error("closed worker delivered") })
		w.compute = func(ctx context.Context, _, _ string) (diffSnapshot, error) {
			a := active.Add(1)
			defer active.Add(-1)
			for {
				m := maxActive.Load()
				if m >= a || maxActive.CompareAndSwap(m, a) {
					break
				}
			}
			entered <- struct{}{}
			<-ctx.Done()
			return diffSnapshot{}, ctx.Err()
		}
		workers = append(workers, w)
		w.request(context.Background(), 1, "a", "b")
	}
	for n := 0; n < 2; n++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	for _, w := range workers {
		w.close()
	}
	for _, w := range workers {
		waitDiffWorker(t, w)
	}
	q.drain()
	if maxActive.Load() > 2 || active.Load() != 0 || len(diffComputePermits) != 0 {
		t.Fatal("CPU permit/worker leaked")
	}
}
func TestDiffSnapshotSplittingAndCancellation(t *testing.T) {
	for _, s := range []string{"", "\n", "a\n", "a\r\nb", strings.Repeat("x\n", 10000)} {
		lines, err := splitDiffLinesContext(context.Background(), s)
		if err != nil || !reflect.DeepEqual(lines, splitDiffLines(s)) {
			t.Fatal("split changed trailing/empty line semantics")
		}
	}
	ctx := &diffCancelChecks{Context: context.Background(), remaining: 2}
	if _, err := computeDiffSnapshot(ctx, strings.Repeat("x\n", 10000), "x"); !errors.Is(err, context.Canceled) {
		t.Fatal("split ignored cancellation")
	}
}
