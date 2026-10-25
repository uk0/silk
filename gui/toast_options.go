package gui

// ToastPosition selects an owner-local corner, not a separate native window.
type ToastPosition int

const (
	ToastTopCenter ToastPosition = iota // Legacy ShowToast placement.
	ToastBottomRight
)

// ToastOptions configures an explicit notification. OnActivate runs on the UI
// thread AFTER the toast is dismissed; a close-button click never activates it.
// DurationMs == 0 uses 2500 ms. Worker callers must use Post, as for all widgets.
type ToastOptions struct {
	DurationMs uint32
	Level      ToastLevel
	Position   ToastPosition
	OnActivate func()
}

// Toast is a non-owning handle to a temporary overlay. Dismiss is idempotent,
// including after its timer expires or its owner is hidden/destroyed.
// Like ShowToast, these methods are UI-thread-only.
type Toast struct{ entry *toastEntry }

func (t *Toast) Active() bool { return t != nil && t.entry != nil && t.entry.manager != nil }
func (t *Toast) Dismiss() {
	if t.Active() {
		t.entry.manager.dismiss(t.entry)
	}
}

// ShowToastWithOptions returns nil for a missing/hidden owner. Bottom-right
// toasts stay in the lower half of content, above a Frame's status bar, and hide
// if space is insufficient. At most eight active entries are kept per owner;
// oldest entries are dismissed on overflow. This bounds timers and
// work during bursts without affecting other windows.
func ShowToastWithOptions(parent IWidget, message string, options ToastOptions) *Toast {
	if options.DurationMs == 0 {
		options.DurationMs = 2500
	}
	return GetToastManager().showWithOptions(parent, message, options)
}
