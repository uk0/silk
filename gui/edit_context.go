package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"unicode/utf8"
)

// Availability is a state query, never a native clipboard read. Edit commands
// affect selected text only (unlike CodeEditor's no-selection line commands).
func (e *Edit) CanCopy() bool      { return e.IsEnabled() && e.sel0 != e.sel1 }
func (e *Edit) CanCut() bool       { return !e.readonly && e.CanCopy() }
func (e *Edit) CanPaste() bool     { return e.IsEnabled() && !e.readonly }
func (e *Edit) CanSelectAll() bool { return e.IsEnabled() && e.RunesCount() > 0 }

// CopySelection/Cut return native write errors. Cut preserves the buffer and
// history on failure; Copy retains its existing void signature for SDK callers.
func (e *Edit) CopySelection() error { return e.copyEditSelection(false, writeEditorClipboard) }
func (e *Edit) Cut() error           { return e.copyEditSelection(true, writeEditorClipboard) }
func (e *Edit) Paste()               { e.paste() }
func (e *Edit) copyEditSelection(cut bool, write func(string) error) error {
	if cut && !e.CanCut() || !cut && !e.CanCopy() {
		return nil
	}
	a, b := e.Selection()
	a = max(0, min(a, e.RunesCount()))
	b = max(a, min(b, e.RunesCount()))
	size := 0
	for _, r := range e.runesNoSentinel()[a:b] {
		size += utf8.RuneLen(r)
		if size > maxClipboardTextBytes {
			return ErrClipboardTooLarge
		}
	}
	s := e.SelectionText()
	if err := clipboardTextErr(s); err != nil {
		return err
	}
	revision, anchor, caret := e.editRevision, e.sel0, e.sel1
	if err := write(s); err != nil {
		return err
	}
	// A clipboard owner may re-enter native callbacks while rendering data.
	if cut && e.CanCut() && e.editRevision == revision && e.sel0 == anchor && e.sel1 == caret {
		e.replace(a, b, "")
		e.caretXSaved = false
		e.emitEdited()
	}
	return nil
}
func (e *Edit) editClipboardFailed(err error) {
	if err == nil {
		return
	}
	core.Warn("clipboard edit: ", err)
	if e.OwnerWindow() != nil {
		ShowToast(e.Self(), i18n.Tf("Clipboard operation failed: %v", err), 3500, ToastError)
	}
}

// HandleShortcut keeps field Undo/Redo local even if the host has document
// history bindings. Empty or read-only history must not fall through and undo
// another document. Other application shortcuts retain their existing priority.
func (e *Edit) HandleShortcut(key int, mods uint8) bool {
	if !e.IsEnabled() || mods&ModAction == 0 || mods&ModAlt != 0 {
		return false
	}
	switch key {
	case 'Z':
		if mods&ModShift != 0 {
			e.Redo()
		} else {
			e.Undo()
		}
		return true
	case 'Y':
		if mods&ModShift == 0 {
			e.Redo()
			return true
		}
	}
	return false
}

func (e *Edit) editMenuItems() []editorContextMenuItem {
	keys := CodeEditorShortcuts()
	return []editorContextMenuItem{
		{Label: "Undo", Icon: "edit-undo", Shortcut: keys.Undo, Enabled: e.CanUndo(), Action: e.Undo},
		{Label: "Redo", Icon: "edit-redo", Shortcut: keys.Redo, Enabled: e.CanRedo(), Action: e.Redo},
		{Separator: true},
		{Label: "Cut", Icon: "edit-cut", Shortcut: keys.Cut, Enabled: e.CanCut(), Action: e.cut},
		{Label: "Copy", Icon: "edit-copy", Shortcut: keys.Copy, Enabled: e.CanCopy(), Action: e.Copy},
		{Label: "Paste", Icon: "edit-paste", Shortcut: keys.Paste, Enabled: e.CanPaste(), Action: e.Paste},
		{Label: "Delete", Icon: "line-delete", Enabled: e.CanCut(), Action: func() {
			if e.CanCut() {
				e.replace(e.sel0, e.sel1, "")
				e.emitEdited()
			}
		}},
		{Separator: true},
		{Label: "Select All", Icon: "edit-select-all", Shortcut: keys.SelectAll, Enabled: e.CanSelectAll(), Action: func() {
			if e.CanSelectAll() {
				e.SelectAll()
			}
		}},
	}
}
func editContextAvailable(e *Edit) bool {
	return textContextAvailable(e.Self())
}

// BuildContextMenu also supports SDK hosts that explicitly use TryContextMenu.
// Commands capture a revision/selection/owner, not merely a pointer to an Edit.
func (e *Edit) BuildContextMenu(m *Menu, _, _ float64) {
	revision, a, b, owner := e.editRevision, e.sel0, e.sel1, e.OwnerWindow()
	for _, it := range e.editMenuItems() {
		if it.Separator {
			m.AddSeparator()
			continue
		}
		button := m.AddButton1(i18n.T(it.Label), paint.LoadIcon(it.Icon))
		button.SetShortcutText(it.Shortcut)
		button.SetEnabled(it.Enabled)
		if !it.Enabled {
			continue
		}
		fn := it.Action
		button.Action().BindFunc0(func() {
			if !editContextAvailable(e) || e.OwnerWindow() != owner || e.editRevision != revision || e.sel0 != a || e.sel1 != b {
				return
			}
			e.SetFocus()
			fn()
		})
	}
}

var showEditContextPopup = func(m *Menu, x, y float64) { m.ShowAsPopup(x, y, true) }

// The popup temporarily owns keyboard focus, but the selected text is still
// the action target. Keep its highlight without drawing an unfocused caret.
func (e *Edit) ownsVisibleContextMenu() bool {
	return e.contextMenu != nil && e.contextMenu.Parent() == e.Self() && e.contextMenu.IsVisible()
}

func (e *Edit) showEditContextMenu(x, y float64) {
	if !editContextAvailable(e) {
		return
	}
	if e.contextMenu == nil {
		e.contextMenu = NewPopupMenu()
		e.contextMenu.clearOnHide = true
		e.contextMenu.SetParent(e.Self())
	}
	m := e.contextMenu
	m.Hide()
	m.clearOwnedContextItems()
	contextMenuRegistryMu.Lock()
	builder := contextMenuRegistry[e.Self()]
	contextMenuRegistryMu.Unlock()
	if builder != nil {
		builder(m, x, y)
	} else if provider, ok := e.Self().(IContextMenuProvider); ok {
		provider.BuildContextMenu(m, x, y)
	}
	if len(m.Items()) == 0 || !editContextAvailable(e) {
		m.clearOwnedContextItems()
		return
	}
	gx, gy := e.MapToGlobal(x, y)
	showEditContextPopup(m, gx, gy)
	e.Update()
}
func (e *Edit) OnRightDown(x, y float64) {
	e.contextDown = false
	if !editContextAvailable(e) || x < 0 || y < 0 || x >= e.Width() || y >= e.Height() {
		return
	}
	e.SetFocus()
	// Preserve the selected range even when the context click is elsewhere in
	// the field. Without a selection, place the caret at the clicked glyph.
	if e.sel0 == e.sel1 {
		e.SetCaretPos(e.PointToPos(x-e.padding.L+e.ScrollX(), y-e.padding.T+e.ScrollY()*e.RowHeight()))
	}
	e.contextDown = true
}
func (e *Edit) OnRightUp(x, y float64) {
	pressed := e.contextDown
	e.contextDown = false
	if pressed && x >= 0 && y >= 0 && x < e.Width() && y < e.Height() {
		e.showEditContextMenu(x, y)
	}
}
func (e *Edit) editContextKey(key int, repeat bool) bool {
	if key != KeyF10 {
		return false
	}
	return e.editContextKeyWithMods(key, repeat, currentMods())
}
func (e *Edit) editContextKeyWithMods(key int, repeat bool, mods uint8) bool {
	if key != KeyF10 || mods != ModShift || !e.HasFocus() || !editContextAvailable(e) {
		return false
	}
	if !repeat {
		caret := e.InputMethodCursorRect()
		e.showEditContextMenu(caret.X, caret.Bottom())
	}
	return true
}
func (e *Edit) OnSystemKeyDown(key int, repeat bool) bool { return e.editContextKey(key, repeat) }
