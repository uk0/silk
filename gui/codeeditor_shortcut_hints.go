package gui

import "runtime"

// CodeEditorShortcutHints describes the built-in editor keys for menu/help UI.
// It is presentation metadata, not a keybinding registry. A host that overrides
// a binding must show its own hint. Each call returns an independent value.
type CodeEditorShortcutHints struct {
	Undo, Redo                                                                           string
	RenameSymbol, GoToDefinition, FindReferences, GoToTypeDefinition, GoToImplementation string
	Cut, Copy, Paste, SelectAll                                                          string
	AddCursorAbove, AddCursorBelow, KeepPrimaryCursor                                    string
	MoveLinesUp, MoveLinesDown, DuplicateLines, DeleteLines                              string
	IndentLines, OutdentLines                                                            string
	GoToLine, GoBack, GoForward                                                          string
}

// CodeEditorShortcuts returns platform-aware built-in shortcuts. On Windows the
// preferred multi-caret chord includes Shift: some desktop/driver hotkeys consume
// Ctrl+Alt+arrows before a key-down reaches the app. The old chord remains an
// accepted alias when delivered; no global hook or key-up synthesis is used.
func CodeEditorShortcuts() CodeEditorShortcutHints {
	return codeEditorShortcutsForOS(runtime.GOOS)
}

func codeEditorShortcutsForOS(goos string) CodeEditorShortcutHints {
	key := "Ctrl"
	if goos == "darwin" {
		key = "Cmd"
	}
	caret := key + "+Alt"
	if goos == "windows" {
		caret = key + "+Shift+Alt"
	}
	return CodeEditorShortcutHints{
		Undo: key + "+Z", Redo: key + "+Shift+Z",
		RenameSymbol: key + "+F2", GoToDefinition: "F12", FindReferences: "Shift+F12", GoToTypeDefinition: key + "+F12", GoToImplementation: "Alt+F12",
		Cut: key + "+X", Copy: key + "+C", Paste: key + "+V", SelectAll: key + "+A",
		AddCursorAbove: caret + "+Up", AddCursorBelow: caret + "+Down", KeepPrimaryCursor: "Esc",
		MoveLinesUp: "Alt+Up", MoveLinesDown: "Alt+Down", DuplicateLines: key + "+Shift+D", DeleteLines: key + "+Shift+K",
		IndentLines: "Tab", OutdentLines: "Shift+Tab", GoToLine: key + "+G", GoBack: "Alt+Left", GoForward: "Alt+Right",
	}
}
