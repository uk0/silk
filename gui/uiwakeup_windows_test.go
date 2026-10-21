//go:build windows

package gui

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/uk0/silk/win32"
)

// TestUIWakeupHookIsInstalled pins that init() actually reached SetUIWakeup.
// Without the hook gui.Post enqueues silently and the task waits for the idle
// timer, which is indistinguishable from "the IDE ignored me".
//
// It reads the source rather than uiWakeupFn: resetUIQueue nils that global in
// a t.Cleanup, and on the GLFW side the nil is load-bearing — restoring the
// real hook would have tests calling glfw.PostEmptyEvent against an
// uninitialised GLFW. So the live value says nothing about what init did once
// any queue test has run, and this assertion has to sit above that.
func TestUIWakeupHookIsInstalled(t *testing.T) {
	raw, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatalf("cannot read the Win32 backend: %v", err)
	}
	body := funcBodyIn(t, string(raw), "func init() {")
	if !strings.Contains(body, "SetUIWakeup(") {
		t.Error("init() does not install a UI wakeup hook; gui.Post waits for the idle timer instead of waking the pump")
	}
}

// TestPostThreadMessageReachesItsQueue pins the win32 wrapper the wakeup rests
// on. A misspelled proc name or a wrong argument order fails silently here —
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
