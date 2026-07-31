//go:build windows

package gui

import (
	"runtime"
	"testing"

	"github.com/uk0/silk/win32"
)

// TestInitRecordedARealThreadId pins the half of the fix that is easy to undo
// by accident. The backend used to store win32.GetCurrentThread(), which is a
// pseudo-handle: the same constant on every thread, so comparing it tells you
// nothing and the "wrong thread" guard in MainLoop would never fire.
func TestInitRecordedARealThreadId(t *testing.T) {
	if uiThreadId == 0 {
		t.Fatal("init did not record the UI thread id")
	}
	if uiThreadId == uint32(win32.GetCurrentThread()) {
		t.Error("uiThreadId holds the GetCurrentThread pseudo-handle, not a thread id; it cannot identify a thread")
	}
}

// TestGetCurrentThreadIdDistinguishesThreads is the property the guard rests
// on: two different OS threads must report two different ids.
func TestGetCurrentThreadIdDistinguishesThreads(t *testing.T) {
	// Both goroutines stay locked and alive at once. A locked goroutine owns
	// its thread even while parked, so the two cannot land on the same one and
	// the comparison below is not a race with the scheduler.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	self := win32.GetCurrentThreadId()
	if self == 0 {
		t.Fatal("GetCurrentThreadId returned 0")
	}

	ids := make(chan uint32)
	release := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		ids <- win32.GetCurrentThreadId()
		<-release
	}()
	other := <-ids
	close(release)

	if other == 0 {
		t.Fatal("GetCurrentThreadId returned 0 on the second thread")
	}
	if other == self {
		t.Error("both threads report the same id; the call is not reading real thread identity")
	}
}
