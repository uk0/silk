package gui

// IEventPointerCancel opts a control into cancellation when its pointer capture
// is displaced, its native window loses input focus/capture, or is torn down.
// Callbacks run on the UI thread and must retire their gesture before releasing
// capture. They must not commit an edit or reacquire capture during cancellation.
type IEventPointerCancel interface {
	OnPointerCancel()
}

func cancelPointerGesture(widget IWidget) {
	if target, ok := widget.(IEventPointerCancel); ok {
		target.OnPointerCancel()
	}
}

func (win *Window) cancelPointerGestures() {
	if win == nil || win.cancelingPointer {
		return
	}
	win.cancelingPointer = true
	defer func() { win.cancelingPointer = false }()
	win.autoCaptured, win.toCapture = false, false
	oldMouse := lastMouseWidget
	mouseOwned := oldMouse != nil && oldMouse.OwnerWindow() == win
	// Snapshot ownership before host callbacks can dispose/reparent controls.
	// Keep legacy captures too: no cancellation hook does not grant continued
	// routing after the native surface has lost its input ownership.
	var small [8]IWidget
	retired := small[:0]
	for _, captured := range captureStack {
		if captured != nil && captured.OwnerWindow() == win {
			retired = append(retired, captured)
		}
	}
	wasRetired := func(widget IWidget) bool {
		for _, old := range retired {
			if old.NakedWidget() == widget.NakedWidget() {
				return true
			}
		}
		return false
	}
	if mouseOwned && !wasRetired(oldMouse) {
		retired = append(retired, oldMouse)
	}
	// Retire inner captures before their ancestors; restoring an already-
	// canceled ancestor must not turn it back into a live gesture.
	for i := len(retired) - 1; i >= 0; i-- {
		cancelPointerGesture(retired[i])
	}
	if mouseHoverWidget != nil && mouseHoverWidget.OwnerWindow() == win {
		setMouseHoverWidget(nil)
	}
	previous := curCapture()
	kept := captureStack[:0]
	for _, captured := range captureStack {
		if captured != nil && captured.OwnerWindow() != win && !wasRetired(captured) {
			kept = append(kept, captured)
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
	// PopCapture may restore an independent capture. Preserve that capture,
	// not the canceled event's mouse target or an uncaptured old press.
	if mouseOwned && (lastMouseWidget == oldMouse || lastMouseWidget == curCapture()) {
		lastMouseWidget = nil
	}
	if lastMouseWidget != nil && lastMouseWidget.OwnerWindow() == win {
		lastMouseWidget = nil
	}
}

func (v *SceneView) OnPointerCancel() {
	if v.dragging || (v.translation != nil && v.translation.gesture != nil) {
		v.OnHide()
	}
}
