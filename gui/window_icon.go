package gui

import "github.com/uk0/silk/paint"

// Icon returns the logical source most recently assigned to this native window.
// UI-thread-only. macOS still obtains its native application icon from the .app
// bundle; this value allows owned forms to inherit identity on every backend.
func (win *Window) Icon() paint.Icon { return win.icon }

// Inheritance is a snapshot at creation or explicit Form.SetIcon(nil), not a
// subscription. No polling, icon rasterization or native-handle sharing here.
func initialWindowIcon(widget IWidget, owner *Window) paint.Icon {
	usable := func(icon paint.Icon) bool { return icon != nil && !icon.IsAir() }
	if source, ok := widget.(IIcon); ok {
		if icon := source.Icon(); usable(icon) {
			return icon
		}
	}
	if owner != nil && usable(owner.Icon()) {
		return owner.Icon()
	}
	// Do not turn an unconfigured standalone form into an image-missing icon.
	// Frame creation retains its existing executable-name lookup behavior.
	if icon := appIcon; usable(icon) {
		return icon
	}
	return nil
}
