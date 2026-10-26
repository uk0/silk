package gui

import (
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"slices"
	"strings"
	"unicode/utf8"
)

// Menu creation never reads the clipboard, copies the document or asks LSP.
// Labels are translation keys; shortcut text describes the real built-in chord.
func (e *CodeEditor) contextMenuItems(word string) []editorContextMenuItem {
	k := CodeEditorShortcuts()
	renameHint := k.RenameSymbol
	if e.symbolActions.RenameSymbol != nil {
		renameHint = e.symbolActions.RenameShortcut
	}
	hasWord := word != ""
	items := []editorContextMenuItem{
		{Label: "Undo", Icon: "edit-undo", Shortcut: k.Undo, Enabled: e.CanUndo(), Action: e.Undo},
		{Label: "Redo", Icon: "edit-redo", Shortcut: k.Redo, Enabled: e.CanRedo(), Action: e.Redo},
		{Separator: true},
		{Label: "Cut", Icon: "edit-cut", Shortcut: k.Cut, Enabled: e.CanCut(), Action: e.editorCut},
		{Label: "Copy", Icon: "edit-copy", Shortcut: k.Copy, Enabled: e.CanCopy(), Action: e.editorCopy},
		{Label: "Paste", Icon: "edit-paste", Shortcut: k.Paste, Enabled: e.CanPaste(), Action: e.Paste},
		{Label: "Delete", Icon: "line-delete", Enabled: e.CanDeleteSelectedText(), Action: func() { e.DeleteSelectedText() }},
		{Label: "Select All", Icon: "edit-select-all", Shortcut: k.SelectAll, Enabled: e.CanSelectAll(), Action: e.editorSelectAll},
		{Separator: true},
		{Label: "Add Cursor Above", Icon: "cursor-add-above", Shortcut: k.AddCursorAbove, Enabled: e.CanAddCursorAbove(), Action: func() { e.AddCursorAbove() }},
		{Label: "Add Cursor Below", Icon: "cursor-add-below", Shortcut: k.AddCursorBelow, Enabled: e.CanAddCursorBelow(), Action: func() { e.AddCursorBelow() }},
		{Label: "Keep Primary Cursor", Icon: "cursor-primary", Shortcut: k.KeepPrimaryCursor, Enabled: e.CursorCount() > 1, Action: func() { e.KeepPrimaryCursor() }},
		{Separator: true},
		{Label: "Rename Symbol", Icon: "edit", Shortcut: renameHint, Enabled: hasWord && !e.readOnly, Action: e.RequestRenameSymbol},
		{Label: "Go to Definition", Icon: "navigate-forward", Shortcut: k.GoToDefinition, Enabled: hasWord, Action: e.RequestGoToDefinition},
		{Label: "Find References", Icon: "references", Shortcut: k.FindReferences, Enabled: hasWord, Action: e.RequestFindReferences},
		{Label: "Go to Type Definition", Icon: "type-hierarchy", Shortcut: k.GoToTypeDefinition, Enabled: hasWord && e.symbolActions.GoToTypeDefinition != nil, Action: e.RequestGoToTypeDefinition},
		{Label: "Go to Implementation", Icon: "call-hierarchy", Shortcut: k.GoToImplementation, Enabled: hasWord && e.symbolActions.GoToImplementation != nil, Action: e.RequestGoToImplementation},
	}
	if !textContextAvailable(e.Self()) {
		for i := range items {
			items[i].Enabled = false
		}
	}
	return items
}

// CanDeleteSelectedText is a constant-time, clipboard-free state query.
// Unlike DeleteLines, this command does nothing when there is no selection.
func (e *CodeEditor) CanDeleteSelectedText() bool {
	return e.IsEnabled() && !e.readOnly && e.hasSelection && (e.selStartLine != e.selEndLine || e.selStartCol != e.selEndCol)
}

// DeleteSelectedText removes only the selection in one undoable edit. It keeps
// byte-exact line endings, does not write the clipboard and returns false for
// a disabled/read-only editor or an empty selection. UI-thread-only.
func (e *CodeEditor) DeleteSelectedText() bool {
	if !e.CanDeleteSelectedText() {
		return false
	}
	e.breakTypingUndoGroup()
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	selection := e.selectionSnapshot()
	sl, sc, _, _ := e.selectionRange()
	edit := editorByteEdit{line: sl, column: textPositionInLine(e.lines[sl], sc).ByteColumn, before: strings.Clone(e.SelectedText())}
	e.prepareCaretCommand()
	e.CancelPendingPaste()
	p := e.replaceByteRange(edit.line, edit.column, len(edit.before), "")
	after := []cursorPos{p}
	e.restoreEditCursors(after)
	e.clearSelection()
	e.pushUndo(editAction{kind: 4, group: []editorByteEdit{edit}, beforeCursors: before, afterCursors: after, selection: &editorSelectionHistory{selection, e.selectionSnapshot()}})
	e.rebuildText()
	e.ensureCursorVisible()
	e.Update()
	return true
}

// BuildContextMenu supports standalone SDK hosts through IContextMenuProvider.
// Commands belong to the captured document/caret/owner, never whichever editor
// happens to be active later. Menu hide precedes Button's action dispatch, so
// visibility of the popup itself is deliberately not an activation condition.
func (e *CodeEditor) BuildContextMenu(m *Menu, _, _ float64) {
	e.contextGeneration++
	generation, revision := e.contextGeneration, e.TextRevision()
	parent, owner := e.Parent(), e.OwnerWindow()
	path, readonly := e.FilePath(), e.readOnly
	primary := cursorPos{e.cursorLine, e.cursorCol}
	selection := e.selectionSnapshot()
	extra := append([]cursorPos(nil), e.additionalCursors...)
	valid := func() bool {
		if generation != e.contextGeneration || revision != e.TextRevision() || e.Parent() != parent || e.OwnerWindow() != owner {
			return false
		}
		if e.FilePath() != path || e.readOnly != readonly || !textContextAvailable(e.Self()) {
			return false
		}
		if primary != (cursorPos{e.cursorLine, e.cursorCol}) || selection != e.selectionSnapshot() {
			return false
		}
		return slices.Equal(extra, e.additionalCursors)
	}

	word := ""
	if e.contextWordPresent() {
		word = "symbol"
	}
	for _, it := range e.contextMenuItems(word) {
		if it.Separator {
			m.AddSeparator()
			continue
		}
		b := m.AddButton1(i18n.T(it.Label), paint.LoadIcon(it.Icon))
		b.SetShortcutText(it.Shortcut)
		b.SetEnabled(it.Enabled)
		if !it.Enabled {
			continue
		}
		action := it.Action
		b.Action().BindFunc0(func() {
			if !valid() {
				return
			}
			e.SetFocus()
			// Focus callbacks are host code and can replace the document synchronously.
			if valid() {
				action()
			}
		})
	}
}

var showEditorContextPopup = func(m *Menu, x, y float64) { m.ShowAsPopup(x, y, true) }

func (e *CodeEditor) showEditorContextMenu(x, y float64) {
	if !textContextAvailable(e.Self()) {
		return
	}
	e.breakTypingUndoGroup()
	e.DismissLanguageHints()
	if e.contextMenu == nil {
		e.contextMenu = NewPopupMenu()
		e.contextMenu.clearOnHide = true
		e.contextMenu.SetParent(e.Self())
	}
	m := e.contextMenu
	m.Hide()
	m.clearOwnedContextItems()
	owner := e.OwnerWindow()
	contextMenuRegistryMu.Lock()
	builder := contextMenuRegistry[e.Self()]
	contextMenuRegistryMu.Unlock()
	if builder != nil {
		builder(m, x, y)
	} else if provider, ok := e.Self().(IContextMenuProvider); ok {
		provider.BuildContextMenu(m, x, y)
	}
	if m.Parent() != e.Self() || e.OwnerWindow() != owner || len(m.Items()) == 0 || !textContextAvailable(e.Self()) {
		m.clearOwnedContextItems()
		return
	}
	gx, gy := e.MapToGlobal(x, y)
	showEditorContextPopup(m, gx, gy)
	e.Update()
}

func (e *CodeEditor) closeEditorContextMenu() {
	e.contextDown = false
	e.contextGeneration++
	if e.contextMenu != nil {
		e.contextMenu.Hide()
		e.contextMenu.clearOwnedContextItems()
	}
}

// Open on release, matching other SDK context menus. The press only establishes
// the target; drag-away/cancel does not open a popup or execute its first row.
func (e *CodeEditor) OnRightDown(x, y float64) {
	e.contextDown = false
	if !textContextAvailable(e.Self()) || x < 0 || y < 0 || x >= e.Width() || y >= e.Height() {
		return
	}
	e.breakTypingUndoGroup()
	e.DismissLanguageHints()
	e.SetFocus()
	if !textContextAvailable(e.Self()) {
		return
	}
	e.repositionCaretForRightClick(x, y)
	e.contextDown = true
}
func (e *CodeEditor) OnRightUp(x, y float64) {
	pressed := e.contextDown
	e.contextDown = false
	if pressed && x >= 0 && y >= 0 && x < e.Width() && y < e.Height() {
		e.showEditorContextMenu(x, y)
	}
}

func (e *CodeEditor) editorContextKey(key int, repeat bool) bool {
	if key != KeyF10 {
		return false
	}
	return e.editorContextKeyWithMods(key, repeat, currentMods())
}
func (e *CodeEditor) editorContextKeyWithMods(key int, repeat bool, mods uint8) bool {
	if key != KeyF10 || mods != ModShift || !e.HasFocus() || !textContextAvailable(e.Self()) {
		return false
	}
	if !repeat {
		caret := e.InputMethodCursorRect()
		e.showEditorContextMenu(caret.X, caret.Bottom())
	}
	return true
}

// A menu needs identifier availability, not the entire word. Inspect at most
// the cursor prefix and adjacent runes; never allocate a rune copy of a huge
// line or retain a source substring in the menu's callbacks.
func (e *CodeEditor) contextWordPresent() bool {
	if len(e.lines) == 0 {
		return false
	}
	line := e.lines[max(0, min(e.cursorLine, len(e.lines)-1))]
	p := textPositionInLine(line, max(0, e.cursorCol))
	if p.ByteColumn < len(line) {
		r, _ := utf8.DecodeRuneInString(line[p.ByteColumn:])
		if isIdentPart(r) {
			return true
		}
	}
	return p.HasPreviousRune && isIdentPart(p.PreviousRune)
}

func (e *CodeEditor) promptRenameSymbol(word string) {
	revision, path := e.TextRevision(), e.FilePath()
	line, col := e.CursorLine(), e.CursorCol()
	parent, owner := e.Parent(), e.OwnerWindow()
	name, ok := showRenameSymbolInput(e.Self(), nil, i18n.T("Rename Symbol"), i18n.T("New name:"), word, GoIdentifierValidator())
	if !ok || name == word {
		return
	}
	// A modal loop can process host callbacks. Never retarget a changed editor.
	if e.readOnly || !e.IsEnabled() || e.TextRevision() != revision || e.FilePath() != path || e.CursorLine() != line || e.CursorCol() != col || e.Parent() != parent || e.OwnerWindow() != owner {
		return
	}
	if _, _, err := e.RenameSymbolAtCursor(name); err != nil {
		ShowMessageBox(e.Self(), nil, i18n.T("Rename Symbol"), err.Error(), []string{"@ok"})
	}
}
