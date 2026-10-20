package gui

import (
	"strings"
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

// TestUIWatchdogStartsOnce guards against one watcher goroutine per message —
// uiHeartbeat runs on every dispatched message on Windows.
func TestUIWatchdogStartsOnce(t *testing.T) {
	uiHeartbeat()
	if !uiWatchOnce.Load() {
		t.Fatal("watchdog was not armed by the first heartbeat")
	}
	for i := 0; i < 100; i++ {
		uiHeartbeat()
	}
	if !uiWatchOnce.Load() {
		t.Error("watchdog flag flipped back; a second goroutine could start")
	}
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
	now := time.Now()
	last := now.Add(-10 * time.Second).UnixNano()
	if got := uiWatchdogDelay(now, last, true); got != 0 {
		t.Fatalf("native dialog misclassified as %v stall", got)
	}
	if got := uiWatchdogDelay(now, last, false); got < uiStallThreshold {
		t.Fatalf("real blocked work no longer detected: %v", got)
	}
	if got := uiWatchdogDelay(now, now.UnixNano(), false); got != 0 {
		t.Fatalf("resumed event loop has stale deadline: %v", got)
	}
}
