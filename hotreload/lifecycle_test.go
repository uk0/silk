package hotreload

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/fswatch"
)

func awaitReloadSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("reload lifecycle timeout")
	}
}
func reloadTestInstance(t *testing.T, fn ReloadFunc, onError ErrorFunc, limit int) *Reloader {
	t.Helper()
	r, e := New(fn, onError, Options{Debounce: time.Millisecond, PollInterval: time.Hour, MaxPending: limit})
	if e != nil {
		t.Fatal(e)
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
func registerReloadState(t *testing.T, r *Reloader, path string) {
	t.Helper()
	if err := r.WatchFileState(path, time.Time{}, 1); err != nil {
		t.Fatal(err)
	}
}
func TestReloaderSerialLatestCoalescingAndBoundedOverflow(t *testing.T) {
	root := t.TempDir()
	a, b, c := filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "c")
	entered, release := make(chan struct{}), make(chan struct{})
	var version atomic.Int32
	version.Store(1)
	var active, maxActive atomic.Int32
	var mu sync.Mutex
	seen := map[string][]string{}
	var reported uint64
	r := reloadTestInstance(t, func(p string, n *decl.Node) error {
		now := active.Add(1)
		defer active.Add(-1)
		for {
			old := maxActive.Load()
			if now <= old || maxActive.CompareAndSwap(old, now) {
				break
			}
		}
		title := n.Props[0].Value.(decl.Lit).V.(string)
		mu.Lock()
		first := p == a && len(seen[a]) == 0
		seen[p] = append(seen[p], title)
		mu.Unlock()
		if first {
			close(entered)
			<-release
		}
		return nil
	}, func(_ string, err error) {
		if errors.Is(err, ErrPendingLimit) {
			mu.Lock()
			reported++
			mu.Unlock()
		} else {
			t.Errorf("unexpected callback error: %v", err)
		}
	}, 2)
	r.load = func(string) (*decl.Node, error) { return decl.Form(decl.P("title", fmt.Sprint(version.Load()))), nil }
	for _, p := range []string{a, b, c} {
		registerReloadState(t, r, p)
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	r.scheduleRebuild(a)
	awaitReloadSignal(t, entered)
	version.Store(2)
	for i := 0; i < 5000; i++ {
		r.scheduleRebuild(a)
		r.scheduleRebuild(b)
		r.scheduleRebuild(c)
	}
	s := r.Stats()
	if s.Pending != 2 || s.Rejected != 5000 || !s.Active {
		t.Fatal("burst was not bounded/coalesced", s)
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		done := len(seen[a]) == 2 && len(seen[b]) == 1 && reported > 0
		mu.Unlock()
		if done && r.Stats().Pending == 0 && !r.Stats().Active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("latest queued work never drained", r.Stats())
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxActive.Load() != 1 || fmt.Sprint(seen[a]) != "[1 2]" || fmt.Sprint(seen[b]) != "[2]" || len(seen[c]) != 0 {
		t.Fatal("overlap, lost latest state or admitted overflow", maxActive.Load(), seen)
	}
}
func TestReloaderConcurrentStopJoinsAdmittedCallback(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	r := reloadTestInstance(t, func(string, *decl.Node) error { close(entered); <-release; return nil }, nil, 0)
	r.load = func(string) (*decl.Node, error) { return decl.Form(), nil }
	p := filepath.Join(t.TempDir(), "a")
	registerReloadState(t, r, p)
	r.scheduleRebuild(p)
	awaitReloadSignal(t, entered)
	var once sync.Once
	defer once.Do(func() { close(release) })
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.Stop() }()
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		t.Fatal("Stop returned before active callback finished")
	case <-time.After(50 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	awaitReloadSignal(t, done)
	if s := r.Stats(); !s.Closed || s.Active || s.Pending != 0 {
		t.Fatal("physical shutdown not reflected", s)
	}
	if err := r.Watch(p); !errors.Is(err, fswatch.ErrClosed) {
		t.Fatal("stopped reloader admitted Watch", err)
	}
	if err := r.Wait(nil); err == nil {
		t.Fatal("accepted nil wait context")
	}
}
func TestReloaderCloseDoesNotJoinBlockedReadAndDropsLateError(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r := reloadTestInstance(t, func(string, *decl.Node) error { calls.Add(1); return nil }, func(string, error) { calls.Add(100) }, 0)
	r.load = func(string) (*decl.Node, error) { close(entered); <-release; return nil, errors.New("obsolete read") }
	p := filepath.Join(t.TempDir(), "a")
	registerReloadState(t, r, p)
	r.scheduleRebuild(p)
	awaitReloadSignal(t, entered)
	r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := r.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatal("Wait falsely joined a blocked read", err)
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := r.Wait(ctx2); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("closed read published callback/error")
	}
}
func TestReloaderUnwatchRewatchRejectsOldInputAndDirectoryChildren(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(fmt.Sprint(directory), func(t *testing.T) {
			entered, release, got := make(chan struct{}), make(chan struct{}), make(chan struct{}, 4)
			var reads atomic.Int32
			var calls atomic.Int32
			root := t.TempDir()
			path := filepath.Join(root, "a")
			watched := path
			if directory {
				watched = root
			}
			r := reloadTestInstance(t, func(string, *decl.Node) error { calls.Add(1); got <- struct{}{}; return nil }, func(string, error) { t.Error("stale error delivered") }, 0)
			r.load = func(string) (*decl.Node, error) {
				if reads.Add(1) == 1 {
					close(entered)
					<-release
				}
				return decl.Form(), nil
			}
			registerReloadState(t, r, watched)
			r.scheduleRebuild(path)
			awaitReloadSignal(t, entered)
			r.Unwatch(watched)
			registerReloadState(t, r, watched)
			r.scheduleRebuild(path)
			close(release)
			awaitReloadSignal(t, got)
			r.Stop()
			if reads.Load() != 2 || calls.Load() != 1 {
				t.Fatal("old generation was delivered or latest lost", reads.Load(), calls.Load())
			}
		})
	}
}
func TestReloaderWatchIntentCanBeRetiredWhileRegistrationIsBlocked(t *testing.T) {
	for _, closeIt := range []bool{false, true} {
		t.Run(fmt.Sprint(closeIt), func(t *testing.T) {
			r := reloadTestInstance(t, func(string, *decl.Node) error { return nil }, nil, 0)
			p := filepath.Join(t.TempDir(), "a")
			entered, release, leader, duplicate := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan error, 1)
			go func() { leader <- r.watch(p, func(string) error { close(entered); <-release; return nil }) }()
			awaitReloadSignal(t, entered)
			go func() { duplicate <- r.Watch(p) }()
			want := fswatch.ErrSuperseded
			if closeIt {
				want = fswatch.ErrClosed
				r.Close()
			} else {
				r.Unwatch(p)
			}
			// A duplicate may race after Unwatch; exercise only an admitted request in
			// the deterministic case below. The leader must never revive retired intent.
			close(release)
			if err := <-leader; !errors.Is(err, want) {
				t.Fatal("late registration revived", err)
			}
			if err := <-duplicate; closeIt && !errors.Is(err, fswatch.ErrClosed) {
				t.Fatal(err)
			}
			r.mu.Lock()
			roots := len(r.roots)
			r.mu.Unlock()
			if !closeIt && roots > 0 {
				r.Unwatch(p)
			}
		})
	}
}
func TestReloaderCallbackCanCloseAndPanicsDoNotKillWorker(t *testing.T) {
	got := make(chan struct{}, 4)
	var n atomic.Int32
	var r *Reloader
	r = reloadTestInstance(t, func(string, *decl.Node) error {
		if n.Add(1) == 1 {
			panic("host panic")
		}
		r.Close()
		got <- struct{}{}
		return errors.New("closed callback error")
	}, func(_ string, e error) {
		if e == nil {
			t.Error("nil panic report")
		}
		got <- struct{}{}
		panic("error hook panic")
	}, 0)
	r.load = func(string) (*decl.Node, error) { return decl.Form(), nil }
	p := filepath.Join(t.TempDir(), "a")
	registerReloadState(t, r, p)
	r.scheduleRebuild(p)
	awaitReloadSignal(t, got)
	// First hook panic must not kill the worker or strand the second job.
	r.scheduleRebuild(p)
	awaitReloadSignal(t, got)
	r.Stop()
	if n.Load() != 2 {
		t.Fatal("worker failed to survive hook panic")
	}
}
func TestReloaderRootPrefixAndNegativeCapacity(t *testing.T) {
	if _, e := New(func(string, *decl.Node) error { return nil }, nil, Options{MaxPending: -1}); e == nil {
		t.Fatal("accepted negative capacity")
	}
	r := reloadTestInstance(t, func(string, *decl.Node) error { return nil }, nil, 0)
	p := t.TempDir()
	root := filepath.VolumeName(p) + string(filepath.Separator)
	request := &watchRequest{path: root, done: make(chan struct{})}
	r.mu.Lock()
	r.roots[root] = request
	got := r.findRootLocked(p)
	r.mu.Unlock()
	if got != request {
		t.Fatal("filesystem-root prefix did not match")
	}
}

func TestReloaderQueuedEventWaitsForWatchRegistration(t *testing.T) {
	got := make(chan struct{}, 1)
	r := reloadTestInstance(t, func(string, *decl.Node) error { got <- struct{}{}; return nil }, nil, 0)
	var reads atomic.Int32
	r.load = func(string) (*decl.Node, error) { reads.Add(1); return decl.Form(), nil }
	p := filepath.Join(t.TempDir(), "a")
	entered, release, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { result <- r.watch(p, func(string) error { close(entered); <-release; return nil }) }()
	awaitReloadSignal(t, entered)
	r.scheduleRebuild(p)
	time.Sleep(20 * time.Millisecond)
	if s := r.Stats(); s.Pending != 1 || s.Active || reads.Load() != 0 {
		close(release)
		t.Fatal("event consumed before registration could commit", s)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	awaitReloadSignal(t, got)
}
func TestReloaderRebasesMostSpecificRootWithoutOldCallback(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a")
	entered, release, got := make(chan struct{}), make(chan struct{}), make(chan struct{}, 2)
	var reads, calls atomic.Int32
	r := reloadTestInstance(t, func(string, *decl.Node) error { calls.Add(1); got <- struct{}{}; return nil }, nil, 0)
	r.load = func(string) (*decl.Node, error) {
		if reads.Add(1) == 1 {
			close(entered)
			<-release
		}
		return decl.Form(), nil
	}
	registerReloadState(t, r, root)
	r.scheduleRebuild(child)
	awaitReloadSignal(t, entered)
	registerReloadState(t, r, child)
	r.scheduleRebuild(child)
	r.Unwatch(child)
	close(release)
	deadline := time.Now().Add(time.Second)
	for r.Stats().Active {
		if time.Now().After(deadline) {
			t.Fatal("retired worker did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 0 || r.Stats().Pending != 0 {
		t.Fatal("old parent input survived more-specific root retirement")
	}
	r.scheduleRebuild(child)
	awaitReloadSignal(t, got)
	if calls.Load() != 1 {
		t.Fatal("remaining parent watch failed")
	}
}
