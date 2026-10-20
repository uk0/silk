package gui

import "sync"

// nativeWakeup joins in-flight native wake calls before backend destruction.
// Removing uiWakeupFn alone is insufficient: Post may have copied the old hook
// under uiTaskMu and call it after that lock has been released. Only wrap a
// native non-reentrant wake primitive here, never arbitrary UI task callbacks.
type nativeWakeup struct {
	mu sync.Mutex
	fn func()
}

func (w *nativeWakeup) wake() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fn != nil {
		w.fn()
	}
}

func (w *nativeWakeup) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fn = nil
}
