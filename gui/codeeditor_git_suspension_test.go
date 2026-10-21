package gui

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEditorGitPathSuspensionJoinsRetiredJobsAndCoalescesRefresh(t *testing.T) {
	root := filepath.Join(t.TempDir(), "source")
	old, next, other := filepath.Join(root, "old.go"), filepath.Join(root, "next.go"), root+"-sibling.go"
	q := make(chan func(), 8)
	calls := make(chan string, 8)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	s := newGitStatusService(2, func(ctx context.Context, path string) (map[int]GitLineStatus, error) {
		calls <- path
		if path == old {
			<-release
		}
		return map[int]GitLineStatus{1: GitModified}, nil
	}, func(fn func()) { q <- fn })
	a, b := NewCodeEditor(), NewCodeEditor()
	a.gitService = s
	b.gitService = s
	defer a.CancelGitStatus()
	defer b.CancelGitStatus()
	a.SetFilePath(old)
	select {
	case p := <-calls:
		if p != old {
			t.Fatal(p)
		}
	case <-time.After(time.Second):
		t.Fatal("worker not started")
	}
	// Even a closed tab's still-reaping job must be included in the reservation.
	a.CancelGitStatus()
	gate, err := s.suspendPath(root)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Release()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.Wait(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel mistaken for child exit", err)
	}
	a.SetFilePath(next)
	for i := 0; i < 10000; i++ {
		a.RefreshGitStatus()
	}
	a.SetText("package p\n")
	a.ReplaceAllText("package p\nvar Value int\n")
	a.Undo()
	if a.Text() != "package p\n" {
		t.Fatal("suspension blocked typing/undo")
	}
	b.SetFilePath(other)
	select {
	case p := <-calls:
		if p != other {
			t.Fatal("held path ran or unrelated path starved", p)
		}
	case <-time.After(time.Second):
		t.Fatal("unrelated worker blocked")
	}
	s.mu.Lock()
	held, queued, active := len(s.held), s.queue.Len(), len(s.active)
	s.mu.Unlock()
	if held != 1 || queued != 0 || active > 2 {
		t.Fatal("unbounded suspension state", held, queued, active)
	}
	nested, err := s.suspendPath(root)
	if err != nil {
		t.Fatal(err)
	}
	defer nested.Release()
	once.Do(func() { close(release) })
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := gate.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	gate.Release()
	gate.Release()
	s.mu.Lock()
	held = len(s.held)
	s.mu.Unlock()
	if held != 1 {
		t.Fatal("outer release ignored nested reservation")
	}
	select {
	case p := <-calls:
		t.Fatal("work ran before last release", p)
	default:
	}
	nested.Release()
	select {
	case p := <-calls:
		if p != next {
			t.Fatal("resumed stale path", p)
		}
	case <-time.After(time.Second):
		t.Fatal("latest request not resumed")
	}
	waitGitUI(t, q, func() bool { return !a.GitStatusLoading() && !b.GitStatusLoading() })
	waitGitIdle(t, s)
	select {
	case p := <-calls:
		t.Fatal("refresh storm replayed", p)
	default:
	}
	if a.GitStatusError() != nil || len(a.GitLineStatuses()) != 1 {
		t.Fatal("cancellation leaked as current failure")
	}
}

func TestEditorGitSuspensionRetiresQueuedDeliveriesAndShutdownHeld(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	q := make(chan func(), 8)
	var calls atomic.Int32
	s := newGitStatusService(2, func(context.Context, string) (map[int]GitLineStatus, error) {
		calls.Add(1)
		return map[int]GitLineStatus{1: GitAdded}, nil
	}, func(fn func()) { q <- fn })
	e := NewCodeEditor()
	e.gitService = s
	defer e.CancelGitStatus()
	e.SetFilePath(path)
	waitGitIdle(t, s)
	gate, err := s.suspendPath(root)
	if err != nil {
		t.Fatal(err)
	}
	for len(q) > 0 {
		(<-q)()
	}
	if len(e.GitLineStatuses()) != 0 {
		t.Fatal("pre-reservation result repainted")
	}
	gate.Release()
	waitGitUI(t, q, func() bool { return !e.GitStatusLoading() })
	waitGitIdle(t, s)
	if calls.Load() != 2 {
		t.Fatal("fresh resume not coalesced", calls.Load())
	}
	gate, err = s.suspendPath(root)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		e.RefreshGitStatus()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	gate.Release()
	e.RefreshGitStatus()
	s.mu.Lock()
	held, queued, pauses := len(s.held), s.queue.Len(), len(s.suspensions)
	s.mu.Unlock()
	if held != 0 || queued != 0 || pauses != 0 || e.GitStatusLoading() || calls.Load() != 2 {
		t.Fatal("shutdown restarted or retained paused work", held, queued, pauses, calls.Load())
	}
	if _, err := s.suspendPath(""); err == nil {
		t.Fatal("empty reservation accepted")
	}
}

func TestEditorGitSuspensionFromCallbackRetiresDetachedBatch(t *testing.T) {
	root := t.TempDir()
	q := make(chan func(), 8)
	s := newGitStatusService(2, func(context.Context, string) (map[int]GitLineStatus, error) {
		return map[int]GitLineStatus{1: GitModified}, nil
	}, func(fn func()) { q <- fn })
	a, b := NewCodeEditor(), NewCodeEditor()
	a.gitService = s
	b.gitService = s
	defer a.CancelGitStatus()
	defer b.CancelGitStatus()
	var gate *GitPathSuspension
	applies := 0
	callback := func() {
		applies++
		if gate == nil {
			var err error
			gate, err = s.suspendPath(root)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	a.SigGitStatusChanged(callback)
	b.SigGitStatusChanged(callback)
	a.SetFilePath(filepath.Join(root, "a.go"))
	b.SetFilePath(filepath.Join(root, "b.go"))
	waitGitIdle(t, s)
	if len(q) != 1 {
		t.Fatal("fixture was not one coalesced UI batch", len(q))
	}
	(<-q)()
	if applies != 1 || gate == nil {
		t.Fatal("detached stale result escaped suspension", applies)
	}
	gate.Release()
	waitGitUI(t, q, func() bool { return applies == 2 })
	waitGitIdle(t, s)
}
