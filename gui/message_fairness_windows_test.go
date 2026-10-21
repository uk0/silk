package gui

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/uk0/silk/win32"
)

func TestWindowsMessageFairnessIncludesEveryClassAndNormalMessages(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	oldPeek, oldGet, oldTurn := peekUIMessage, getUIMessage, windowsMessageTurn
	defer func() { peekUIMessage, getUIMessage, windowsMessageTurn = oldPeek, oldGet, oldTurn }()
	windowsMessageTurn = 0
	Post(func() {})
	var classes []uint32
	peekUIMessage = func(m *win32.MSG, h win32.HWND, a, b, f uint32) bool {
		kind := f >> 16
		if a == win32.WM_PAINT && b == a {
			kind = 0x20
		}
		if a == win32.WM_TIMER && b == a {
			kind = 0x10
		}
		classes = append(classes, kind)
		m.Message = win32.WM_NULL
		return true
	}
	gets := 0
	getUIMessage = func(m *win32.MSG, h win32.HWND, a, b uint32) (bool, error) { gets++; return true, nil }
	var msg win32.MSG
	for range 8 {
		if ok, err := nextWindowsUIMessage(&msg); !ok || err != nil {
			t.Fatal(ok, err)
		}
	}
	if gets != 2 || len(classes) != 6 {
		t.Fatalf("fair turns gets=%d classes=%v", gets, classes)
	}
	for i, v := range classes {
		if v != ([3]uint32{0x407, 0x20, 0x10})[i%3] {
			t.Fatalf("class order %v", classes)
		}
	}
	resetUIQueue()
	nextWindowsUIMessage(&msg)
	if gets != 3 || len(classes) != 6 {
		t.Fatal("idle loop polled instead of blocking")
	}
}

func TestWindowsMessageFairnessPreservesQuit(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	oldPeek, oldTurn := peekUIMessage, windowsMessageTurn
	defer func() { peekUIMessage, windowsMessageTurn = oldPeek, oldTurn }()
	windowsMessageTurn = 0
	Post(func() {})
	peekUIMessage = func(m *win32.MSG, h win32.HWND, a, b, f uint32) bool {
		m.Message = win32.WM_QUIT
		m.WParam = 7
		return true
	}
	var m win32.MSG
	if ok, err := nextWindowsUIMessage(&m); ok || err != nil || m.WParam != 7 {
		t.Fatal("WM_QUIT was swallowed or dispatched")
	}
}

func TestWindowsMessageFairnessNativeTimerProgressDuringPostBurst(t *testing.T) {
	// A Go test goroutine can reuse an OS thread whose native queue still has
	// window/caret timers from earlier tests. Such a WM_TIMER is legitimate
	// progress, but cannot satisfy this test's exact NULL-HWND timer identity.
	// Use a fresh process, not a weaker "any timer" assertion or queue draining
	// that could consume another fixture's messages.
	if os.Getenv("SILK_TEST_FAIRNESS_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWindowsMessageFairnessNativeTimerProgressDuringPostBurst$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "SILK_TEST_FAIRNESS_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated native timer probe: %v (context=%v)\n%s", err, ctx.Err(), out)
		}
		t.Logf("isolated native queue:\n%s", out)
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	old := windowsMessageTurn
	defer func() { windowsMessageTurn = old }()
	var msg win32.MSG
	win32.PeekMessage(&msg, 0, 0, 0, win32.PM_NOREMOVE)
	id := win32.SetTimer(10)
	if id == 0 {
		t.Fatal("SetTimer failed")
	}
	defer win32.KillTimer(id)
	// Queue a real burst before the timer becomes due. First prove the
	// unfiltered pump chooses a wake; then the production fair selector must
	// retrieve our exact timer ahead of the remaining posted messages.
	thread := win32.GetCurrentThreadId()
	for i := 0; i < 64; i++ {
		if !win32.PostThreadMessage(thread, WM_UI_WAKEUP, uintptr(i), 0) {
			t.Fatal("PostThreadMessage failed")
		}
	}
	defer func() {
		for win32.PeekMessage(&msg, 0, WM_UI_WAKEUP, WM_UI_WAKEUP, win32.PM_REMOVE) {
		}
	}()
	Post(func() {})
	time.Sleep(35 * time.Millisecond)
	if ok, err := win32.GetMessageResult(&msg, 0, 0, 0); !ok || err != nil || msg.Message != WM_UI_WAKEUP || msg.WParam != 0 {
		t.Fatalf("unfiltered control did not select the first posted wake: ok=%v err=%v msg=%+v", ok, err, msg)
	}
	windowsMessageTurn = 2 // timer's reserved turn
	if ok, err := nextWindowsUIMessage(&msg); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if msg.Message != win32.WM_TIMER || msg.Hwnd != 0 || msg.WParam != id {
		t.Fatalf("posted wake starved native timer: %+v", msg)
	}
	if !win32.PeekMessage(&msg, 0, WM_UI_WAKEUP, WM_UI_WAKEUP, win32.PM_REMOVE) || msg.WParam != 1 {
		t.Fatalf("timer selection consumed or reordered the remaining wake burst: %+v", msg)
	}
	t.Log("exact native timer progressed ahead of 63 pending wakes; unfiltered control chose a wake")
}
