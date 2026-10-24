package gui

// Only the existing multi-caret chord opts into WM_SYSKEYDOWN. Never consume
// arbitrary Alt keys: window closing, system menus and AltGr remain native.
func editorSystemKey(key int, action, alt bool) bool {
	return action && alt && (key == KeyUp || key == KeyDown)
}

func (this *CodeEditor) OnSystemKeyDown(key int, repeat bool) bool {
	if !editorSystemKey(key, IsKeyDown(KeyCtrl) || isActionModifier(), IsKeyDown(KeyMenu)) {
		return false
	}
	this.OnKeyDown(key, repeat)
	return true
}
