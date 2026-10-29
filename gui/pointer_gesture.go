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
	if win == nil {
		return
	}
	// Cancellation may PopCapture or dispose a control. Snapshot the owner set
	// before callbacks, without canceling gestures in independent native forms.
	var targets []IWidget
	for _, captured := range captureStack {
		if captured != nil && captured.OwnerWindow() == win {
			if _, ok := captured.(IEventPointerCancel); ok {
				targets = append(targets, captured)
			}
		}
	}
	for _, target := range targets {
		cancelPointerGesture(target)
	}
}

func (v *SceneView) OnPointerCancel() {
	if v.dragging || (v.translation != nil && v.translation.gesture != nil) {
		v.OnHide()
	}
}
