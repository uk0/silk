package main

import (
	"testing"

	"github.com/uk0/silk/gui"
)

func TestEditorHintSnapshotRejectsRetiredInteractionAndIdentity(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "hint.go", "package p\n// 😀 Target\n")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	ed := openEditors[path]
	hint := captureEditorHint(ed, path)
	if !hint.current() {
		t.Fatal("fresh snapshot rejected")
	}
	if got := utf16ColForRune(ed, 1, 5); got != 6 {
		t.Fatal("hover rune to UTF16", got)
	}
	ed.OnKeyDown(gui.KeyEsc, false)
	if hint.current() {
		t.Fatal("late response resurrected dismissed hint")
	}
	hint = captureEditorHint(ed, path)
	ed.ReplaceAllText("package changed\n")
	if hint.current() {
		t.Fatal("hint survived text edit")
	}
	hint = captureEditorHint(ed, path)
	ed.SetVisible(false)
	if hint.current() {
		t.Fatal("hidden editor accepted hint")
	}
	ed.SetVisible(true)
	hint = captureEditorHint(ed, path)
	delete(openEditors, path)
	if hint.current() {
		t.Fatal("closed editor accepted hint")
	}
	openEditors[path] = ed
}
