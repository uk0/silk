package gui

// CurrentFocusWidget returns the current logical input owner, or nil. UI thread
// only. Reading it does not move focus, activate a window, or send an event.
// Async hosts can recheck ownership after a reentrant page replacement instead
// of stealing focus from a field selected while the replacement was committed.
func CurrentFocusWidget() IWidget { return focusWidget }
