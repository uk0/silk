//go:build !windows

package gui

import (
	"os"
	"strings"
	"testing"
)

// TestServiceUIFrameDrainsPostedTasksAndBeats covers what both GLFW loops owe
// the rest of the framework once per iteration, and what ShowModal's loop used
// to skip: it ran timers and animations but neither drained the UI task queue
// nor fed the watchdog. A task posted from a background goroutine — a dlv step
// callback, an LSP result, a git refresh, a toast — was therefore queued with
// nothing to run it for as long as the dialog stayed open, and the watchdog,
// beating only in MainLoop, warned that the event loop had stopped for the
// whole of a perfectly healthy modal.
//
// The service work is asserted here rather than through ShowModal itself: the
// modal loop blocks in glfw.WaitEventsTimeout, an AppKit call that may only run
// on the thread init() pinned, and a test function never runs on it. What the
// loop actually calls is checked separately by
// TestModalLoopServicesTheUIFrame — between them, deleting either call from
// serviceUIFrame or the serviceUIFrame call from the modal loop turns a test
// red.
func TestServiceUIFrameDrainsPostedTasksAndBeats(t *testing.T) {
	// This checks one frame's service wiring, not an unlimited drain. Isolate
	// older fixtures' pending layout work: production intentionally yields
	// after 64 tasks or 4ms so input/rendering cannot be starved by a backlog.
	uiTaskMu.Lock()
	savedTasks := uiTasks
	uiTasks = nil
	uiTaskMu.Unlock()
	defer func() {
		uiTaskMu.Lock()
		uiTasks = append(savedTasks, uiTasks...)
		uiTaskMu.Unlock()
	}()
	// init() arms the 47ms idle timer, and processTimers is part of the frame;
	// swap the map out so this test fires no callback of anyone else's.
	saved := timerMap
	timerMap = make(map[uintptr]*timerEntry)
	defer func() { timerMap = saved }()

	// Post the way every real caller does — off the UI thread — and join, so
	// the task is queued before the frame runs.
	ran := false
	queued := make(chan struct{})
	go func() {
		Post(func() { ran = true })
		close(queued)
	}()
	<-queued
	// Leave no task behind for the rest of the package if the drain is gone.
	defer drainUITasks()

	before := uiLastBeat.Load()

	serviceUIFrame()

	if !ran {
		t.Error("a task posted while the loop was running never ran: the frame does not drain the UI task queue, so everything gui.Post queues from a background goroutine stalls until the loop that does drain it takes over")
	}
	if got := uiLastBeat.Load(); got == before {
		t.Errorf("uiLastBeat still %d after a serviced frame: the frame does not call uiHeartbeat, so the watchdog reports \"event loop has not run for ...\" while the loop is in fact turning", got)
	}
}

// TestModalLoopServicesTheUIFrame pins the wiring the test above cannot reach.
// A modal dialog runs its own loop, and the failure being guarded is that loop
// silently servicing less than MainLoop does — which is invisible to a compiler
// and to every widget test, and stays invisible until someone opens a dialog
// and watches the application stop answering its own background work.
func TestModalLoopServicesTheUIFrame(t *testing.T) {
	src, err := os.ReadFile("window_glfw.go")
	if err != nil {
		t.Fatalf("cannot read the GLFW backend: %v", err)
	}
	body := modalLoopBody(string(src))
	if body == "" {
		t.Fatal("window_glfw.go has no ShowModal loop to check")
	}
	if !strings.Contains(body, "serviceUIFrame()") {
		t.Error("the modal loop does not call serviceUIFrame(); posted tasks stall and the watchdog reports a false stall for as long as a modal dialog is open")
	}
}

// modalLoopBody returns the text of the `for this.inModal` loop in ShowModal,
// or "" when there is none.
func modalLoopBody(src string) string {
	fn := strings.Index(src, "\nfunc (this *Window) ShowModal(")
	if fn < 0 {
		return ""
	}
	rest := src[fn:]
	start := strings.Index(rest, "\n\tfor this.inModal")
	if start < 0 {
		return ""
	}
	rest = rest[start:]
	if end := strings.Index(rest, "\n\t}\n"); end >= 0 {
		return rest[:end]
	}
	return rest
}
