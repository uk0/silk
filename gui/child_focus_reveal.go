package gui

// IChildFocusReveal lets a scrolling container reveal an inline descendant
// after it receives keyboard focus. It does not change focus or the authored
// widget geometry. Independently owned forms/popups stop this notification.
// Implementations run synchronously on the UI thread and must tolerate reentry.
type IChildFocusReveal interface {
	RevealChildFocus(target IWidget)
}

func revealFocusedChild(target IWidget) {
	for child := target; child != nil; {
		if focusWidget != target || !inputEnabled(target) {
			return
		}
		w := child.NakedWidget()
		if w.wt == WtForm || w.wt == WtPopup {
			return // Parent is a native owner, not a viewport ancestor.
		}
		parent := child.Parent()
		if parent == nil || !inlineFocusDescendant(parent, target) {
			return
		}
		if viewport, ok := parent.(IChildFocusReveal); ok {
			viewport.RevealChildFocus(target)
		}
		if child.Parent() != parent {
			return // Host callback reparented the branch; do not notify stale owners.
		}
		child = parent
	}
}

func inlineFocusDescendant(parent, target IWidget) bool {
	for child := target; child != nil; child = child.Parent() {
		if child == parent {
			return true
		}
		w := child.NakedWidget()
		if w.wt == WtForm || w.wt == WtPopup {
			return false
		}
	}
	return false
}
