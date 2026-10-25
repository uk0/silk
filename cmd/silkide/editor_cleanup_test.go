package main

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"os"
	"strings"
	"testing"
)

func TestCleanupMenuPaletteAndSavedBytes(t *testing.T) {
	w := testWorkbench(t)
	source := "中🙂 \t\r\nb\t\nlast  "
	path := writeFile(t, t.TempDir(), "cleanup.txt", source)
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	e.SetFocus()
	e.SelectAll()
	e.SetBookmark(2, true)
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	for _, s := range editorCleanupCommands() {
		b := menuCommandButton(menu, s.id)
		if b == nil || b.Icon() == nil || !b.IsEnabled() {
			t.Fatal("missing usable command", s.id)
		}
	}
	menuCommandButton(menu, "editor.trimTrailing").Action().Trigger(nil)
	trimmed := "中🙂\r\nb\nlast"
	if e.Text() != trimmed || e.SelectedText() != trimmed || !e.HasBookmark(2) {
		t.Fatal("menu anchors", e.Text(), e.SelectedText())
	}
	if !saveEditorToDisk(e, path) {
		t.Fatal("save")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != trimmed {
		t.Fatal("saved trim bytes", err, string(data))
	}
	p := bindEditorCleanupCommand(w.tabs, w.canvas, editorCleanupCommands()[1])
	if !p.available() {
		t.Fatal("append unavailable")
	}
	p.Fn()
	if e.Text() != trimmed+"\r\n" || e.SelectedText() != trimmed || !e.HasBookmark(2) {
		t.Fatal("append changed anchors", e.Text())
	}
	if !saveEditorToDisk(e, path) {
		t.Fatal("save")
	}
	data, _ = os.ReadFile(path)
	if string(data) != trimmed+"\r\n" {
		t.Fatal("saved final newline", string(data))
	}
	e.Undo()
	e.Undo()
	if e.Text() != source || e.SelectedText() != source {
		t.Fatal("two undo", e.Text(), e.SelectedText())
	}
	e.SetFocus()
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	registerPaletteCommands(w.tabs, w.canvas)
	for _, spec := range editorCleanupCommands() {
		count := 0
		for _, c := range paletteCommands {
			if c.Name == i18n.T(spec.label) {
				count++
				if c.Fn == nil || c.Bind == nil || !c.Bind().available() {
					t.Fatal("incomplete palette command", spec.id)
				}
			}
		}
		if count != 1 {
			t.Fatal("duplicated/missing command", spec.label, count)
		}
	}
}
func TestCleanupGuardsReadonlyFocusAndStaleTargets(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "a  ")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	e.SetFocus()
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	bound := []paletteCommand{}
	for _, s := range editorCleanupCommands() {
		bound = append(bound, bindEditorCleanupCommand(w.tabs, w.canvas, s))
	}
	e.SetReadOnly(true)
	for i, s := range editorCleanupCommands() {
		menuCommandButton(m, s.id).Action().Trigger(nil)
		bound[i].Fn()
		if bound[i].available() {
			t.Fatal("readonly command available")
		}
	}
	if e.Text() != "a  " || e.CanUndo() {
		t.Fatal("readonly mutation")
	}
	e.SetReadOnly(false)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "b.txt", "b  "))
	other := activeEditor(w.tabs)
	for i, s := range editorCleanupCommands() {
		menuCommandButton(m, s.id).Action().Trigger(nil)
		bound[i].Fn()
	}
	if e.Text() != "a  " || other.Text() != "b  " {
		t.Fatal("stale target mutation")
	}
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	m2 := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m2)
	for _, s := range editorCleanupCommands() {
		if menuCommandButton(m2, s.id).IsEnabled() || bindEditorCleanupCommand(w.tabs, w.canvas, s).available() {
			t.Fatal("stole input focus")
		}
		runEditorCleanupCommand(w.tabs, w.canvas, nil, s)
	}
	if other.Text() != "b  " {
		t.Fatal("input focus mutation")
	}
	w.activate(modeDesign)
	for _, s := range editorCleanupCommands() {
		runEditorCleanupCommand(w.tabs, w.canvas, nil, s)
	}
	if other.Text() != "b  " {
		t.Fatal("design mode mutation")
	}
}
func TestCleanupErrorFeedbackAndFinalNewlineAvailability(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "unsafe.txt", "a\r \nlast  ")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	e.SetFocus()
	old := silkideToast
	t.Cleanup(func() { silkideToast = old })
	warnings := 0
	silkideToast = func(text string, level gui.ToastLevel) {
		if !strings.Contains(text, "CR") || level != gui.ToastWarning {
			t.Fatal(text, level)
		}
		warnings++
	}
	runEditorCleanupCommand(w.tabs, w.canvas, e, editorCleanupCommands()[0])
	if warnings != 1 || e.Text() != "a\r \nlast  " || e.CanUndo() {
		t.Fatal("atomic warning", warnings)
	}
	e.SetText("last\r")
	runEditorCleanupCommand(w.tabs, w.canvas, e, editorCleanupCommands()[1])
	if warnings != 2 || e.Text() != "last\r" || e.CanUndo() {
		t.Fatal("append warning")
	}
	for _, text := range []string{"", "already\n", "already\r\n\r\n"} {
		e.SetText(text)
		if editorCleanupCommands()[1].available(e) {
			t.Fatal("no-op append enabled", text)
		}
	}
}
