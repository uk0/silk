package gui

// IChildEnablement lets a compound control gate its direct children's effective
// enabled state without overwriting their explicit SetEnabled flags. The getter
// runs on the UI thread: it must not mutate the tree or call child.IsEnabled().
// Independently owned native forms/popups do not inherit this gate.
type IChildEnablement interface {
	IsChildEnabled(child IWidget) bool
}

// IsExplicitlyEnabled returns only this widget's authored SetEnabled flag,
// without ancestor gates or an Action's state. Persist this value, not a
// transient effective disabled state inherited from a container.
func (w *Widget) IsExplicitlyEnabled() bool { return !w.disabled }

// IsEnabled reports effective availability, preserving derived-parent checks
// such as a Button's Action state. Native child surfaces still inherit, while
// an independent form/popup's parent is an owner rather than an input gate.
func (w *Widget) IsEnabled() bool {
	if w.disabled {
		return false
	}
	if w.parent == nil || w.wt == WtForm || w.wt == WtPopup {
		return true
	}
	if !w.parent.IsEnabled() {
		return false
	}
	if gate, ok := w.parent.(IChildEnablement); ok {
		return gate.IsChildEnabled(w.self)
	}
	return true
}

// Native routing must also fence older SDK controls whose IsEnabled override
// reports only local state, without delegating to Widget. Keep ordinary widget
// queries cheap; validate the complete native input scope only at admission.
func inputEnabled(target IWidget) bool {
	if target == nil {
		return false
	}
	for current := target; current != nil; {
		w := current.NakedWidget()
		if w.disabled || !current.IsEnabled() {
			return false
		}
		if w.wt == WtForm || w.wt == WtPopup {
			return true
		}
		parent := w.parent
		if gate, ok := parent.(IChildEnablement); ok && !gate.IsChildEnabled(current) {
			return false
		}
		current = parent
	}
	return true
}

// No worker or timer is needed when a gate changes. A disabled inline subtree
// cancels pending clipboard reads, but independent native owners stay intact.
func cancelInlineClipboardPastes(root IWidget) {
	if root == nil {
		return
	}
	root.NakedWidget().CancelPendingPaste()
	if transient, ok := root.(interface{ onInputDisabled() }); ok {
		transient.onInputDisabled()
	}
	root.NakedWidget().eachChild(func(_ int, child IWidget) bool {
		w := child.NakedWidget()
		if w.wt != WtForm && w.wt != WtPopup {
			cancelInlineClipboardPastes(child)
		}
		return true
	})
}

// InvalidateEnabledState synchronously retires input belonging to this now-
// disabled subtree and schedules repaint. Custom IChildEnablement containers
// call this on each affected direct child after changing their gate. No local
// SetEnabled flag is rewritten and no polling timer/worker is created.
func (w *Widget) InvalidateEnabledState() {
	root := w.Self()
	if root == nil {
		return
	}
	assertUIThread("Widget.InvalidateEnabledState")
	if !inputEnabled(root) {
		cancelInlineClipboardPastes(root)
	}
	retireDisabledInput()
	updateInlineNativeEnabled(root)
	root.Update()
}

func updateInlineNativeEnabled(root IWidget) {
	root.NakedWidget().eachChild(func(_ int, child IWidget) bool {
		w := child.NakedWidget()
		if w.wt != WtForm && w.wt != WtPopup {
			if child.Window() != nil {
				child.Update()
			}
			updateInlineNativeEnabled(child)
		}
		return true
	})
}

func retireDisabledInput() {
	// Cancellation is host code: snapshot before callbacks, then filter the
	// current stack rather than reinstalling a stale snapshot over new owners.
	mouse := lastMouseWidget
	mouseDisabled := mouse != nil && !inputEnabled(mouse)
	var mouseWindow *Window
	if mouseDisabled {
		mouseWindow = mouse.OwnerWindow()
	}
	captured := append([]IWidget(nil), captureStack...)
	var smallCancelled [8]IWidget
	cancelled := smallCancelled[:0]
	wasCancelled := func(target IWidget) bool {
		for _, old := range cancelled {
			if old == target {
				return true
			}
		}
		return false
	}
	for _, target := range captured {
		if target != nil && !inputEnabled(target) {
			cancelled = append(cancelled, target)
			cancelPointerGesture(target)
		}
	}
	if mouseDisabled {
		if !wasCancelled(mouse) {
			cancelled = append(cancelled, mouse)
			cancelPointerGesture(mouse)
		}
		// Reparenting/reenabling inside the callback cannot revive this old
		// press. Retire its original window flags, not its possibly new owner.
		if mouseWindow != nil {
			mouseWindow.autoCaptured, mouseWindow.toCapture = false, false
		}
		if lastMouseWidget == mouse {
			lastMouseWidget = nil
		}
	}
	previous := curCapture()
	kept := captureStack[:0]
	for _, target := range captureStack {
		if target != nil && inputEnabled(target) && !wasCancelled(target) {
			kept = append(kept, target)
		}
	}
	clear(captureStack[len(kept):])
	captureStack = kept
	if current := curCapture(); current != previous {
		if current == nil {
			releaseCapture()
		} else {
			setCapture(current)
		}
	}
	if mouseHoverWidget != nil && !inputEnabled(mouseHoverWidget) {
		setMouseHoverWidget(nil)
	}
	if old := focusWidget; old != nil && !inputEnabled(old) {
		old.NakedWidget().CancelPendingPaste()
		textInputFocusChanged()
		// Keep logical focus remembered across a temporary disable. Native
		// routing rejects it while ineligible; reenabling restores the same
		// field instead of unexpectedly redirecting the next keystroke.
	}
}
