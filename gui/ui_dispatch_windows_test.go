package gui

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/win32"
)

// Native-modal-style pump: intentionally NO handleNonUIMessages, serviceUIFrame
// or processTimers. Only DispatchMessage may deliver work, like OLE's loop.
func pumpNativeDispatchUntil(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !done() && time.Now().Before(deadline) {
		var msg win32.MSG
		if win32.PeekMessage(&msg, 0, 0, 0, win32.PM_REMOVE) {
			win32.TranslateMessage(&msg)
			win32.DispatchMessage(&msg)
		} else {
			time.Sleep(time.Millisecond)
		}
	}
	if !done() {
		t.Fatal("native dispatch did not make progress")
	}
}

func TestNativeDispatchTimerAndNestedLoop(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner := win32.GetCurrentThreadId()
	var outer, inner Timer
	defer outer.Stop()
	defer inner.Stop()
	calls, innerCalls, depth, maxDepth := 0, 0, 0, 0
	if !outer.Start(10, func() {
		if win32.GetCurrentThreadId() != owner {
			t.Error("timer left owner thread")
		}
		calls++
		depth++
		maxDepth = max(maxDepth, depth)
		defer func() { depth-- }()
		if calls == 1 {
			if !inner.Start(10, func() { innerCalls++ }) {
				t.Error("inner timer failed")
				return
			}
			pumpNativeDispatchUntil(t, func() bool { return innerCalls >= 3 })
			inner.Stop()
		}
	}) {
		t.Fatal("timer failed")
	}
	pumpNativeDispatchUntil(t, func() bool { return calls >= 2 })
	if maxDepth != 1 {
		t.Fatalf("recursive timer depth %d", maxDepth)
	}
	id := uintptr(outer)
	outer.Stop()
	before := calls
	win32.DispatchMessage(&win32.MSG{Message: win32.WM_TIMER, WParam: id, LParam: uiTimerCallback})
	if calls != before {
		t.Fatal("stopped timer fired from stale message")
	}
}

func TestNativeDispatchPostsWithoutIdleTimerAndJoinsShutdown(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := newUIMessageWindow()
	defer d.close()
	uiTaskMu.Lock()
	oldTasks, oldWake := uiTasks, uiWakeupFn
	uiTasks = nil
	uiTaskMu.Unlock()
	defer func() { uiTaskMu.Lock(); uiTasks, uiWakeupFn = oldTasks, oldWake; uiTaskMu.Unlock() }()
	SetUIWakeup(d.wake)
	owner := win32.GetCurrentThreadId()
	calls := 0
	posted := make(chan struct{})
	go func() {
		for i := 0; i < uiTaskBatchLimit*3; i++ {
			Post(func() {
				if win32.GetCurrentThreadId() != owner {
					t.Error("post left owner thread")
				}
				calls++
			})
		}
		close(posted)
	}()
	<-posted
	pumpNativeDispatchUntil(t, func() bool { return calls == uiTaskBatchLimit*3 })
	// No timer on this test's thread is a fallback. A copied wake closure may
	// run concurrently with shutdown, and must not target a recycled HWND.
	wake := d.wake
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				wake()
			}
		}()
	}
	d.close()
	wg.Wait()
	wake()
	d.close()
	if d.hwnd != 0 {
		t.Fatal("dispatch window survived shutdown")
	}
}
