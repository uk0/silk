package gui

import (
	"context"
	"fmt"
	"github.com/uk0/silk/i18n"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEditorContextStandardCommands(t *testing.T) {
	e := NewCodeEditor()
	defer e.CancelSymbolIndex()
	e.SetText("hello 中🙂\r\nnext  \n")
	items := e.contextMenuItems("")
	for _, name := range []string{"Undo", "Redo", "Cut", "Copy", "Paste", "Delete", "Select All", "Add Cursor Above", "Add Cursor Below", "Keep Primary Cursor", "Rename Symbol", "Go to Definition", "Find References", "Go to Type Definition", "Go to Implementation"} {
		it := findMenuItem(items, name)
		if it == nil || it.Icon == "" {
			t.Errorf("missing canonical command/icon %q", name)
		}
	}
	if len(items) < 3 || items[0].Label != "Undo" || items[1].Label != "Redo" || !items[2].Separator {
		t.Error("history must lead context menu")
	}
}

func editorContextHarness(t *testing.T) (*CodeEditor, **Menu) {
	t.Helper()
	e := NewCodeEditor()
	e.SetSize(600, 400)
	e.SetText("alpha 中🙂\r\nbeta\t \nlast  ")
	old, oldFocus, locale := showEditorContextPopup, focusWidget, i18n.Locale()
	i18n.SetLocale("en")
	var shown *Menu
	showEditorContextPopup = func(m *Menu, _, _ float64) { shown = m }
	t.Cleanup(func() {
		e.closeEditorContextMenu()
		e.CancelSymbolIndex()
		showEditorContextPopup = old
		focusWidget = oldFocus
		i18n.SetLocale(locale)
	})
	return e, &shown
}
func editorMenuButton(t *testing.T, m *Menu, label string) *Button {
	t.Helper()
	for _, item := range m.Items() {
		if b, ok := item.(*Button); ok && b.Text() == i18n.T(label) {
			return b
		}
	}
	t.Fatalf("missing menu button %q", label)
	return nil
}

func TestEditorContextReuseReleaseKeyboardAndOverride(t *testing.T) {
	e, shown := editorContextHarness(t)
	e.SelectAll()
	selection := e.SelectedText()
	x, y := e.gutterW+20, e.topOffset()+5
	e.OnRightUp(x, y)
	if *shown != nil {
		t.Fatal("release without press opened menu")
	}
	e.OnRightDown(x, y)
	if *shown != nil {
		t.Fatal("press opened menu before release")
	}
	e.OnRightUp(x, y)
	m := *shown
	if m == nil || m.Parent() != e || len(m.Items()) != 18 || e.SelectedText() != selection {
		t.Fatal("menu ownership/selection")
	}
	for i := 0; i < 20; i++ {
		e.OnRightDown(x, y)
		e.OnRightUp(x, y)
		if *shown != m || len(m.Items()) != 18 {
			t.Fatal("popup or item growth")
		}
	}
	old := m.Items()[0]
	m.OnHide()
	if len(m.Items()) != 0 || old.Parent() != nil {
		t.Fatal("menu commands retained after hide")
	}
	*shown = nil
	e.OnRightDown(x, y)
	e.OnRightUp(-1, y)
	if *shown != nil {
		t.Fatal("drag-away opened menu")
	}
	e.SetFocus()
	if !e.editorContextKeyWithMods(KeyF10, false, ModShift) || *shown != m {
		t.Fatal("keyboard context menu")
	}
	*shown = nil
	if !e.editorContextKeyWithMods(KeyF10, true, ModShift) || *shown != nil {
		t.Fatal("keyboard repeat reopened menu")
	}
	for _, mods := range []uint8{0, ModAction, ModAlt, ModShift | ModAction} {
		if e.editorContextKeyWithMods(KeyF10, false, mods) {
			t.Fatal("claimed unrelated system chord")
		}
	}
	AttachContextMenu(e, func(m *Menu, _, _ float64) { m.AddButton1("Custom", nil) })
	t.Cleanup(func() { DetachContextMenu(e) })
	e.showEditorContextMenu(x, y)
	if len(m.Items()) != 1 || m.Items()[0].(*Button).Text() != "Custom" {
		t.Fatal("SDK menu override ignored")
	}
	AttachContextMenu(e, func(*Menu, float64, float64) {})
	*shown = nil
	e.showEditorContextMenu(x, y)
	if *shown != nil || len(m.Items()) != 0 {
		t.Fatal("empty popup shown")
	}
	DetachContextMenu(e)
	e.OnHide()
	if e.contextDown || len(m.Items()) != 0 {
		t.Fatal("editor hide retained popup")
	}
}

func TestEditorContextHistoryDeleteAndSingleDispatch(t *testing.T) {
	e, shown := editorContextHarness(t)
	original := e.Text()
	e.SelectAll()
	before := e.selectionSnapshot()
	primary := cursorPos{e.cursorLine, e.cursorCol}
	e.showEditorContextMenu(0, 0)
	m := *shown
	if editorMenuButton(t, m, "Undo").IsEnabled() || editorMenuButton(t, m, "Redo").IsEnabled() {
		t.Fatal("empty history enabled")
	}
	m.NakedWidget().hidden = false
	m.setHighlight(6)
	m.OnKeyDown(KeyEnter, false)
	if e.Text() != "" || len(e.undoStack) != 1 || len(m.Items()) != 0 {
		t.Fatal("delete not dispatched once after hide")
	}
	e.showEditorContextMenu(0, 0)
	m.NakedWidget().hidden = false
	if !editorMenuButton(t, m, "Undo").IsEnabled() {
		t.Fatal("undo missing after delete")
	}
	m.setHighlight(0)
	m.OnKeyDown(KeyEnter, false)
	if e.Text() != original || e.selectionSnapshot() != before || primary != (cursorPos{e.cursorLine, e.cursorCol}) || !e.CanRedo() {
		t.Fatal("undo lost source/selection/caret")
	}
	e.showEditorContextMenu(0, 0)
	m.NakedWidget().hidden = false
	m.setHighlight(1)
	m.OnKeyDown(KeyEnter, false)
	if e.Text() != "" || e.CanRedo() || len(e.undoStack) != 1 {
		t.Fatal("redo lost delete transaction")
	}
	e.Undo()
	e.clearSelection()
	if e.DeleteSelectedText() || e.Text() != original {
		t.Fatal("Delete without selection removed a line")
	}
	e.SelectAll()
	e.SetReadOnly(true)
	if e.DeleteSelectedText() || findMenuItem(e.contextMenuItems("alpha"), "Delete").Enabled {
		t.Fatal("read-only delete")
	}
	e.SetReadOnly(false)
	e.SetEnabled(false)
	if e.DeleteSelectedText() {
		t.Fatal("disabled delete")
	}
	for _, it := range e.contextMenuItems("alpha") {
		if it.Enabled {
			t.Fatal("disabled context action", it.Label)
		}
	}
}

func TestEditorContextStaleCommandsDoNotRetarget(t *testing.T) {
	for name, mutate := range map[string]func(*CodeEditor){
		"document":  func(e *CodeEditor) { e.SetText("replacement") },
		"cursor":    func(e *CodeEditor) { e.cursorCol++ },
		"selection": func(e *CodeEditor) { e.clearSelection() },
		"carets":    func(e *CodeEditor) { e.additionalCursors = []cursorPos{{1, 0}} },
		"readonly":  func(e *CodeEditor) { e.SetReadOnly(true) },
		"disabled":  func(e *CodeEditor) { e.SetEnabled(false) },
		"hidden":    func(e *CodeEditor) { e.Hide() },
		"parent":    func(e *CodeEditor) { e.SetParent(NewForm()) },
		"file":      func(e *CodeEditor) { e.SetFilePath("replacement.go") },
		"hooks":     func(e *CodeEditor) { e.SetSymbolActions(CodeEditorSymbolActions{}) },
		"rebuilt":   func(e *CodeEditor) { e.BuildContextMenu(NewPopupMenu(), 0, 0) },
	} {
		t.Run(name, func(t *testing.T) {
			e, _ := editorContextHarness(t)
			e.SelectAll()
			m := NewPopupMenu()
			e.BuildContextMenu(m, 0, 0)
			action := editorMenuButton(t, m, "Delete").Action()
			mutate(e)
			text, rev := e.Text(), e.TextRevision()
			action.Trigger(nil)
			if e.Text() != text || e.TextRevision() != rev {
				t.Fatal("stale command changed target")
			}
		})
	}
}

func TestEditorContextLocaleIconsAndShortcutMetadata(t *testing.T) {
	e, _ := editorContextHarness(t)
	e.ScrollToLineCol(0, 2)
	keys := CodeEditorShortcuts()
	for _, locale := range []string{"en", "zh-CN", "zh-Hans", "en-US"} {
		i18n.SetLocale(locale)
		m := NewPopupMenu()
		e.BuildContextMenu(m, 0, 0)
		for _, it := range e.contextMenuItems("alpha") {
			if it.Separator {
				continue
			}
			b := editorMenuButton(t, m, it.Label)
			if b.Text() != i18n.T(it.Label) || b.Icon() == nil || b.ShortcutText() != it.Shortcut {
				t.Fatal("menu presentation", it.Label)
			}
			if strings.HasPrefix(locale, "zh") && b.Text() == it.Label {
				t.Fatal("untranslated SDK command", it.Label)
			}
		}
		if editorMenuButton(t, m, "Undo").ShortcutText() != keys.Undo || editorMenuButton(t, m, "Redo").ShortcutText() != keys.Redo {
			t.Fatal("history hints")
		}
		m.Clear()
	}
	i18n.SetLocale("en")
	if findMenuItem(e.contextMenuItems("alpha"), "Rename Symbol").Shortcut != keys.RenameSymbol {
		t.Fatal("SDK rename hint")
	}
	e.SetSymbolActions(CodeEditorSymbolActions{RenameSymbol: func(*CodeEditor) {}})
	if findMenuItem(e.contextMenuItems("alpha"), "Rename Symbol").Shortcut != "" {
		t.Fatal("unbound host hint advertised")
	}
	e.SetSymbolActions(CodeEditorSymbolActions{RenameSymbol: func(*CodeEditor) {}, RenameShortcut: "F2"})
	if findMenuItem(e.contextMenuItems("alpha"), "Rename Symbol").Shortcut != "F2" {
		t.Fatal("IDE rename override")
	}
}

func TestEditorContextWordProbeAndQueriesAreBounded(t *testing.T) {
	e, _ := editorContextHarness(t)
	for _, s := range []string{"", "alpha beta", "中🙂 word", "_foo.42", "\xffz\t", "é\u0301"} {
		e.SetText(s)
		for col := 0; col <= utf8.RuneCountInString(s)+1; col++ {
			e.cursorLine, e.cursorCol = 0, col
			got := e.contextWordPresent()
			want := e.wordAtCursor() != ""
			if got != want {
				t.Fatalf("word probe %q col %d", s, col)
			}
		}
	}
	e.SetText(strings.Repeat("a", 1<<20))
	e.SelectAll()
	e.clipboardReader = func(context.Context, func(string, error)) error { t.Fatal("menu read clipboard"); return nil }
	e.cursorCol = 0
	if n := testing.AllocsPerRun(20, func() { _ = e.contextWordPresent(); _ = e.CanDeleteSelectedText() }); n != 0 {
		t.Fatal("query allocated", n)
	}
	for _, it := range e.contextMenuItems("symbol") {
		if it.Label == "Paste" && !it.Enabled {
			t.Fatal("paste disabled without clipboard read")
		}
	}
}

func BenchmarkEditorContextMenuRows(b *testing.B) {
	for _, size := range []int{32, 1 << 20} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			e := NewCodeEditor()
			defer e.CancelSymbolIndex()
			e.SetText(strings.Repeat("a", size))
			e.cursorCol = 0
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				word := ""
				if e.contextWordPresent() {
					word = "symbol"
				}
				_ = e.contextMenuItems(word)
			}
		})
	}
}
