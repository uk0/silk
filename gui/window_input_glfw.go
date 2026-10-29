//go:build !windows

package gui

import "github.com/go-gl/glfw/v3.3/glfw"

// Keep input in event coordinates, not the independently polled OS pointer.
// Accessibility tools and remote desktops can deliver valid window events
// without moving that pointer. Polling it made clicks miss their hovered widget.
var pointerWindow *Window // most recent pointer-event source, UI thread only

// Platform bridge to the actual button event, if available. The injectable
// seam keeps regression tests on real window dispatch without native windows.
var mouseButtonEventPosition = nativeMouseButtonEventPosition
var scrollEventPosition = nativeScrollEventPosition

type pointerEventState struct {
	x, y  float64
	valid bool
}

func (p pointerEventState) position(fallback func() (float64, float64)) (float64, float64) {
	if p.valid {
		return p.x, p.y
	}
	return fallback()
}

// UI-thread-only, scoped to the current key/button callback. Save/restore at
// entry permits nested modal event loops without leaking modifier state.
var eventModifiers *glfw.ModifierKey

func modifierKeyState(key int, mods glfw.ModifierKey) (held, modifier bool) {
	var flag glfw.ModifierKey
	switch key {
	case KeyCtrl:
		flag = glfw.ModControl
	case KeyShift:
		flag = glfw.ModShift
	case KeyMenu:
		flag = glfw.ModAlt
	case KeyLWin, KeyRWin:
		flag = glfw.ModSuper
	default:
		return false, false
	}
	return mods&flag != 0, true
}

// Only the outer MainLoop drains this queue: modal loops can themselves be
// nested inside a GLFW callback that still refers to a closing native window.
var pendingNativeDestruction []func()

// A nested modal can keep the outer event dispatch alive for minutes. Native
// destruction must wait, but presentation must not: an already-detached window
// left visible becomes an inert, floating window that still steals OS focus.
// Hiding is safe during dispatch (as for ordinary popups); freeing is not.
func retireNativeWindow(hide, destroy func()) {
	duringTransientTeardown(hide)
	pendingNativeDestruction = append(pendingNativeDestruction, func() {
		duringTransientTeardown(destroy)
	})
}

func flushNativeDestruction() {
	pending := pendingNativeDestruction
	pendingNativeDestruction = nil
	for _, destroy := range pending {
		destroy()
	}
}

func hasOpenFormWindow() bool {
	for _, win := range winMap {
		if win != nil && win.wt == WtForm {
			return true
		}
	}
	return false
}
