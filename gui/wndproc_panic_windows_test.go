package gui

import (
	"testing"
	"time"

	"github.com/uk0/silk/win32"
)

// resetWndPanicState puts the reporter back to a known state between cases.
func resetWndPanicState() {
	wndPanicVerbose = 0
	wndPanicSuppressed = 0
	wndPanicLastReport = time.Time{}
}

// TestWndProcPanicReportIsRateLimited pins the reason this exists: a panic in a
// mouse handler recurs on EVERY WM_MOUSEMOVE, and the original code wrote a
// warning to the log file each time. Hundreds of file writes a second on the UI
// thread is itself a freeze, so only the first few may be verbose.
func TestWndProcPanicReportIsRateLimited(t *testing.T) {
	resetWndPanicState()
	defer resetWndPanicState()

	for i := 0; i < wndPanicVerboseLimit; i++ {
		reportWndProcPanic(win32.HWND(1), win32.WM_MOUSEMOVE, 0, 0, "boom")
	}
	if wndPanicVerbose != wndPanicVerboseLimit {
		t.Fatalf("verbose count = %d, want %d", wndPanicVerbose, wndPanicVerboseLimit)
	}
	if wndPanicSuppressed != 0 {
		t.Fatalf("nothing should be suppressed yet, got %d", wndPanicSuppressed)
	}

	// Everything past the limit is counted, not logged. The first call after
	// the limit also emits the one-per-second summary (last report is zero
	// time), so drive several and check they accumulate rather than log.
	for i := 0; i < 500; i++ {
		reportWndProcPanic(win32.HWND(1), win32.WM_MOUSEMOVE, 0, 0, "boom")
	}
	if wndPanicVerbose != wndPanicVerboseLimit {
		t.Errorf("verbose count grew past the limit: %d", wndPanicVerbose)
	}
	if wndPanicSuppressed == 0 {
		t.Error("suppressed count never advanced; repeats are still being logged in full")
	}
}

// TestWndProcPanicSummaryResetsCounter checks the summary path clears its
// backlog, so the count reported is "since the last summary" rather than a
// number that only ever grows.
func TestWndProcPanicSummaryResetsCounter(t *testing.T) {
	resetWndPanicState()
	defer resetWndPanicState()

	wndPanicVerbose = wndPanicVerboseLimit // straight to the quiet path
	wndPanicLastReport = time.Now().Add(-2 * time.Second)

	reportWndProcPanic(win32.HWND(1), win32.WM_MOUSEMOVE, 0, 0, "boom")
	if wndPanicSuppressed != 0 {
		t.Errorf("summary should have flushed the counter, got %d", wndPanicSuppressed)
	}
	if time.Since(wndPanicLastReport) > time.Second {
		t.Error("summary did not record its timestamp")
	}
}
