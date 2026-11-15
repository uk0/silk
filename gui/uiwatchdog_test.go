package gui

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestUIHeartbeatRecordsProgress checks the cheap path: every event-loop
// iteration must move the timestamp forward, since the watchdog's whole
// judgement rests on it.
func TestUIHeartbeatRecordsProgress(t *testing.T) {
	uiHeartbeat()
	first := uiLastBeat.Load()
	if first == 0 {
		t.Fatal("uiHeartbeat did not record a timestamp")
	}
	time.Sleep(2 * time.Millisecond)
	uiHeartbeat()
	if second := uiLastBeat.Load(); second <= first {
		t.Errorf("heartbeat did not advance: %d -> %d", first, second)
	}
}

func waitUIWatchDone(t *testing.T, run *uiWatchRun) {
	t.Helper()
	select {
	case <-run.done:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop")
	}
}

func TestUIWatchdogPumpOwnsOneWorkerAndRearmsAfterLastLease(t *testing.T) {
	var w uiPumpWatchdog
	w.beat(uiWatchTick())
	if w.active.Load() != nil {
		t.Fatal("construction heartbeat armed watchdog without a pump")
	}
	outer := w.enter()
	run := w.active.Load()
	inner := w.enter()
	for i := 0; i < 100; i++ {
		w.beat(uiWatchTick())
	}
	if w.active.Load() != run {
		t.Fatal("nested pump or heartbeat created another worker")
	}
	inner()
	inner()
	if w.active.Load() != run || run.stopped.Load() {
		t.Fatal("nested unwind stopped outer pump")
	}
	// Deliberately invalidate the retired run's timestamp. Monotonic clocks
	// need not advance between adjacent reads (notably on Windows); identity,
	// fresh bounded state and cancellation matter, not a strictly newer tick.
	run.last.Store(0)
	outer()
	outer()
	waitUIWatchDone(t, run)
	w.beat(uiWatchTick())
	if w.active.Load() != nil {
		t.Fatal("teardown heartbeat resurrected worker")
	}
	before := uiWatchTick()
	closeNext := w.enter()
	next := w.active.Load()
	if next == run || next.stopped.Load() || next.last.Load() == 0 || next.last.Load() < before || next.last.Load() > uiWatchTick() {
		t.Fatal("new pump inherited old deadline or cancellation")
	}
	closeNext()
	waitUIWatchDone(t, next)
}

func TestUIWatchdogLeasesCancelOnPanicAndConcurrentUnwind(t *testing.T) {
	var w uiPumpWatchdog
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			closeLease := w.enter()
			run := w.active.Load()
			defer waitUIWatchDone(t, run)
			defer func() { _ = recover() }()
			defer closeLease()
			closeLease()
			panic("pump failure")
		}()
	}
	wg.Wait()
	if w.active.Load() != nil || w.depth != 0 {
		t.Fatal("unwind retained a pump")
	}
}

func TestUIWatchdogReportsActualBlockedPumpAndRecovery(t *testing.T) {
	run := &uiWatchRun{stop: make(chan struct{}), done: make(chan struct{})}
	run.last.Store(1)
	ticks := make(chan time.Time)
	var clock atomic.Int64
	clock.Store(int64(6*time.Second) + 1)
	type event struct {
		recovered bool
		delay     time.Duration
	}
	events := make(chan event, 8)
	go watchUIThreadTicks(run, ticks, clock.Load, func(recovered bool, delay time.Duration) { events <- event{recovered, delay} })
	t.Cleanup(func() { run.stopped.Store(true); close(run.stop); waitUIWatchDone(t, run) })
	read := func(recovered bool, delay time.Duration) {
		t.Helper()
		select {
		case e := <-events:
			if e.recovered != recovered || e.delay != delay {
				t.Fatalf("event %+v", e)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("missing stall/recovery event")
		}
	}
	ticks <- time.Time{}
	read(false, 6*time.Second)
	// Still blocked: one warning, not one per poll. The unbuffered next send
	// also establishes that the preceding iteration finished.
	ticks <- time.Time{}
	run.last.Store(int64(7*time.Second) + 1)
	clock.Store(int64(7*time.Second) + 1)
	ticks <- time.Time{}
	read(true, 7*time.Second)
	clock.Store(int64(13*time.Second) + 1)
	ticks <- time.Time{}
	read(false, 6*time.Second)
	select {
	case e := <-events:
		t.Fatal("duplicate event", e)
	default:
	}
}

func TestUIWatchdogCancellationWinsOverReadyTick(t *testing.T) {
	run := &uiWatchRun{stop: make(chan struct{}), done: make(chan struct{})}
	run.last.Store(1)
	run.stopped.Store(true)
	close(run.stop)
	ticks := make(chan time.Time, 1)
	ticks <- time.Time{}
	watchUIThreadTicks(run, ticks, func() int64 { return int64(20 * time.Second) }, func(bool, time.Duration) { t.Fatal("cancelled watcher reported shutdown as a stall") })
	waitUIWatchDone(t, run)
}

func TestUIWatchdogStopDoesNotWaitOnAnInFlightLogSink(t *testing.T) {
	var w uiPumpWatchdog
	ticks := make(chan time.Time, 1)
	entered, release := make(chan struct{}), make(chan struct{})
	w.watch = func(run *uiWatchRun) {
		watchUIThreadTicks(run, ticks, func() int64 { return int64(20 * time.Second) }, func(bool, time.Duration) { close(entered); <-release })
	}
	stop := w.enter()
	run := w.active.Load()
	run.last.Store(1)
	ticks <- time.Time{}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("sink was not invoked")
	}
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("UI shutdown waited on user logging callback")
	}
	if w.active.Load() != nil {
		t.Fatal("cancelled callback retained pump ownership")
	}
	close(release)
	waitUIWatchDone(t, run)
}

// TestGoroutineDumpIsComplete makes sure the diagnostic actually carries stacks
// — a truncated dump would hide the very frame that is blocking the UI thread.
func TestGoroutineDumpIsComplete(t *testing.T) {
	dump := goroutineDump()
	if !strings.Contains(dump, "goroutine ") {
		t.Fatalf("dump does not look like goroutine stacks: %.120s", dump)
	}
	if !strings.Contains(dump, "gui.TestGoroutineDumpIsComplete") {
		t.Error("dump does not include the running test's own frame")
	}
	if strings.HasSuffix(dump, "... (truncated)") {
		t.Error("dump hit the size cap in a plain test process")
	}
}

// TestUIStallThresholdMatchesWindows keeps the trigger tied to the behaviour it
// explains: Windows paints the "Not Responding" title at about five seconds, so
// warning later than that would report a hang the user already saw.
func TestUIStallThresholdMatchesWindows(t *testing.T) {
	if uiStallThreshold > 5*time.Second {
		t.Errorf("uiStallThreshold = %v, want <= 5s (when Windows flags Not Responding)", uiStallThreshold)
	}
	if uiStallPoll >= uiStallThreshold {
		t.Errorf("uiStallPoll = %v must be well under the threshold %v", uiStallPoll, uiStallThreshold)
	}
}

func TestUIWatchdogNativeWaitIsNestedAndPanicSafe(t *testing.T) {
	var wait nativeUIWait
	beats := 0
	beat := func() { beats++ }
	func() {
		defer wait.begin(beat)()
		if wait.depth.Load() != 1 {
			t.Fatal("outer native wait not active")
		}
		func() {
			defer func() { _ = recover() }()
			defer wait.begin(beat)()
			if wait.depth.Load() != 2 {
				t.Fatal("nested native wait not active")
			}
			panic("native dialog failure")
		}()
		if wait.depth.Load() != 1 {
			t.Fatal("inner panic ended outer scope")
		}
	}()
	if wait.depth.Load() != 0 || beats != 4 {
		t.Fatalf("unbalanced wait: depth=%d, beats=%d", wait.depth.Load(), beats)
	}
}

func TestUIWatchdogIgnoresNativeWaitButDetectsBlockedWork(t *testing.T) {
	now := int64(20 * time.Second)
	last := now - int64(10*time.Second)
	if got := uiWatchdogDelay(now, last, true); got != 0 {
		t.Fatalf("native dialog misclassified as %v stall", got)
	}
	if got := uiWatchdogDelay(now, last, false); got < uiStallThreshold {
		t.Fatalf("real blocked work no longer detected: %v", got)
	}
	if got := uiWatchdogDelay(now, now, false); got != 0 {
		t.Fatalf("resumed event loop has stale deadline: %v", got)
	}
	if got := uiWatchdogDelay(now, now+int64(time.Second), false); got != 0 {
		t.Fatal("negative duration escaped monotonic deadline guard", got)
	}
}
