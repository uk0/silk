//go:build windows

package gui

import (
	"os"
	"runtime"
	"testing"

	"github.com/uk0/silk/win32"
)

// Verify the actual object created by package init, not os.ReadFile of source:
// runtime file reads bypass go -overlay and can inspect a different program.
// Other queue tests replace the wake hook, but must not retire this owner HWND.
func TestUIWakeupHookIsInstalled(t *testing.T) {
	if uiDispatcher == nil {
		t.Fatal("init did not create a native UI dispatcher")
	}
	uiDispatcher.mu.Lock()
	hwnd := uiDispatcher.hwnd
	uiDispatcher.mu.Unlock()
	if hwnd == 0 || !win32.IsWindow(hwnd) {
		t.Fatal("UI dispatcher has no live HWND")
	}
	thread, process := win32.GetWindowThreadProcessId(hwnd)
	if uint32(thread) != uiThreadId || process != os.Getpid() {
		t.Fatalf("dispatcher owner = %d/%d, want %d/%d", thread, process, uiThreadId, os.Getpid())
	}
}

// TestPostThreadMessageReachesItsQueue pins the low-level thread-message
// wrapper (not the native-modal-safe UI wakeup). Wrong arguments fail here —
// PostThreadMessage just returns false — and the only symptom would be a UI
// that updates 47ms late, which reads as ordinary sluggishness rather than a
// broken call.
func TestPostThreadMessageReachesItsQueue(t *testing.T) {
	testPostThreadMessageReachesItsQueue(t, false)
}

func TestPostThreadMessageKeepsUnrelatedQueuedMessages(t *testing.T) {
	testPostThreadMessageReachesItsQueue(t, true)
}

func testPostThreadMessageReachesItsQueue(t *testing.T, withNoise bool) {
	t.Helper()
	// The post must target the thread that will read it, so pin this goroutine
	// for the duration of the round trip.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// A thread has no message queue until a user32 call creates one, and
	// PostThreadMessage fails against a thread without one.
	var msg win32.MSG
	win32.PeekMessage(&msg, 0, 0, 0, win32.PM_NOREMOVE)
	// Windows queues belong to OS threads, not Go tests. Other tests can leave
	// timers queued on a reused thread. Select only the message under test;
	// consuming the queue's arbitrary first message tests ordering, not delivery.
	const noise = WM_UI_WAKEUP + 1
	for win32.PeekMessage(&msg, 0, WM_UI_WAKEUP, WM_UI_WAKEUP, win32.PM_REMOVE) {
	}
	defer func() {
		win32.PeekMessage(&msg, 0, WM_UI_WAKEUP, WM_UI_WAKEUP, win32.PM_REMOVE)
		if withNoise {
			win32.PeekMessage(&msg, 0, noise, noise, win32.PM_REMOVE)
		}
	}()
	if withNoise && !win32.PostThreadMessage(win32.GetCurrentThreadId(), noise, 0, 0) {
		t.Fatal("cannot queue unrelated message")
	}

	const wantWParam, wantLParam = 0x51a5, 0x71c3
	if !win32.PostThreadMessage(win32.GetCurrentThreadId(), WM_UI_WAKEUP, wantWParam, wantLParam) {
		t.Fatal("PostThreadMessage failed against this thread's own queue")
	}
	if !win32.PeekMessage(&msg, 0, WM_UI_WAKEUP, WM_UI_WAKEUP, win32.PM_REMOVE) {
		t.Fatal("the posted message never landed in the queue; GetMessage would keep blocking")
	}
	if msg.Message != WM_UI_WAKEUP {
		t.Fatalf("read back message %#x, want WM_UI_WAKEUP (%#x)", msg.Message, uint32(WM_UI_WAKEUP))
	}
	// MainLoop routes on this: a NULL Hwnd goes to handleNonUIMessages instead
	// of TranslateMessage/DispatchMessage, which is what makes the wakeup a
	// no-op rather than something a window procedure has to understand.
	if msg.Hwnd != 0 {
		t.Errorf("thread message arrived with Hwnd %#x, want 0", msg.Hwnd)
	}
	if msg.WParam != wantWParam || msg.LParam != wantLParam {
		t.Errorf("payload = %#x/%#x, want %#x/%#x", msg.WParam, msg.LParam, wantWParam, wantLParam)
	}
	if withNoise && !win32.PeekMessage(&msg, 0, noise, noise, win32.PM_REMOVE) {
		t.Error("wakeup retrieval consumed the unrelated queued message")
	}
}
