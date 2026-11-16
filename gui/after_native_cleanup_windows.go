//go:build windows

package gui

// PostAfterNativeCleanup is UI-thread-only. Win32 destroys the closed modal
// synchronously; its continuation still runs in a later posted UI batch.
// Callers must independently recheck ownership, edits and focus.
func PostAfterNativeCleanup(fn func()) { Post(fn) }
