package gui

import "testing"

func TestCodeEditorShortcutHintsPlatformAndValueIsolation(t *testing.T) {
	for _, tc := range []struct{ os, action, caret string }{
		{"windows", "Ctrl", "Ctrl+Shift+Alt"},
		{"darwin", "Cmd", "Cmd+Alt"},
		{"linux", "Ctrl", "Ctrl+Alt"},
	} {
		t.Run(tc.os, func(t *testing.T) {
			got := codeEditorShortcutsForOS(tc.os)
			want := CodeEditorShortcutHints{
				Undo: tc.action + "+Z", Redo: tc.action + "+Shift+Z", RenameSymbol: tc.action + "+F2", GoToDefinition: "F12", FindReferences: "Shift+F12", GoToTypeDefinition: tc.action + "+F12", GoToImplementation: "Alt+F12",
				Cut: tc.action + "+X", Copy: tc.action + "+C", Paste: tc.action + "+V", SelectAll: tc.action + "+A",
				AddCursorAbove: tc.caret + "+Up", AddCursorBelow: tc.caret + "+Down", KeepPrimaryCursor: "Esc",
				MoveLinesUp: "Alt+Up", MoveLinesDown: "Alt+Down", DuplicateLines: tc.action + "+Shift+D", DeleteLines: tc.action + "+Shift+K",
				IndentLines: "Tab", OutdentLines: "Shift+Tab", GoToLine: tc.action + "+G", GoBack: "Alt+Left", GoForward: "Alt+Right",
			}
			if got != want {
				t.Fatalf("got %+v want %+v", got, want)
			}
			got.AddCursorBelow = "host-local"
			if codeEditorShortcutsForOS(tc.os) != want {
				t.Fatal("caller mutation changed shared hints")
			}
		})
	}
}

func TestEditorContextClipboardAndCaretShortcutHints(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("first\nsecond\n")
	keys := CodeEditorShortcuts()
	want := map[string]string{"Cut": keys.Cut, "Copy": keys.Copy, "Paste": keys.Paste, "Select All": keys.SelectAll,
		"Add Cursor Above": keys.AddCursorAbove, "Add Cursor Below": keys.AddCursorBelow, "Keep Primary Cursor": keys.KeepPrimaryCursor}
	for label, hint := range want {
		item := findMenuItem(e.contextMenuItems(""), label)
		if item == nil || item.Shortcut != hint || item.Icon == "" {
			t.Fatalf("missing hint/icon for %s: %+v", label, item)
		}
	}
	if findMenuItem(e.contextMenuItems(""), "Add Cursor Above").Enabled {
		t.Fatal("hint enabled unavailable command")
	}
}
