package gui

// PumpTimersForTest fires every timer that is due, on the calling goroutine,
// without an event loop.
//
// The two backends time differently — GLFW polls timerMap from the frame loop,
// Win32 arms a real WM_TIMER and fires from the message pump — so neither
// mechanism is reachable from a test process, and every test of a debounced
// behaviour ended up calling the work function directly. That leaves the path
// from "timer armed" to "work actually ran" driven by nothing: a Start() with
// the wrong delay, a Stop() in the wrong place or a callback that never re-arms
// is invisible until a user sees a stale view.
//
// This is the seam. It does not advance real time — it declares every armed
// timer due and runs it, which is what a test wants: waiting out a 200ms
// debounce in a unit test buys nothing but 200ms. A test arms the behaviour it
// is exercising, calls this, and asserts on the effect rather than on the
// arming.
func PumpTimersForTest() {
	expireTimersForTest()
	processTimers()
}
