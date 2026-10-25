package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestEditorFormatMenuPaletteShortcutAndDocumentIdentity(t *testing.T) {
	w := testWorkbench(t)
	oldStart, oldPalette, oldLSP := startLocalFormat, paletteCommands, globalLSP
	t.Cleanup(func() { startLocalFormat, paletteCommands, globalLSP = oldStart, oldPalette, oldLSP })
	globalLSP = nil
	var targets []*gui.CodeEditor
	var compact []bool
	startLocalFormat = func(ed *gui.CodeEditor, o gui.EditorFormatOptions, done func(error)) error {
		targets = append(targets, ed)
		compact = append(compact, o.Compact)
		done(nil)
		return nil
	}
	path := writeFile(t, t.TempDir(), "a.json", `{"x":1}`)
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	for _, id := range []string{"editor.format", "editor.compactJSON", "editor.cancelFormat"} {
		b := menuCommandButton(menu, id)
		if b == nil || b.Icon() == nil || b.Text() == "" {
			t.Fatal("missing format menu", id)
		}
		if b.IsEnabled() != (id != "editor.cancelFormat") {
			t.Fatal("wrong enabled state", id)
		}
	}
	menuCommandButton(menu, "editor.format").Action().Trigger(nil)
	menuCommandButton(menu, "editor.compactJSON").Action().Trigger(nil)
	registerPaletteCommands(w.tabs, w.canvas)
	for _, label := range []string{"Format Document", "Compact JSON"} {
		found := false
		for _, c := range paletteCommands {
			if c.Name == i18n.T(label) {
				found = true
				c.Fn()
			}
		}
		if !found {
			t.Fatal("missing palette", label)
		}
	}
	oldShortcut := gui.ShortcutHandler(gui.ModAction|gui.ModShift, 'I')
	t.Cleanup(func() { gui.RegisterShortcut(gui.ModAction|gui.ModShift, 'I', oldShortcut) })
	registerEditorFormatShortcut(w.tabs, w.canvas)
	if h := gui.ShortcutHandler(gui.ModAction|gui.ModShift, 'I'); h != nil {
		h()
	} else {
		t.Fatal("shortcut missing")
	}
	if len(targets) != 5 || compact[0] || !compact[1] || compact[2] || !compact[3] || compact[4] {
		t.Fatal("wrong command routing", compact)
	}
	for _, target := range targets {
		if target != ed {
			t.Fatal("wrong editor")
		}
	}
	other := writeFile(t, t.TempDir(), "b.json", `{"y":2}`)
	openFileInEditor(w.tabs, other)
	menuCommandButton(menu, "editor.format").Action().Trigger(nil)
	if len(targets) != 5 {
		t.Fatal("stale menu formatted replacement editor")
	}
	w.activate(modeDesign)
	formatActiveDocument(w.tabs, w.canvas, nil, false)
	if len(targets) != 5 {
		t.Fatal("formatted hidden document")
	}
	if menuCommandButton(buildWorkbenchSubMenu("Edit", w.tabs, w.canvas), "editor.format").IsEnabled() {
		t.Fatal("hidden format enabled")
	}
}

func TestLocalFormatFailureReportsWithoutMutatingText(t *testing.T) {
	w := testWorkbench(t)
	oldStart, oldToast := startLocalFormat, silkideToast
	t.Cleanup(func() { startLocalFormat, silkideToast = oldStart, oldToast })
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.json", `{"x":]}`))
	w.activate(modeEdit)
	var message string
	silkideToast = func(s string, l gui.ToastLevel) {
		message = s
		if l != gui.ToastError {
			t.Error("wrong severity")
		}
	}
	startLocalFormat = func(_ *gui.CodeEditor, _ gui.EditorFormatOptions, done func(error)) error {
		done(errors.New("invalid JSON"))
		return nil
	}
	formatActiveDocument(w.tabs, w.canvas, nil, false)
	if !strings.Contains(message, "invalid JSON") || activeEditor(w.tabs).Text() != `{"x":]}` {
		t.Fatal(message)
	}
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.txt", "plain"))
	if menuCommandButton(buildWorkbenchSubMenu("Edit", w.tabs, w.canvas), "editor.format").IsEnabled() {
		t.Fatal("plain text format enabled")
	}
}
