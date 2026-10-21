package gui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/diagnostic"
)

func waitGitUI(t *testing.T, q <-chan func(), done func() bool) {
	t.Helper()
	deadline := time.After(4 * time.Second)
	for !done() {
		select {
		case fn := <-q:
			fn()
		case <-deadline:
			t.Fatal("git UI result timed out")
		}
	}
}
func waitGitIdle(t *testing.T, s *gitStatusService) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		idle := s.workers == 0 && s.queue.Len() == 0
		s.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("git workers did not exit")
}
func TestEditorGitNeverBlocksIdentityTypingOrCancel(t *testing.T) {
	q := make(chan func(), 4)
	entered, release := make(chan struct{}), make(chan struct{})
	var released sync.Once
	defer released.Do(func() { close(release) })
	var once sync.Once
	s := newGitStatusService(2, func(ctx context.Context, path string) (map[int]GitLineStatus, error) {
		once.Do(func() { close(entered) })
		<-release
		return map[int]GitLineStatus{2: GitModified}, nil
	}, func(fn func()) { q <- fn })
	e := NewCodeEditor()
	e.gitService = s
	start := time.Now()
	e.SetFilePath("old.go")
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("path setter waited for Git")
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Git worker did not start")
	}
	e.SetText("package p\n")
	e.ReplaceAllText("package p\nvar Value int\n")
	e.Undo()
	if e.Text() != "package p\n" {
		t.Fatal("typing/undo interfered with Git")
	}
	e.SetFilePath("next.go")
	if e.FilePath() != "next.go" || !e.GitStatusLoading() {
		t.Fatal("immediate identity")
	}
	e.CancelGitStatus()
	released.Do(func() { close(release) })
	waitGitIdle(t, s)
	for len(q) > 0 {
		(<-q)()
	}
	if e.GitStatusLoading() || len(e.GitLineStatuses()) != 0 {
		t.Fatal("retired result repainted editor")
	}
}
func TestEditorGitGlobalConcurrencyAndReopenBounded(t *testing.T) {
	q := make(chan func(), 4)
	release := make(chan struct{})
	var released sync.Once
	defer released.Do(func() { close(release) })
	entered := make(chan struct{}, 10)
	var active, peak, calls atomic.Int32
	s := newGitStatusService(2, func(ctx context.Context, path string) (map[int]GitLineStatus, error) {
		calls.Add(1)
		n := active.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		defer active.Add(-1)
		entered <- struct{}{}
		<-release
		return map[int]GitLineStatus{1: GitAdded}, nil
	}, func(fn func()) { q <- fn })
	a, b := NewCodeEditor(), NewCodeEditor()
	a.gitService = s
	b.gitService = s
	a.SetFilePath("a.go")
	b.SetFilePath("b.go")
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("second worker starved")
		}
	}
	for i := 0; i < 1000; i++ {
		e := NewCodeEditor()
		e.gitService = s
		e.SetFilePath(fmt.Sprintf("closed-%d.go", i))
		e.CancelGitStatus()
	}
	last := NewCodeEditor()
	last.gitService = s
	last.SetFilePath("latest.go")
	for i := 0; i < 10000; i++ {
		last.RefreshGitStatus()
	}
	s.mu.Lock()
	queued, workers := s.queue.Len(), s.workers
	s.mu.Unlock()
	if queued != 1 || workers != 2 || calls.Load() != 2 {
		t.Fatalf("unbounded/stale work: queued=%d workers=%d calls=%d", queued, workers, calls.Load())
	}
	released.Do(func() { close(release) })
	waitGitUI(t, q, func() bool { return !a.GitStatusLoading() && !b.GitStatusLoading() && !last.GitStatusLoading() })
	waitGitIdle(t, s)
	if peak.Load() > 2 || calls.Load() != 3 {
		t.Fatal("wrong concurrency/coalescing", peak.Load(), calls.Load())
	}
	a.CancelGitStatus()
	b.CancelGitStatus()
	last.CancelGitStatus()
}
func TestEditorGitDropsOldPathAndCoalescesUIDelivery(t *testing.T) {
	q := make(chan func(), 4)
	calls := make(chan string, 20)
	release := make(chan struct{})
	var released sync.Once
	defer released.Do(func() { close(release) })
	s := newGitStatusService(1, func(ctx context.Context, path string) (map[int]GitLineStatus, error) {
		calls <- path
		if path == "old.go" {
			<-release
		}
		return map[int]GitLineStatus{len(path): GitModified}, nil
	}, func(fn func()) { q <- fn })
	e := NewCodeEditor()
	e.gitService = s
	defer e.CancelGitStatus()
	e.SetFilePath("old.go")
	select {
	case <-calls:
	case <-time.After(3 * time.Second):
		t.Fatal("Git worker did not start")
	}
	for i := 0; i < 10000; i++ {
		e.SetFilePath("new.go")
	}
	released.Do(func() { close(release) })
	waitGitIdle(t, s)
	if len(q) != 1 {
		t.Fatal("UI callbacks unbounded", len(q))
	}
	waitGitUI(t, q, func() bool { return !e.GitStatusLoading() })
	if e.FilePath() != "new.go" || e.GitLineStatuses()[len("new.go")] != GitModified {
		t.Fatal("new path not delivered")
	}
	// Finish another result but cancel/rebind before the queued GUI callback runs.
	e.SetFilePath("late.go")
	waitGitIdle(t, s)
	e.SetFilePath("")
	for len(q) > 0 {
		(<-q)()
	}
	if len(e.GitLineStatuses()) != 0 || e.GitStatusLoading() {
		t.Fatal("old queued callback survived empty identity")
	}
}
func TestEditorGitErrorsEmptyResultsAndOwnedSnapshots(t *testing.T) {
	q := make(chan func(), 4)
	s := newGitStatusService(1, func(ctx context.Context, path string) (map[int]GitLineStatus, error) {
		switch path {
		case "error.go":
			return nil, diagnostic.New(diagnostic.GitDiffFailed, "failed", errors.New("underlying cause"))
		case "clean.go":
			return map[int]GitLineStatus{}, nil
		default:
			return map[int]GitLineStatus{2: GitModified}, nil
		}
	}, func(fn func()) { q <- fn })
	e := NewCodeEditor()
	e.gitService = s
	defer e.CancelGitStatus()
	delivered := 0
	e.SigGitStatusChanged(func() { delivered++ })
	e.SetFilePath("dirty.go")
	waitGitUI(t, q, func() bool { return !e.GitStatusLoading() })
	snapshot := e.GitLineStatuses()
	snapshot[2] = GitDeleted
	if e.GitLineStatuses()[2] != GitModified {
		t.Fatal("public map alias")
	}
	e.SetFilePath("error.go")
	waitGitUI(t, q, func() bool { return !e.GitStatusLoading() })
	if len(e.GitLineStatuses()) != 0 || diagnostic.CodeOf(e.GitStatusError()) != diagnostic.GitDiffFailed {
		t.Fatal("stale markers or missing error")
	}
	d := e.GitStatusError().(*diagnostic.Error)
	d.Message = "mutated"
	if e.GitStatusError().(*diagnostic.Error).Message == "mutated" {
		t.Fatal("error alias")
	}
	e.SetFilePath("clean.go")
	waitGitUI(t, q, func() bool { return !e.GitStatusLoading() })
	if e.GitStatusError() != nil || len(e.GitLineStatuses()) != 0 || delivered != 3 {
		t.Fatal("empty successful result not applied", delivered)
	}
}

func TestEditorGitCoalescesGlobalDeliveryAndRemovesClosedEditors(t *testing.T) {
	q := make(chan func(), 128)
	s := newGitStatusService(2, func(context.Context, string) (map[int]GitLineStatus, error) {
		return map[int]GitLineStatus{1: GitAdded}, nil
	}, func(fn func()) { q <- fn })
	editors := make([]*CodeEditor, 64)
	delivered := 0
	for i := range editors {
		e := NewCodeEditor()
		e.gitService = s
		e.SigGitStatusChanged(func() { delivered++ })
		e.SetFilePath(fmt.Sprintf("%d.go", i))
		editors[i] = e
	}
	defer func() {
		for _, e := range editors {
			e.CancelGitStatus()
		}
	}()
	waitGitIdle(t, s)
	if len(q) != 1 {
		t.Fatalf("got %d GUI posts for 64 results; want one", len(q))
	}
	for _, e := range editors[:32] {
		e.CancelGitStatus()
	}
	(<-q)()
	if delivered != 32 {
		t.Fatal("closed editors received stale delivery", delivered)
	}
	for _, e := range editors[32:] {
		if e.GitStatusLoading() || e.GitLineStatuses()[1] != GitAdded {
			t.Fatal("live editor result missing")
		}
	}
}

func TestEditorGitShutdownWaitsForActualWorkerExit(t *testing.T) {
	q := make(chan func(), 8)
	entered := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var released sync.Once
	defer released.Do(func() { close(release) })
	s := newGitStatusService(1, func(ctx context.Context, _ string) (map[int]GitLineStatus, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return nil, ctx.Err()
	}, func(fn func()) { q <- fn })
	e := NewCodeEditor()
	e.gitService = s
	e.SetFilePath("active.go")
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	queued := NewCodeEditor()
	queued.gitService = s
	queued.SetFilePath("pending.go")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("shutdown returned before I/O cleanup", err)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("active job was not canceled")
	}
	queued.RefreshGitStatus()
	if queued.GitStatusLoading() {
		t.Fatal("terminal service accepted more work")
	}
	released.Do(func() { close(release) })
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := s.shutdown(ctx2); err != nil {
		t.Fatal(err)
	}
	waitGitIdle(t, s)
	for len(q) > 0 {
		(<-q)()
	}
	if len(e.GitLineStatuses()) != 0 || len(queued.GitLineStatuses()) != 0 {
		t.Fatal("shutdown published stale results")
	}
	e.CancelGitStatus()
	queued.CancelGitStatus()
}
