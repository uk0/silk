package main

import (
	"testing"

	"github.com/uk0/silk/gui"
)

// onUI must hand its func to the gui event-loop queue rather than run it
// inline — that is the whole point of UI-thread marshaling. These are
// GL-free assertions: no window, no Draw, no real event loop.
//
// The gui package's window layer installs glfw.PostEmptyEvent as the UI
// wakeup hook in its init(), so a bare gui.Post would nudge GLFW and
// SIGTRAP in this headless test binary. Nil the wakeup first so Post stays
// headless — the same "uiWakeupFn nil'd so Post stays headless" convention
// gui's own tag/alarm tests use.
//
// Limitation: gui's drain seam (drainUITasks / resetUIQueue) is unexported,
// so from package main we cannot run the queued closure on the "UI thread"
// and observe its effect — gui's own uiqueue_test.go covers that drain
// path. Here we assert the two properties reachable from main: onUI never
// panics (including on a nil func, which gui.Post tolerates) and onUI does
// NOT run its func synchronously (proving it was enqueued, not run inline).

func TestOnUIDoesNotPanic(t *testing.T) {
	gui.SetUIWakeup(nil) // headless: Post must not nudge GLFW (see gui/uiqueue.go)
	onUI(func() {})
	onUI(nil)
}

func TestOnUIDefersInsteadOfRunningInline(t *testing.T) {
	gui.SetUIWakeup(nil) // headless: Post must not nudge GLFW (see gui/uiqueue.go)
	ran := false
	onUI(func() { ran = true })
	// gui.Post only enqueues; nothing drains the queue in this headless
	// test, so the closure must NOT have run on this goroutine. If onUI
	// ever regressed to calling fn() directly, ran would be true here.
	if ran {
		t.Fatal("onUI ran its func synchronously; it must marshal onto the UI thread via gui.Post")
	}
}
