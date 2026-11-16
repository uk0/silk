package fswatch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCapturedFileStateRegistersWithoutIOAndDetectsInterveningChange(t *testing.T) {
	w := New()
	w.SetPollInterval(time.Hour)
	defer w.Stop()
	calls := 0
	w.snapshotFn = func(string) *snapshot { calls++; return &snapshot{exists: true, size: 9} }
	p := filepath.Join(t.TempDir(), "captured")
	if err := w.AddFileState(p, time.Time{}, 3); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("captured state performed synchronous filesystem I/O")
	}
	w.tick()
	if calls != 1 {
		t.Fatal("poll skipped current state")
	}
	select {
	case e := <-w.Events():
		if e.Path != p || e.Kind != Modified {
			t.Fatal(e)
		}
	default:
		t.Fatal("edit since captured state was lost")
	}
	if err := w.AddFileState(p, time.Time{}, -1); err == nil {
		t.Fatal("accepted negative file size")
	}
}
func TestWatcherCloseAndRemoveRetireBlockedRegistrationWithoutHoldingLock(t *testing.T) {
	for _, closeIt := range []bool{false, true} {
		t.Run(fmt.Sprint(closeIt), func(t *testing.T) {
			w := New()
			w.SetPollInterval(time.Hour)
			defer w.Stop()
			entered, release, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			w.snapshotFn = func(string) *snapshot { close(entered); <-release; return &snapshot{exists: true} }
			p := filepath.Join(t.TempDir(), "a")
			go func() { result <- w.AddPath(p) }()
			<-entered
			// These calls deadlock if the blocked filesystem snapshot holds mu.
			done := make(chan struct{})
			go func() {
				_ = w.Paths()
				if closeIt {
					w.Close()
				} else {
					w.RemovePath(p)
				}
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				close(release)
				t.Fatal("filesystem read blocked watcher control lock")
			}
			close(release)
			err := <-result
			want := ErrSuperseded
			if closeIt {
				want = ErrClosed
			}
			if !errors.Is(err, want) || len(w.Paths()) != 0 {
				t.Fatal("late AddPath revived retired watch", err)
			}
			if closeIt {
				if err := w.AddFileState(p, time.Now(), 0); !errors.Is(err, ErrClosed) {
					t.Fatal("closed watcher restarted", err)
				}
			}
		})
	}
}
func TestWatcherRemoveDuringSnapshotDoesNotReinsertOrEmit(t *testing.T) {
	w := New()
	w.SetPollInterval(time.Hour)
	defer w.Stop()
	p := filepath.Join(t.TempDir(), "a")
	if err := w.AddFileState(p, time.Time{}, 1); err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	w.snapshotFn = func(string) *snapshot { close(entered); <-release; return &snapshot{exists: true, size: 20} }
	go func() { w.tick(); close(done) }()
	<-entered
	w.RemovePath(p)
	close(release)
	<-done
	if len(w.Paths()) != 0 {
		t.Fatal("late tick reinserted removed watch")
	}
	select {
	case e := <-w.Events():
		t.Fatal("retired path emitted", e)
	default:
	}
}
func TestWatcherCloseIsNonblockingWhileWaitReportsPhysicalPoll(t *testing.T) {
	w := New()
	w.SetPollInterval(50 * time.Millisecond)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	w.snapshotFn = func(string) *snapshot {
		once.Do(func() { close(entered) })
		<-release
		return &snapshot{exists: true, size: 9}
	}
	if err := w.AddFileState(filepath.Join(t.TempDir(), "a"), time.Time{}, 1); err != nil {
		t.Fatal(err)
	}
	<-entered
	w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := w.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Wait falsely reported a stopped OS read", err)
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := w.Wait(ctx2); err != nil {
		t.Fatal(err)
	}
	if _, open := <-w.Events(); open {
		t.Fatal("closed/late polling emitted an event")
	}
	if err := w.Wait(nil); err == nil {
		t.Fatal("accepted nil wait context")
	}
}
func TestWatcherConcurrentRegistrationAndPermanentStop(t *testing.T) {
	for iteration := 0; iteration < 30; iteration++ {
		w := New()
		w.SetPollInterval(time.Hour)
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_ = w.AddFileState(fmt.Sprintf("captured-%d", i), time.Time{}, int64(i))
				if i%2 == 0 {
					w.Stop()
				} else {
					w.Close()
					_ = w.Wait(context.Background())
				}
			}(i)
		}
		wg.Wait()
		w.Stop()
		if _, open := <-w.Events(); open {
			t.Fatal("events not closed")
		}
		if err := w.AddPath("later"); !errors.Is(err, ErrClosed) {
			t.Fatal("permanent stop admitted new I/O", err)
		}
	}
}
