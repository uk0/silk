package gui

// CharacterPaletteAvailable reports whether this backend provides a native
// Unicode character chooser. It is currently supported by macOS with cgo.
func CharacterPaletteAvailable() bool { return nativeCharacterPaletteAvailable }

var requestCharacterPalette = nativeShowCharacterPalette

// ShowCharacterPalette requests the OS character chooser for an already focused
// text widget. Call on the UI thread, after a context menu has closed. It never
// steals focus from another widget/window, reads the clipboard or inserts text
// itself; commits follow normal native text input, validation and Undo routing.
// False means unsupported or ineligible; true means requested, not committed.
func ShowCharacterPalette(target IWidget) bool {
	if !CharacterPaletteAvailable() || target == nil || focusWidget != target {
		return false
	}
	if _, ok := target.(IInputMethodCursor); !ok {
		return false
	}
	win := target.OwnerWindow()
	if win == nil || !win.IsEnabled() || win.focusedInputTarget() != target || win.textInputTarget() == nil {
		return false
	}
	return requestCharacterPalette(win)
}
