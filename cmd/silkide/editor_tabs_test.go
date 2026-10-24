package main

import (
	"github.com/uk0/silk/gui"
	"strings"
	"testing"
)

func TestEditorTabCommandsPreserveBuffersAndRevealEdit(t *testing.T) {
	for _, entry := range []string{"menu", "palette"} {
		t.Run(entry, func(t *testing.T) {
			w := testWorkbench(t)
			w.tabs.SetSize(600, 350)
			dir := t.TempDir()
			a := writeFile(t, dir, "a.txt", strings.Repeat("中🙂 line\n", 100))
			b := writeFile(t, dir, "b.txt", "beta")
			openFileInEditor(w.tabs, a)
			ea := activeEditor(w.tabs)
			ea.NavigateToLineCol(40, 3)
			ea.OnTextInput("x")
			pos := ea.NavigationPosition()
			openFileInEditor(w.tabs, b)
			eb := activeEditor(w.tabs)
			w.activate(modeWelcome)
			if entry == "menu" {
				btn := menuCommandButton(buildWorkbenchSubMenu("View", w.tabs, w.canvas), "editor.next")
				if btn == nil || !btn.IsEnabled() || btn.Icon() == nil {
					t.Fatal("next editor menu missing")
				}
				btn.Action().Trigger(nil)
			} else {
				old := paletteCommands
				t.Cleanup(func() { paletteCommands = old })
				registerPaletteCommands(w.tabs, w.canvas)
				found := false
				for _, c := range paletteCommands {
					if c.Name == "Next Editor" {
						c.Fn()
						found = true
						break
					}
				}
				if !found {
					t.Fatal("palette command missing")
				}
			}
			if activeEditor(w.tabs) != ea || !ea.HasFocus() || documentMode(w.tabs, w.canvas) != modeEdit || ea.NavigationPosition() != pos || !ea.Modified() {
				t.Fatal("navigation lost page, cursor, dirty state or focus")
			}
			menuCommandButton(buildWorkbenchSubMenu("View", w.tabs, w.canvas), "editor.previous").Action().Trigger(nil)
			if activeEditor(w.tabs) != eb || !eb.HasFocus() {
				t.Fatal("previous editor not focused")
			}
			navigateEditors(w.tabs, false)
			ea.Undo()
			if ea.Text() != strings.Repeat("中🙂 line\n", 100) {
				t.Fatal("navigation changed undo history")
			}
		})
	}
}
func TestEditorTabCommandsEmptySingleAndForeign(t *testing.T) {
	w := testWorkbench(t)
	for _, stage := range []string{"empty", "single"} {
		if stage == "single" {
			openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.txt", "a"))
		}
		for _, id := range []string{"editor.next", "editor.previous"} {
			if menuCommandButton(buildWorkbenchSubMenu("View", w.tabs, w.canvas), id).IsEnabled() {
				t.Fatal("non-actionable menu enabled")
			}
		}
		if navigateEditors(w.tabs, false) || canNavigateEditors(gui.NewTabWidget()) {
			t.Fatal("empty/single/foreign group accepted")
		}
	}
}
