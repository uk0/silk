package gui

// CurrentFocusWidget returns the current logical input owner, or nil. UI thread
// only. Reading it does not move focus, activate a window, or send an event.
// A temporarily disabled target stays remembered; HasFocus/IsEnabled distinguish
// active input eligibility from that identity without losing focus on recovery.
// Async hosts can recheck ownership after a reentrant page replacement instead
// of stealing focus from a field selected while the replacement was committed.
func CurrentFocusWidget() IWidget { return focusWidget }
