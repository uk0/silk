package gui

// IEventTreeVisibility is opt-in, UI-thread visibility notification after a
// widget/ancestor is shown, hidden or reparented. It may repeat the same value.
// Unlike local OnShow/OnHide, it never implies automatic controller startup.
// This is logical widget-tree visibility, not native occlusion/minimization.
type IEventTreeVisibility interface {
	OnTreeVisibilityChanged(visible bool)
}

// WidgetTreeVisible checks the widget and its ancestors, without I/O or polling.
// An unmounted visible widget is logically visible; it is not necessarily drawn.
func WidgetTreeVisible(widget IWidget) bool {
	if widget == nil {
		return false
	}
	for p := widget; p != nil; p = p.Parent() {
		if !p.IsVisible() {
			return false
		}
	}
	return true
}
