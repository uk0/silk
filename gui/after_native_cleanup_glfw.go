//go:build !windows

package gui

// PostAfterNativeCleanup is a UI-thread-only scheduling boundary for work
// following a closed native modal/popup. Unlike Post, it cannot run inside a
// nested loop before deferred destruction and queued owner-focus-return attempts. The
// callback is then posted, not invoked from native cleanup itself. Hosts must
// still recheck their ownership, edits and focus; this is not an input lock.
// It uses the existing outer loop, with no timer or waiting thread.
func PostAfterNativeCleanup(fn func()) {
	if fn == nil {
		return
	}
	pendingNativeDestruction = append(pendingNativeDestruction, func() { Post(fn) })
	Post(func() {}) // wake the outer loop even if no more native events arrive
}
