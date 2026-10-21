//go:build !windows

package gui

import (
	"fmt"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/core"
)

// A transient close may make Cocoa select a different Form. Only focus
// changes caused synchronously by our own Hide/Destroy are ignored; a later
// native focus transfer or newly shown window cancels the pending return.
var transientFocusGeneration uint64
var transientTeardownDepth int

func duringTransientTeardown(fn func()) {
	transientTeardownDepth++
	defer func() { transientTeardownDepth-- }()
	fn()
}

func noteNativeFocusTransfer() {
	if transientTeardownDepth == 0 {
		transientFocusGeneration++
	}
}

type transientFocusReturn struct {
	source     *Window
	owner      *Window
	target     IWidget
	generation uint64
}

func nativeWindowFocused(win *Window) bool {
	return win != nil && win.glfwWin != nil && win.IsVisible() && win.glfwWin.GetAttrib(glfw.Focused) == glfw.True
}

func focusReturnWindowTag(win *Window) string {
	if win == nil {
		return "nil"
	}
	// Types and transient addresses distinguish two Frame windows without
	// logging their titles, document paths, task names or entered content.
	return fmt.Sprintf("%p/%d/%T", win, win.wt, win.widget)
}

func debugFocusReturn(stage, reason string, source, owner *Window, generation uint64, focused func(*Window) bool) {
	if !core.IsDebugOn() {
		return
	}
	var focusOwner *Window
	if focusWidget != nil {
		focusOwner = focusWidget.OwnerWindow()
	}
	core.Debug("focus-return ", stage, ": reason=", reason,
		", generation=", generation, "/", transientFocusGeneration,
		", source=", focusReturnWindowTag(source), ", owner=", focusReturnWindowTag(owner),
		", native-focused=", focused(source), "/", focused(owner),
		", logical=", fmt.Sprintf("%T", focusWidget), ", logical-owner=", focusReturnWindowTag(focusOwner))
}

func prepareTransientFocusReturn(source, owner *Window, target IWidget, focused func(*Window) bool) *transientFocusReturn {
	if owner == nil || owner == source || owner.widget == nil {
		debugFocusReturn("prepare", "invalid-owner", source, owner, transientFocusGeneration, focused)
		return nil
	}
	// Closing a transient from a timer while the user is in another app must
	// not reactivate this app. Non-activating menus keep native focus on owner.
	if !focused(source) && !focused(owner) {
		debugFocusReturn("prepare", "native-family-unfocused", source, owner, transientFocusGeneration, focused)
		return nil
	}
	debugFocusReturn("prepare", "accepted", source, owner, transientFocusGeneration, focused)
	return &transientFocusReturn{source, owner, focusTargetForOwner(owner, target), transientFocusGeneration}
}

func (r *transientFocusReturn) queue() {
	if r == nil {
		return
	}
	debugFocusReturn("queue", "after-native-destruction", r.source, r.owner, r.generation, nativeWindowFocused)
	// Share the existing outer-loop queue, AFTER Close has queued native
	// destruction. Post() can run in nested modal loops and is too early.
	pendingNativeDestruction = append(pendingNativeDestruction, func() {
		r.restore(func(w *Window) bool {
			return w.glfwWin != nil && w.IsVisible()
		}, nativeWindowFocused, nativeApplicationActive, func(w *Window) { w.glfwWin.Focus() })
	})
}

// The injected native operations keep this policy testable without a desktop.
func (r *transientFocusReturn) restore(visible, focused func(*Window) bool, applicationActive func(bool) bool, activate func(*Window)) {
	owner := r.owner
	reason := ""
	switch {
	case shouldQuit:
		reason = "quit"
	case r.generation != transientFocusGeneration:
		reason = "generation-changed"
	case owner.widget == nil || owner.widget.Window() != owner:
		reason = "owner-detached"
	case !owner.enabled:
		reason = "owner-disabled"
	case owner.iconified:
		reason = "owner-minimized"
	case !visible(owner):
		reason = "owner-hidden"
	}
	if reason != "" {
		debugFocusReturn("reject", reason, r.source, owner, r.generation, focused)
		return
	}
	// Another modal/popup or a deliberately focused widget wins over a stale
	// return, even when it was opened by the closing menu's callback.
	if focusWidget != nil && focusWidget.IsAllAncentorsVisible() && focusWidget.OwnerWindow() != owner {
		debugFocusReturn("reject", "foreign-logical-focus", r.source, owner, r.generation, focused)
		return
	}
	appFocused := focused(owner)
	for _, win := range winMap {
		if win != owner && win.widget != nil && win.inModal && visible(win) {
			debugFocusReturn("reject", "another-modal", r.source, owner, r.generation, focused)
			return
		}
		appFocused = appFocused || focused(win)
	}
	appActive := applicationActive(appFocused)
	if core.IsDebugOn() {
		core.Debug("focus-return application: key-window-focused=", appFocused, ", application-active=", appActive)
	}
	if !appActive {
		debugFocusReturn("reject", "application-unfocused", r.source, owner, r.generation, focused)
		return // The user switched to another application before queue drain.
	}
	if focusWidget == nil || !focusWidget.IsAllAncentorsVisible() {
		if target := focusTargetForOwner(owner, r.target); target != nil {
			target.SetFocus()
		}
	}
	debugFocusReturn("activate", "owner", r.source, owner, r.generation, focused)
	activate(owner)
	debugFocusReturn("activated", "native-focus-result", r.source, owner, r.generation, focused)
}
