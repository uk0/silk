package gui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"
)

func TestSDKDispatchNaturalTabCaption(t *testing.T) {
	b := NewTabBar()
	b.AddTab(IconText{nil, "编辑器"}, true)
	b.SetSize(1000, 32)
	b.Layout()
	if b.tabs[0].paintLabel != "编辑器" {
		t.Fatalf("unconstrained dock title was elided: label=%q natural=%g painted=%g", b.tabs[0].paintLabel, b.tabs[0].SizeHints().Width, b.tabs[0].paintWidth)
	}
	f := Theme().Font
	title := "编辑器"
	width := f.TextExtents(title).Width
	// Layout adds margins, then subtracts them for its content rectangle. That
	// round trip can lose an ULP without losing a single visible glyph pixel.
	if got := elideToWidth(f, title, math.Nextafter(width, math.Inf(-1))); got != title {
		t.Fatalf("floating point round trip truncated a fitting caption: %q", got)
	}
}

func sdkDispatchFixture(t *testing.T, onUI bool) *atomic.Bool {
	t.Helper()
	resetUIQueue()
	old := uiThreadOwner
	flag := new(atomic.Bool)
	flag.Store(onUI)
	uiThreadOwner = flag.Load
	t.Cleanup(func() { resetUIQueue(); uiThreadOwner = old })
	return flag
}
func dispatchWait[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("UI dispatch did not complete")
		var zero T
		return zero
	}
}
func TestSDKDispatchInlineAndErrors(t *testing.T) {
	sdkDispatchFixture(t, true)
	if !IsUIThread() {
		t.Fatal("owner not detected")
	}
	got, err := InvokeValue(context.Background(), func() (int, error) { return 42, nil })
	if got != 42 || err != nil {
		t.Fatal(got, err)
	}
	cause := errors.New("callback failure")
	if err := Invoke(context.Background(), func() error { panic(cause) }); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	var panicErr *UIInvocationPanicError
	if err := Invoke(context.Background(), func() error { panic("boom") }); !errors.As(err, &panicErr) {
		t.Fatal(err)
	}
	if err := Invoke(nil, func() error { return nil }); !errors.Is(err, ErrUIInvalid) {
		t.Fatal(err)
	}
	if err := Invoke(context.Background(), nil); !errors.Is(err, ErrUIInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Invoke(ctx, func() error { t.Fatal("cancelled inline work executed"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	uiThreadOwner = nil
	if IsUIThread() {
		t.Fatal("missing owner accepted")
	}
	if _, err := InvokeValue(context.Background(), func() (int, error) { return 1, nil }); !errors.Is(err, ErrUIUnavailable) {
		t.Fatal(err)
	}
}
func TestSDKDispatchWorkerTypedResultAndReentrancy(t *testing.T) {
	flag := sdkDispatchFixture(t, false)
	wake := make(chan struct{}, 1)
	SetUIWakeup(func() { wake <- struct{}{} })
	result := make(chan uiInvocationResult[string], 1)
	go func() {
		v, e := InvokeValue(context.Background(), func() (string, error) {
			if !IsUIThread() {
				return "", errors.New("not on UI")
			}
			n, err := InvokeValue(context.Background(), func() (int, error) { return 7, nil })
			return fmt.Sprint(n), err
		})
		result <- uiInvocationResult[string]{v, e}
	}()
	dispatchWait(t, wake)
	flag.Store(true)
	drainUITasks()
	r := dispatchWait(t, result)
	if r.value != "7" || r.err != nil {
		t.Fatal(r)
	}
}
func TestSDKDispatchCancellationBeforeDelivery(t *testing.T) {
	sdkDispatchFixture(t, false)
	wake := make(chan struct{}, 1)
	SetUIWakeup(func() { wake <- struct{}{} })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	var calls atomic.Int32
	go func() { result <- Invoke(ctx, func() error { calls.Add(1); return nil }) }()
	dispatchWait(t, wake)
	cancel()
	if err := dispatchWait(t, result); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	drainUITasks()
	if calls.Load() != 0 {
		t.Fatal("cancelled queued callback still ran")
	}
	c := &uiInvocation[int]{fn: func() (int, error) { return 0, nil }, ctx: context.Background()}
	c.retire()
	if c.fn != nil || c.ctx != nil {
		t.Fatal("cancel retained captured state")
	}
}
func TestSDKDispatchCancellationDuringStartedCallback(t *testing.T) {
	sdkDispatchFixture(t, false)
	wake := make(chan struct{}, 1)
	SetUIWakeup(func() { wake <- struct{}{} })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release, drained := make(chan struct{}), make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := InvokeValue(ctx, func() (int, error) { close(started); <-release; return 9, nil })
		result <- err
	}()
	dispatchWait(t, wake)
	go func() { drainUITasks(); close(drained) }()
	dispatchWait(t, started)
	cancel()
	if err := dispatchWait(t, result); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	dispatchWait(t, drained)
}
func TestSDKDispatchQueuedPanicAndDeadline(t *testing.T) {
	sdkDispatchFixture(t, false)
	wake := make(chan struct{}, 1)
	SetUIWakeup(func() { wake <- struct{}{} })
	result := make(chan error, 1)
	go func() { result <- Invoke(context.Background(), func() error { panic("queued panic") }) }()
	dispatchWait(t, wake)
	drainUITasks()
	var p *UIInvocationPanicError
	if err := dispatchWait(t, result); !errors.As(err, &p) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := Invoke(ctx, func() error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestSDKDispatchContextPostAndLatest(t *testing.T) {
	sdkDispatchFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	if err := PostContext(ctx, func() { calls++ }); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := PostContext(ctx, func() { calls++ }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := PostContext(nil, func() {}); !errors.Is(err, ErrUIInvalid) {
		t.Fatal(err)
	}
	if err := PostContext(context.Background(), nil); !errors.Is(err, ErrUIInvalid) {
		t.Fatal(err)
	}
	if err := PostContext(context.Background(), func() { calls += 2 }); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("PostContext ran inline")
	}
	drainUITasks()
	if calls != 2 {
		t.Fatal(calls)
	}
	var values []int
	latest, err := NewLatestUI(func(v int) { values = append(values, v) })
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100000; i++ {
		if !latest.Publish(i) {
			t.Fatal("publish rejected")
		}
	}
	uiTaskMu.Lock()
	n := len(uiTasks)
	uiTaskMu.Unlock()
	if n != 1 {
		t.Fatal("burst grew UI queue", n)
	}
	drainUITasks()
	if len(values) != 1 || values[0] != 99999 {
		t.Fatal(values)
	}
	latest.Publish(100000)
	latest.Close()
	drainUITasks()
	if len(values) != 1 || latest.Publish(5) {
		t.Fatal("closed receiver delivered stale value")
	}
	if _, err := NewLatestUI[int](nil); err == nil {
		t.Fatal("nil consumer accepted")
	}
}
func TestSDKDispatchCheckedAttachmentRejectsBeforeMutation(t *testing.T) {
	sdkDispatchFixture(t, false)
	w := NewForm()
	before := w.wt
	if err := w.TryAttachWindow(WtForm); !errors.Is(err, ErrUIThread) {
		t.Fatal(err)
	}
	if w.wt != before || w.win != nil {
		t.Fatal("wrong-thread attach mutated widget")
	}
	uiThreadOwner = func() bool { return true }
	if err := w.TryAttachWindow(WindowType(-1)); err == nil {
		t.Fatal("invalid window type accepted")
	}
	if err := w.TryAttachWindow(WtChild); err == nil {
		t.Fatal("orphan child accepted")
	}
	if w.wt != before || w.win != nil {
		t.Fatal("invalid attach mutated widget")
	}
	w.win = &Window{}
	if err := w.TryAttachWindow(WtChild); err == nil {
		t.Fatal("existing window incorrectly served as its own child owner")
	}
	if w.win == nil {
		t.Fatal("invalid conversion destroyed existing window")
	}
}
func BenchmarkSDKDispatchInline(b *testing.B) {
	old := uiThreadOwner
	uiThreadOwner = func() bool { return true }
	defer func() { uiThreadOwner = old }()
	ctx := context.Background()
	fn := func() (int, error) { return 42, nil }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := InvokeValue(ctx, fn)
		if v != 42 || err != nil {
			b.Fatal(v, err)
		}
	}
}
