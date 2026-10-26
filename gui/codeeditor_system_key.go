package gui

// Action+Alt+vertical-arrow adds a caret, with or without Shift. Windows menus
// advertise Ctrl+Shift+Alt to avoid reserved Ctrl+Alt desktop shortcuts.
// Bare Alt+vertical-arrow moves lines.
// Other Alt keys (window closing, system menus and AltGr text) remain native.
func editorSystemKey(key int, action, alt bool) bool {
	return action && alt && (key == KeyUp || key == KeyDown)
}

func editorLineMoveSystemKey(key int, action, alt bool) bool {
	return !action && alt && (key == KeyUp || key == KeyDown)
}

func (this *CodeEditor) OnSystemKeyDown(key int, repeat bool) bool {
	if this.editorContextKey(key, repeat) {
		return true
	}
	action, alt := IsKeyDown(KeyCtrl) || isActionModifier(), IsKeyDown(KeyMenu)
	if !editorSystemKey(key, action, alt) && !editorLineMoveSystemKey(key, action, alt) {
		return false
	}
	this.OnKeyDown(key, repeat)
	return true
}
