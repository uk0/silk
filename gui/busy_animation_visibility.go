package gui

// Only widgets with continuous visual work opt in. Ordinary OnShow/OnHide
// handlers keep their historical local-only semantics; propagating those
// events would incorrectly reopen popups or restart unrelated controllers.
type busyAnimationVisibilityObserver interface {
	syncBusyAnimationVisibility()
}

// notifyBusyAnimationVisibility is called after attachment/visibility changes,
// not from Draw or the idle loop. It also covers removing an entire container:
// descendants keep their direct parent, but no longer belong to a window.
func notifyBusyAnimationVisibility(root IWidget) {
	if root == nil {
		return
	}
	pending := []IWidget{root}
	for len(pending) > 0 {
		w := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if observer, ok := w.(busyAnimationVisibilityObserver); ok {
			observer.syncBusyAnimationVisibility()
		}
		if observer, ok := w.(IEventTreeVisibility); ok {
			observer.OnTreeVisibilityChanged(WidgetTreeVisible(w))
		}
		pending = append(pending, w.Children()...)
	}
}

func busyAnimationVisible(w IWidget) bool {
	attached := false
	for p := w; p != nil; p = p.Parent() {
		if !p.IsVisible() {
			return false
		}
		if p.Window() != nil {
			attached = true
		}
	}
	// A visible standalone widget or an unmounted form cannot paint anything.
	// Native minimization/occlusion is handled by the window scheduler, while
	// Widget visibility is the portable source of truth for subtree lifecycle.
	return attached
}
