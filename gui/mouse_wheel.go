package gui

import "math"

// Wheel routing is shared by Win32 and GLFW: use the current pointer/capture,
// not a stale hover widget, then bubble through non-scrollable children. Screen
// coordinates matter when a captured popup lives in another native window.
func dispatchMouseWheel(root IWidget, gx, gy, delta float64) bool {
	if root == nil || delta == 0 || math.IsNaN(gx+gy+delta) || math.IsInf(gx+gy+delta, 0) {
		return false
	}
	menuPointer.keyboard = false
	target := curCapture()
	if target == nil {
		x, y := root.MapFromGlobal(gx, gy)
		target = root.FindWidgetAt(x, y)
	}
	for w := target; w != nil; w = w.Parent() {
		if !w.IsVisible() || !w.IsAllAncentorsVisible() {
			return false
		}
		if handler, ok := w.(IEventMouseWheel); ok && w.IsEnabled() {
			x, y := w.MapFromGlobal(gx, gy)
			handler.OnMouseWheel(x, y, delta)
			return true
		}
	}
	return false
}
