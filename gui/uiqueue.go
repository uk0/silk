package gui

import "sync"

var (
	uiTaskMu   sync.Mutex
	uiTasks    []func()
	uiWakeupFn func() // set by the window layer to glfw.PostEmptyEvent; nil in headless tests
)

// Post enqueues fn to run on the main (event-loop) thread on the next
// iteration. Safe to call from any goroutine. Use this for every GUI
// mutation that originates off the main thread (dlv/LSP callbacks, etc).
func Post(fn func()) {
	if fn == nil {
		return
	}
	uiTaskMu.Lock()
	uiTasks = append(uiTasks, fn)
	wake := uiWakeupFn
	uiTaskMu.Unlock()
	if wake != nil {
		wake() // nudge a blocked WaitEvents so the task runs promptly
	}
}

// drainUITasks runs and clears all queued tasks. MUST be called only from
// the main thread (the event loop). Snapshots under the lock then runs
// unlocked so a task that calls Post (re-entrancy) doesn't deadlock.
func drainUITasks() {
	uiTaskMu.Lock()
	if len(uiTasks) == 0 {
		uiTaskMu.Unlock()
		return
	}
	batch := uiTasks
	uiTasks = nil
	uiTaskMu.Unlock()
	for _, fn := range batch {
		func() { defer func() { _ = recover() }(); fn() }() // a panicking task must not kill the loop
	}
}

// SetUIWakeup installs the wakeup hook (called once by the window layer).
func SetUIWakeup(fn func()) { uiTaskMu.Lock(); uiWakeupFn = fn; uiTaskMu.Unlock() }
