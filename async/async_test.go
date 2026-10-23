package async

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func timeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func TestRunSubmitCapacityCancelAndClose(t *testing.T) {
	ctx := timeout(t)
	e := NewExecutor(1)
	defer e.Close()
	if v, err := Run(ctx, func(context.Context) (int, error) { return 42, nil }); v != 42 || err != nil {
		t.Fatal(v, err)
	}
	started := make(chan struct{})
	f, err := Submit(ctx, e, func(ctx context.Context) (int, error) { close(started); <-ctx.Done(); return 0, ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := Submit(ctx, e, func(context.Context) (int, error) { return 1, nil }); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	early, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.Await(early); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-f.Done():
		t.Fatal("Await canceled the job")
	default:
	}
	f.Cancel()
	if _, err := f.Await(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	again, err := Submit(ctx, e, func(context.Context) (int, error) { return 7, nil })
	if err != nil {
		t.Fatal(err)
	}
	if v, err := again.Await(ctx); v != 7 || err != nil {
		t.Fatal(v, err)
	}
	e.Close()
	if err := e.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := Submit(ctx, e, func(context.Context) (int, error) { return 1, nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
func TestPanicAndGoexitRetireSlot(t *testing.T) {
	ctx := timeout(t)
	e := NewExecutor(1)
	defer e.Close()
	for _, work := range []func(context.Context) (int, error){
		func(context.Context) (int, error) { panic("boom") },
		func(context.Context) (int, error) { runtime.Goexit(); return 0, nil },
	} {
		f, err := Submit(ctx, e, work)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Await(ctx); err == nil {
			t.Fatal("missing failure")
		}
		if err = e.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
func TestCompletionConcurrentRegistrationAndLateObserver(t *testing.T) {
	ctx := timeout(t)
	e := NewExecutor(1)
	defer e.Close()
	release := make(chan struct{})
	f, err := Submit(ctx, e, func(context.Context) (int, error) { <-release; return 42, nil })
	if err != nil {
		t.Fatal(err)
	}
	queue := make(chan func(), 101)
	dispatch := func(fn func()) { queue <- fn }
	var registrations sync.WaitGroup
	count := 0
	callback := func(v int, err error) {
		if err != nil || v != 42 {
			t.Errorf("%d %v", v, err)
		}
		count++
	}
	for i := 0; i < 100; i++ {
		registrations.Add(1)
		go func() {
			defer registrations.Done()
			if err := f.OnComplete(dispatch, callback); err != nil {
				t.Error(err)
			}
		}()
	}
	close(release)
	registrations.Wait()
	if _, err := f.Await(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.OnComplete(dispatch, callback); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("callback bypassed dispatcher")
	}
	for i := 0; i < 101; i++ {
		select {
		case fn := <-queue:
			fn()
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if count != 101 {
		t.Fatal(count)
	}
	if err := f.OnComplete(nil, callback); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
func TestExecutorCloseCancelsAllAndInvalidInputs(t *testing.T) {
	ctx := timeout(t)
	e := NewExecutor(2)
	var jobs []*Future[int]
	for i := 0; i < 2; i++ {
		f, err := Submit(ctx, e, func(ctx context.Context) (int, error) { <-ctx.Done(); return 0, ctx.Err() })
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, f)
	}
	e.Close()
	if err := e.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	for _, f := range jobs {
		if _, err := f.Await(ctx); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if _, err := Run[int](nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := Submit[int](ctx, nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
