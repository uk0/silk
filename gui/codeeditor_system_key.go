package gui

// Ctrl+Alt+vertical-arrow adds a caret; bare Alt+vertical-arrow moves lines.
// Other Alt keys (window closing, system menus and AltGr text) remain native.
func editorSystemKey(key int, action, alt bool) bool {
	return action && alt && (key == KeyUp || key == KeyDown)
}

func editorLineMoveSystemKey(key int, action, alt bool) bool {
	return !action && alt && (key == KeyUp || key == KeyDown)
}

func (this *CodeEditor) OnSystemKeyDown(key int, repeat bool) bool {
	action, alt := IsKeyDown(KeyCtrl) || isActionModifier(), IsKeyDown(KeyMenu)
	if !editorSystemKey(key, action, alt) && !editorLineMoveSystemKey(key, action, alt) {
		return false
	}
	this.OnKeyDown(key, repeat)
	return true
}
