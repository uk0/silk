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
				Cut: tc.action + "+X", Copy: tc.action + "+C", Paste: tc.action + "+V", SelectAll: tc.action + "+A",
				AddCursorAbove: tc.caret + "+Up", AddCursorBelow: tc.caret + "+Down", KeepPrimaryCursor: "Esc",
				MoveLinesUp: "Alt+Up", MoveLinesDown: "Alt+Down", DuplicateLines: tc.action + "+Shift+D", DeleteLines: tc.action + "+Shift+K",
				IndentLines: "Tab", OutdentLines: "Shift+Tab", GoToLine: tc.action + "+G",
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
	want := map[string]string{"剪切": keys.Cut, "复制": keys.Copy, "粘贴": keys.Paste, "全选": keys.SelectAll,
		"向上添加光标": keys.AddCursorAbove, "向下添加光标": keys.AddCursorBelow, "仅保留主光标": keys.KeepPrimaryCursor}
	for label, hint := range want {
		item := findMenuItem(e.contextMenuItems(""), label)
		if item == nil || item.Shortcut != hint || item.Icon == "" {
			t.Fatalf("missing hint/icon for %s: %+v", label, item)
		}
	}
	if findMenuItem(e.contextMenuItems(""), "向上添加光标").Enabled {
		t.Fatal("hint enabled unavailable command")
	}
}
