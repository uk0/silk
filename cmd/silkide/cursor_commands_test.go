package main

import (
	"github.com/uk0/silk/i18n"
	"testing"
)

func TestCaretMenuAndPaletteAreDiscoverable(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "carets.txt", "one\ntwo\nthree")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	w.activate(modeEdit)
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	for _, id := range []string{"editor.addCursorAbove", "editor.addCursorBelow", "editor.clearCursors"} {
		b := menuCommandButton(menu, id)
		if b == nil {
			t.Errorf("missing Edit menu command %s", id)
			continue
		}
		if b.Icon() == nil {
			t.Errorf("missing icon %s", id)
		}
	}
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	registerPaletteCommands(w.tabs, w.canvas)
	for _, name := range []string{"Add Cursor Above", "Add Cursor Below", "Keep Primary Cursor"} {
		found := false
		for _, c := range paletteCommands {
			if c.Name == i18n.T(name) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing palette command %s", name)
		}
	}
}

func TestCaretMenuRoutingAndStaleDocumentGuard(t *testing.T) {
	w := testWorkbench(t)
	first := writeFile(t, t.TempDir(), "a.txt", "one\ntwo\nthree")
	openFileInEditor(w.tabs, first)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	if menuCommandButton(m, "editor.addCursorAbove").IsEnabled() || menuCommandButton(m, "editor.clearCursors").IsEnabled() {
		t.Fatal("boundary actions enabled")
	}
	b := menuCommandButton(m, "editor.addCursorBelow")
	b.Action().Trigger(b)
	if ed.CursorCount() != 2 {
		t.Fatal("menu action did not add caret")
	}
	ed.OnTextInput("!")
	if ed.Text() != "!one\n!two\nthree" {
		t.Fatal("menu caret edit", ed.Text())
	}
	ed.Undo()
	if ed.Text() != "one\ntwo\nthree" || ed.CursorCount() != 2 {
		t.Fatal("menu undo")
	}
	w.activate(modeDesign)
	b.Action().Trigger(b)
	if ed.CursorCount() != 2 {
		t.Fatal("hidden editor caret changed")
	}
	w.activate(modeEdit)
	second := writeFile(t, t.TempDir(), "b.txt", "four\nfive")
	openFileInEditor(w.tabs, second)
	other := activeEditor(w.tabs)
	b.Action().Trigger(b)
	if ed.CursorCount() != 2 || other.CursorCount() != 1 {
		t.Fatal("stale menu changed another document")
	}
}

func TestCaretPaletteResolvesVisibleEditorAtExecution(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	registerPaletteCommands(w.tabs, w.canvas)
	var add, clear func()
	for _, c := range paletteCommands {
		if c.Name == i18n.T("Add Cursor Below") {
			add = c.Fn
		}
		if c.Name == i18n.T("Keep Primary Cursor") {
			clear = c.Fn
		}
	}
	if add == nil || clear == nil {
		t.Fatal("commands missing")
	}
	path := writeFile(t, t.TempDir(), "palette.txt", "x\ny")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	add()
	if ed.CursorCount() != 2 {
		t.Fatal("palette captured empty startup editor")
	}
	w.activate(modeDesign)
	clear()
	if ed.CursorCount() != 2 {
		t.Fatal("palette touched hidden editor")
	}
	w.activate(modeEdit)
	clear()
	if ed.CursorCount() != 1 {
		t.Fatal("palette clear failed")
	}
}

func TestIndentMenuPaletteDocumentGuardAndUndo(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	path := writeFile(t, t.TempDir(), "indent.txt", "abc\nxyz")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetInsertSpaces(true)
	ed.SetTabSize(2)
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	for _, id := range []string{"editor.indentLines", "editor.outdentLines"} {
		b := menuCommandButton(m, id)
		if b == nil || b.Icon() == nil || !b.IsEnabled() {
			t.Fatal("missing indentation menu", id)
		}
	}
	b := menuCommandButton(m, "editor.indentLines")
	b.Action().Trigger(b)
	if ed.Text() != "  abc\nxyz" {
		t.Fatal("menu indent", ed.Text())
	}
	ed.Undo()
	if ed.Text() != "abc\nxyz" {
		t.Fatal("menu undo")
	}
	registerPaletteCommands(w.tabs, w.canvas)
	for _, name := range []string{"Indent Lines", "Outdent Lines"} {
		found := false
		for _, c := range paletteCommands {
			if c.Name == i18n.T(name) {
				found = true
				c.Fn()
				break
			}
		}
		if !found {
			t.Fatal("missing palette", name)
		}
	}
	if ed.Text() != "abc\nxyz" {
		t.Fatal("palette indent/outdent", ed.Text())
	}
	other := writeFile(t, t.TempDir(), "other.txt", "other")
	openFileInEditor(w.tabs, other)
	b.Action().Trigger(b)
	if activeEditor(w.tabs).Text() != "other" || ed.Text() != "abc\nxyz" {
		t.Fatal("stale menu edited another document")
	}
	w.activate(modeDesign)
	b.Action().Trigger(b)
	if activeEditor(w.tabs).Text() != "other" {
		t.Fatal("hidden edit")
	}
}

func TestMoveLinesMenuPaletteBoundaryAndDocumentGuard(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	first := writeFile(t, t.TempDir(), "lines.txt", "one\ntwo\nthree\n")
	openFileInEditor(w.tabs, first)
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	up, down := menuCommandButton(menu, "editor.moveLinesUp"), menuCommandButton(menu, "editor.moveLinesDown")
	if up == nil || down == nil || up.Icon() == nil || down.Icon() == nil || up.IsEnabled() || !down.IsEnabled() {
		t.Fatal("missing commands/icons/boundary state")
	}
	down.Action().Trigger(down)
	if e.Text() != "two\none\nthree\n" || e.CursorLine() != 1 {
		t.Fatal("menu move", e.Text())
	}
	e.Undo()
	if e.Text() != "one\ntwo\nthree\n" || e.CursorLine() != 0 {
		t.Fatal("menu undo")
	}
	registerPaletteCommands(w.tabs, w.canvas)
	var moveUp, moveDown func()
	for _, c := range paletteCommands {
		if c.Name == i18n.T("Move Lines Up") {
			moveUp = c.Fn
		}
		if c.Name == i18n.T("Move Lines Down") {
			moveDown = c.Fn
		}
	}
	if moveUp == nil || moveDown == nil {
		t.Fatal("palette missing")
	}
	moveDown()
	moveDown()
	if e.CursorLine() != 2 || e.CanMoveLinesDown() {
		t.Fatal("palette EOF")
	}
	moveDown()
	if e.Text() != "two\nthree\none\n" {
		t.Fatal("boundary modified text")
	}
	moveUp()
	if e.CursorLine() != 1 {
		t.Fatal("palette up")
	}
	w.activate(modeDesign)
	before := e.Text()
	moveDown()
	if e.Text() != before {
		t.Fatal("hidden editor changed")
	}
	w.activate(modeEdit)
	second := writeFile(t, t.TempDir(), "other.txt", "x\ny\n")
	openFileInEditor(w.tabs, second)
	other := activeEditor(w.tabs)
	down.Action().Trigger(down)
	if e.Text() != before || other.Text() != "x\ny\n" {
		t.Fatal("stale menu changed document")
	}
	moveDown()
	if other.Text() != "y\nx\n" {
		t.Fatal("palette captured old editor")
	}
}

func TestDuplicateDeleteLinesMenuPaletteAndDocumentGuards(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	first := writeFile(t, t.TempDir(), "lines.txt", "a\nb\n")
	openFileInEditor(w.tabs, first)
	w.activate(modeEdit)
	e := activeEditor(w.tabs)
	menu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	dup, del := menuCommandButton(menu, "editor.duplicateLines"), menuCommandButton(menu, "editor.deleteLines")
	if dup == nil || del == nil || dup.Icon() == nil || del.Icon() == nil || !dup.IsEnabled() || !del.IsEnabled() {
		t.Fatal("menu commands/icons")
	}
	dup.Action().Trigger(dup)
	if e.Text() != "a\na\nb\n" {
		t.Fatal("menu copy")
	}
	e.Undo()
	registerPaletteCommands(w.tabs, w.canvas)
	var copy, remove func()
	for _, c := range paletteCommands {
		if c.Name == i18n.T("Duplicate Lines") {
			copy = c.Fn
		}
		if c.Name == i18n.T("Delete Lines") {
			remove = c.Fn
		}
	}
	if copy == nil || remove == nil {
		t.Fatal("palette missing")
	}
	remove()
	if e.Text() != "b\n" {
		t.Fatal("palette delete")
	}
	e.Undo()
	w.activate(modeDesign)
	copy()
	remove()
	if e.Text() != "a\nb\n" {
		t.Fatal("hidden edit")
	}
	w.activate(modeEdit)
	second := writeFile(t, t.TempDir(), "empty.txt", "")
	openFileInEditor(w.tabs, second)
	other := activeEditor(w.tabs)
	dup.Action().Trigger(dup)
	del.Action().Trigger(del)
	if other.Text() != "" || e.Text() != "a\nb\n" {
		t.Fatal("stale menu")
	}
	emptyMenu := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(emptyMenu)
	if menuCommandButton(emptyMenu, "editor.deleteLines").IsEnabled() || !menuCommandButton(emptyMenu, "editor.duplicateLines").IsEnabled() {
		t.Fatal("empty boundary")
	}
	copy()
	if other.Text() != "\n" {
		t.Fatal("current palette document")
	}
	remove()
	if other.Text() != "" {
		t.Fatal("delete current")
	}
}
