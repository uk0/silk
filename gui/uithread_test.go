package gui

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/uk0/silk/core"
)

// armUIThreadDetector makes assertUIThread believe it is running off the UI
// thread and collects what it reports. SILK_DEBUG is read once at process
// start, so the gate has to be forced here.
func armUIThreadDetector(t *testing.T, onUIThread bool) *[]string {
	t.Helper()

	var warnings []string
	unregister := core.RegisterLogSink(func(level core.LogLevel, message string) {
		if level == core.LevelWarn {
			warnings = append(warnings, message)
		}
	})

	oldOwner, oldDebug := uiThreadOwner, uiThreadDebug
	uiThreadOwner = func() bool { return onUIThread }
	uiThreadDebug = func() bool { return true }
	uiThreadReports.Store(0)

	t.Cleanup(func() {
		unregister()
		uiThreadOwner, uiThreadDebug = oldOwner, oldDebug
		uiThreadReports.Store(0)
	})
	return &warnings
}

// TestAssertUIThreadReportsOffThread pins the detector's reason for existing:
// an off-thread mutation is silent — no panic, no error, just corruption seen
// later somewhere else — so the only evidence is the warning it prints here.
func TestAssertUIThreadReportsOffThread(t *testing.T) {
	warnings := armUIThreadDetector(t, false)

	assertUIThread("Widget.Update")

	if len(*warnings) != 1 {
		t.Fatalf("expected one warning, got %d: %v", len(*warnings), *warnings)
	}
	got := (*warnings)[0]
	if !strings.Contains(got, "Widget.Update") {
		t.Errorf("warning does not name the caller: %q", got)
	}
	if !strings.Contains(got, "gui.Post") {
		t.Errorf("warning does not name the fix: %q", got)
	}
	if !strings.Contains(got, "assertUIThread") {
		t.Errorf("warning carries no stack, so it cannot name the offending goroutine: %q", got)
	}
}

// TestAssertUIThreadQuietOnUIThread guards against the detector crying wolf:
// every legitimate mutation goes through this path, so a false positive would
// make the whole log useless.
func TestAssertUIThreadQuietOnUIThread(t *testing.T) {
	warnings := armUIThreadDetector(t, true)

	assertUIThread("Widget.Update")

	if len(*warnings) != 0 {
		t.Fatalf("reported a mutation made ON the UI thread: %v", *warnings)
	}
}

// TestAssertUIThreadOffWithoutDebug pins that the check stays inert in a normal
// build. It runs on Widget.Update, i.e. on every repaint request, so it must
// not reach the backend hook (nor the stack dump) unless SILK_DEBUG armed it.
func TestAssertUIThreadOffWithoutDebug(t *testing.T) {
	warnings := armUIThreadDetector(t, false)

	var owner int32
	uiThreadOwner = func() bool { atomic.AddInt32(&owner, 1); return false }
	uiThreadDebug = func() bool { return false }

	assertUIThread("Widget.Update")

	if n := atomic.LoadInt32(&owner); n != 0 {
		t.Errorf("thread check ran %d time(s) with debug off", n)
	}
	if len(*warnings) != 0 {
		t.Fatalf("reported with debug off: %v", *warnings)
	}
}

// TestAssertUIThreadNoBackendIsSilent covers the headless case: tests and any
// build whose backend installs no hook have no UI thread to be off, and the
// check must not report (or panic on the nil hook).
func TestAssertUIThreadNoBackendIsSilent(t *testing.T) {
	warnings := armUIThreadDetector(t, false)
	uiThreadOwner = nil

	assertUIThread("Widget.Update")

	if len(*warnings) != 0 {
		t.Fatalf("reported without a backend hook: %v", *warnings)
	}
}

// TestAssertUIThreadReportsAreCapped pins the flood guard. A worker that
// mutates in a loop reports on every iteration; without the cap the first
// report — the only one whose stack still points at the original offender —
// scrolls away, and the logging itself becomes the outage.
func TestAssertUIThreadReportsAreCapped(t *testing.T) {
	warnings := armUIThreadDetector(t, false)

	for i := 0; i < uiThreadReportLimit*3; i++ {
		assertUIThread("Widget.Update")
	}

	if len(*warnings) != uiThreadReportLimit {
		t.Fatalf("expected %d reports, got %d", uiThreadReportLimit, len(*warnings))
	}
}

// TestWidgetUpdateIsInstrumented pins the chokepoint itself. The detector is
// worthless if a later edit drops the one call site it hangs off, and nothing
// else in the build would notice.
func TestWidgetUpdateIsInstrumented(t *testing.T) {
	warnings := armUIThreadDetector(t, false)

	label := NewLabel("x")
	label.Update()

	if len(*warnings) == 0 {
		t.Fatal("Widget.Update no longer reports an off-thread mutation")
	}
}
