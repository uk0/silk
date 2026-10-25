package gui

// SetNavigationList links a single-line search field to its result list.
// Unmodified Up/Down/PageUp/PageDown navigate and reveal rows without moving
// focus or editing the query. Home/End, modifiers, Enter and Tab keep their
// normal field behavior. The host submits list.ActiveItem/ActiveIndex from
// SigSubmit, so mouse and keyboard selection share one acceptance path.
//
// Both controls must belong to the same widget tree and native window. A
// hidden/disabled/detached list is ignored. Set nil to detach. This UI-thread
// API adds no timers, workers, filtering or submit/selection callbacks.
func (this *Edit) SetNavigationList(list *ListWidget) { this.navigationList = list }
func (this *Edit) NavigationList() *ListWidget        { return this.navigationList }

func (this *Edit) navigateResultList(key int, modified, repeat bool) bool {
	list := this.navigationList
	if list == nil || this.MultiLine() || modified {
		return false
	}
	switch key {
	case KeyUp, KeyDown, KeyPageUp, KeyPageDown:
	default:
		return false
	}
	if this.RootWidget() != list.RootWidget() || this.OwnerWindow() != list.OwnerWindow() {
		return false
	}
	// A native dialog is a visibility boundary: its logical owner may be a
	// hidden design canvas while its search controls are visible and enabled.
	for _, control := range []IWidget{this.Self(), list} {
		for p := control; p != nil; p = p.Parent() {
			if !p.IsVisible() || !p.IsEnabled() {
				return false
			}
			if p.Window() != nil {
				break
			}
		}
	}
	list.OnKeyDown(key, repeat)
	return true
}
