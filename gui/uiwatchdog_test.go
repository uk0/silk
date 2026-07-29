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
