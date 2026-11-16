package async

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func awaitWrite[T any](t *testing.T, f *Future[T]) (T, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return f.Await(ctx)
}

func TestWriteQueueBlockedCancellationKeepsOnePhysicalAndLatestQueued(t *testing.T) {
	q := NewWriteQueue[int]()
	defer q.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var active, peak, calls atomic.Int32
	work := func(n int) func(context.Context) (int, error) {
		return func(context.Context) (int, error) {
			a := active.Add(1)
			defer active.Add(-1)
			for p := peak.Load(); a > p; p = peak.Load() {
				if peak.CompareAndSwap(p, a) {
					break
				}
			}
			calls.Add(1)
			if n == 1 {
				close(entered)
				<-release
			}
			return n, nil
		}
	}
	first, err := q.Submit(context.Background(), work(1))
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	first.Cancel()
	var last *Future[int]
	for i := 2; i <= 100; i++ {
		next, err := q.Submit(context.Background(), work(i))
		if err != nil {
			t.Fatal(err)
		}
		if last != nil {
			if _, err := awaitWrite(t, last); !errors.Is(err, ErrSuperseded) {
				t.Fatal("replaced future lost explicit outcome", err)
			}
		}
		last = next
	}
	if s := q.Stats(); !s.Active || !s.Queued || calls.Load() != 1 {
		t.Fatal("cancellation replaced a blocked writer", s, calls.Load())
	}
	unblock()
	if n, err := awaitWrite(t, first); n != 1 || err != nil {
		t.Fatal("physical success was replaced by logical cancellation", n, err)
	}
	if n, err := awaitWrite(t, last); n != 100 || err != nil {
		t.Fatal(n, err)
	}
	q.Close()
	if err := q.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 1 || calls.Load() != 2 {
		t.Fatal("physical bound or latest queue violated", peak.Load(), calls.Load())
	}
}

func TestWriteQueueClosePreservesRealFailureAndDoesNotJoinUI(t *testing.T) {
	q := NewWriteQueue[string]()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	cause := errors.New("physical sync failed")
	f, _ := q.Submit(context.Background(), func(context.Context) (string, error) { close(entered); <-release; return "recovery copy", cause })
	<-entered
	queued, _ := q.Submit(context.Background(), func(context.Context) (string, error) { t.Error("closed queued task ran"); return "", nil })
	q.Close()
	if _, err := awaitWrite(t, queued); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := q.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Close or cancellation released physical slot", err)
	}
	if _, err := q.Submit(context.Background(), func(context.Context) (string, error) { return "", nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	unblock()
	if value, err := awaitWrite(t, f); value != "recovery copy" || !errors.Is(err, cause) {
		t.Fatal("real failure lost on close", value, err)
	}
	if err := q.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWriteQueueQueuedCancellationNeverInvokesSideEffect(t *testing.T) {
	q := NewWriteQueue[int]()
	defer q.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f, _ := q.Submit(context.Background(), func(context.Context) (int, error) { close(entered); <-release; return 1, nil })
	<-entered
	next, _ := q.Submit(context.Background(), func(context.Context) (int, error) { t.Error("canceled queued write ran"); return 2, nil })
	next.Cancel()
	unblock()
	_, _ = awaitWrite(t, f)
	if _, err := awaitWrite(t, next); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestWriteQueueWorkPanicAndGoexitDoNotStrandNext(t *testing.T) {
	for _, kind := range []string{"panic", "goexit"} {
		t.Run(kind, func(t *testing.T) {
			q := NewWriteQueue[int]()
			defer q.Close()
			entered, release := make(chan struct{}), make(chan struct{})
			f, _ := q.Submit(context.Background(), func(context.Context) (int, error) {
				close(entered)
				<-release
				if kind == "panic" {
					panic("write")
				}
				runtime.Goexit()
				return 0, nil
			})
			<-entered
			next, _ := q.Submit(context.Background(), func(context.Context) (int, error) { return 2, nil })
			close(release)
			if _, err := awaitWrite(t, f); err == nil {
				t.Fatal("aborted writer became successful")
			}
			if n, err := awaitWrite(t, next); n != 2 || err != nil {
				t.Fatal("next writer stranded", n, err)
			}
		})
	}
}

func TestWriteQueueReentrantCompletionAndBusyInterval(t *testing.T) {
	q := NewWriteQueue[int]()
	defer q.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	first, _ := q.Submit(context.Background(), func(context.Context) (int, error) { close(entered); <-release; return 1, nil })
	<-entered
	q.mu.Lock()
	interval := q.idle
	q.mu.Unlock()
	next := make(chan *Future[int], 1)
	_ = first.OnComplete(func(f func()) { f() }, func(int, error) {
		f, e := q.Submit(context.Background(), func(context.Context) (int, error) { return 2, nil })
		if e != nil {
			t.Error(e)
		}
		next <- f
	})
	close(release)
	_, _ = awaitWrite(t, first)
	var f *Future[int]
	select {
	case f = <-next:
	case <-time.After(5 * time.Second):
		t.Fatal("completion deadlocked queue mutex")
	}
	if n, e := awaitWrite(t, f); n != 2 || e != nil {
		t.Fatal(n, e)
	}
	q.Close()
	if e := q.Wait(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-interval:
	default:
		t.Fatal("a waiter on the original busy interval was stranded by a submit during completion")
	}
}

func TestWriteQueueZeroValueAndInvalidInputs(t *testing.T) {
	var q WriteQueue[int]
	if err := q.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Submit(nil, func(context.Context) (int, error) { return 1, nil }); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := q.Submit(context.Background(), nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := q.Submit(ctx, func(context.Context) (int, error) { t.Error("canceled task ran"); return 1, nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	f, err := q.Submit(context.Background(), func(context.Context) (int, error) { return 7, nil })
	if err != nil {
		t.Fatal(err)
	}
	if n, err := awaitWrite(t, f); n != 7 || err != nil {
		t.Fatal(n, err)
	}
	q.Close()
	if err := q.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
