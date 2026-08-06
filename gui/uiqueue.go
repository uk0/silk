package gui

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/uk0/silk/core"
)

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
		runUITask(fn)
	}
}

// runUITask keeps one panicking task from killing the event loop, and names it.
// The recover used to swallow the panic whole: a posted task — a debugger step,
// a git refresh, a toast — simply did nothing, with nothing in the log to say
// it had even run, and the defect behind it could not be found. The stack is
// the report, since the panic came off whichever goroutine called Post.
func runUITask(fn func()) {
	defer func() {
		if e := recover(); e != nil {
			core.Warn(fmt.Sprintf("ui: posted task panicked: %v\n%s", e, debug.Stack()))
		}
	}()
	fn()
}

// SetUIWakeup installs the wakeup hook (called once by the window layer).
func SetUIWakeup(fn func()) { uiTaskMu.Lock(); uiWakeupFn = fn; uiTaskMu.Unlock() }
