package main

import (
	"github.com/uk0/silk/gui"
	"os"
	"path/filepath"
	"testing"
)

func TestLineEndingMenuPaletteAndSavedBytes(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "eol.txt", "中🙂\r\nb\nc\r\nlast")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	e.SetFocus()
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	for _, s := range editorLineEndingCommands() {
		b := menuCommandButton(m, s.id)
		if b == nil || b.Icon() == nil || !b.IsEnabled() {
			t.Fatal(s.id)
		}
	}
	menuCommandButton(m, "editor.lineEndingLF").Action().Trigger(nil)
	if !saveEditorToDisk(e, path) {
		t.Fatal("save")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "中🙂\nb\nc\nlast" {
		t.Fatal(string(data), err)
	}
	e.Undo()
	if e.Text() != "中🙂\r\nb\nc\r\nlast" {
		t.Fatal("Undo")
	}
	p := bindEditorLineEndingCommand(w.tabs, w.canvas, editorLineEndingCommands()[1])
	if !p.available() {
		t.Fatal("palette unavailable")
	}
	p.Fn()
	if !saveEditorToDisk(e, path) {
		t.Fatal("save")
	}
	data, _ = os.ReadFile(path)
	if string(data) != "中🙂\r\nb\r\nc\r\nlast" {
		t.Fatal(string(data))
	}
}
func TestLineEndingCommandsRespectFocusIdentityAndReadOnly(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.txt", "a\nb"))
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	e.SetFocus()
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	p := bindEditorLineEndingCommand(w.tabs, w.canvas, editorLineEndingCommands()[1])
	e.SetReadOnly(true)
	menuCommandButton(m, "editor.lineEndingCRLF").Action().Trigger(nil)
	p.Fn()
	if e.Text() != "a\nb" {
		t.Fatal("readonly mutation")
	}
	e.SetReadOnly(false)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "b.txt", "other\n"))
	other := activeEditor(w.tabs)
	menuCommandButton(m, "editor.lineEndingCRLF").Action().Trigger(nil)
	p.Fn()
	if other.Text() != "other\n" || e.Text() != "a\nb" {
		t.Fatal("stale target")
	}
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	m2 := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m2)
	for _, s := range editorLineEndingCommands() {
		if menuCommandButton(m2, s.id).IsEnabled() {
			t.Fatal("stole text field focus")
		}
		runEditorLineEndingCommand(w.tabs, w.canvas, nil, s.style)
	}
	if other.Text() != "other\n" {
		t.Fatal("unfocused mutation")
	}
}
func TestSaveGoHonorsSelectedLineEnding(t *testing.T) {
	w := testWorkbench(t)
	dir := t.TempDir()
	path := writeFile(t, dir, "main.go", "package main\nfunc main( ){}\n")
	openFileInEditor(w.tabs, path)
	e := activeEditor(w.tabs)
	if err := e.ConvertLineEndings(gui.LineEndingCRLF); err != nil {
		t.Fatal(err)
	}
	if !saveEditorToDisk(e, path) {
		t.Fatal("save")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	want := "package main\r\n\r\nfunc main() {}\r\n"
	if string(data) != want || e.Text() != want || e.Modified() {
		t.Fatal("Go save lost selected CRLF", string(data))
	}
}

func TestEditMenuKeepsAdvancedCommandsInCompactSubmenus(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.txt", "a\nb"))
	w.activate(modeEdit)
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	for _, id := range []string{"editor.lineOps", "editor.lineEndings"} {
		b := menuCommandButton(m, id)
		if b == nil || b.Icon() == nil {
			t.Fatal("missing submenu", id)
		}
		if sub, ok := b.SubPopup().(*gui.Menu); !ok || len(sub.Items()) == 0 {
			t.Fatal("empty submenu", id)
		}
	}
	for _, spec := range editorCaretCommandSpecs() {
		if menuCommandButton(m, spec.id) == nil {
			t.Fatal("lost command", spec.id)
		}
	}
	if h := m.SizeHints().Height; h > 720 {
		t.Fatal("Edit menu exceeds an ordinary desktop viewport", h)
	}
}
