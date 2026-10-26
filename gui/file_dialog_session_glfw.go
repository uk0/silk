//go:build !windows

package gui

type nativeFileDialogKind int

const (
	nativeFileDialogOpen   nativeFileDialogKind = iota
	nativeFileDialogFolder                      // native support only; no new public folder API yet
	nativeFileDialogSave
)

// The Cocoa completion block keeps its own native state; Go only polls it
// while servicing the existing UI loop. This preserves the synchronous SDK
// API without an external process or calling Go from an AppKit callback.
type nativeFileDialogSession interface {
	result() (path string, done bool)
	cancel()
	close()
}

func waitNativeFileDialog(owner *Window, session nativeFileDialogSession, ownerAlive func() bool) string {
	wasEnabled := owner.IsEnabled()
	owner.SetEnabled(false)
	// Native teardown may restore focus: re-enable the owner first.
	defer session.close()
	defer owner.SetEnabled(wasEnabled)
	for {
		if shouldQuit || !ownerAlive() {
			session.cancel()
			return ""
		}
		if path, done := session.result(); done {
			return path // Cancel is an empty path, not an error.
		}
		nativeDialogFrame()
	}
}

// The native half additionally checks completion, app activation, visibility,
// and foreign key windows/sheets. Never resurrect a detached or disabled owner.
func fileDialogOwnerCanRestoreFocus(owner *Window) bool {
	return !shouldQuit && owner != nil && owner.widget != nil &&
		owner.widget.Window() == owner && owner.enabled && !owner.iconified && owner.widget.IsVisible()
}
