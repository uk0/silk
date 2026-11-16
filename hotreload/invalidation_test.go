package hotreload

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/decl"
)

func invalidationTestInstance(t *testing.T, change InvalidationFunc, onError ErrorFunc) *Reloader {
	t.Helper()
	r, err := NewInvalidations(change, onError, Options{Debounce: time.Millisecond, PollInterval: time.Hour, MaxPending: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := r.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	return r
}

func TestInvalidationsDoNotReadOrParseChangedSource(t *testing.T) {
	if _, err := NewInvalidations(nil, nil, Options{}); err == nil {
		t.Fatal("nil invalidation callback accepted")
	}
	if _, err := NewInvalidations(func(string) error { return nil }, nil, Options{MaxPending: -1}); err == nil {
		t.Fatal("negative capacity accepted")
	}
	path := filepath.Join(t.TempDir(), "not-created.silkui")
	got := make(chan string, 2)
	r := invalidationTestInstance(t, func(path string) error { got <- path; return nil }, nil)
	var reads atomic.Int32
	r.load = func(string) (*decl.Node, error) {
		reads.Add(1)
		return nil, errors.New("invalidation mode must not call the AST reader")
	}
	if err := r.WatchFileState(path, time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	r.scheduleRebuild(path)
	select {
	case value := <-got:
		if value != path {
			t.Fatal("changed path lost", value)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing source incorrectly prevented invalidation")
	}
	if reads.Load() != 0 {
		t.Fatal("invalidation performed a duplicate source read")
	}
}

func TestInvalidationsCoalesceWhileCallbackActiveAndJoinOnClose(t *testing.T) {
	entered, next, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var calls atomic.Int32
	r := invalidationTestInstance(t, func(string) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		} else {
			close(next)
		}
		return nil
	}, nil)
	t.Cleanup(unblock)
	path := filepath.Join(t.TempDir(), "source.silkui")
	if err := r.WatchFileState(path, time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	r.scheduleRebuild(path)
	awaitReloadSignal(t, entered)
	for i := 0; i < 1000; i++ {
		r.scheduleRebuild(path)
	}
	if state := r.Stats(); state.Pending != 1 || !state.Active {
		t.Fatal("burst exceeded the shared pending/worker budget", state)
	}
	unblock()
	awaitReloadSignal(t, next)
	r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || r.Stats().Active {
		t.Fatal("coalescing/physical join changed", calls.Load(), r.Stats())
	}
}

func TestInvalidationErrorsAndPanicsDoNotKillWorker(t *testing.T) {
	want := errors.New("host invalidation failure")
	errorsSeen := make(chan error, 3)
	var calls atomic.Int32
	r := invalidationTestInstance(t, func(string) error {
		if calls.Add(1) == 1 {
			panic("host panic")
		}
		return want
	}, func(_ string, err error) { errorsSeen <- err })
	path := filepath.Join(t.TempDir(), "source.silkui")
	if err := r.WatchFileState(path, time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r.scheduleRebuild(path)
		select {
		case err := <-errorsSeen:
			if err == nil || i == 1 && !errors.Is(err, want) {
				t.Fatal("host failure lost", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("callback panic killed the worker")
		}
	}
}

func TestInvalidationCallbackCanCloseWithoutJoiningItself(t *testing.T) {
	called := make(chan struct{})
	var r *Reloader
	r = invalidationTestInstance(t, func(string) error { r.Close(); close(called); return nil }, nil)
	path := filepath.Join(t.TempDir(), "source.silkui")
	if err := r.WatchFileState(path, time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	r.scheduleRebuild(path)
	awaitReloadSignal(t, called)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}
