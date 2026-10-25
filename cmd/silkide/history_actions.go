package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// Menus capture document identity, never just a filename or mode. Long-lived
// toolbar/palette/shortcut commands resolve the visible target when invoked.
type documentHistoryTarget struct {
	editor *gui.CodeEditor
	scene  *ged.GedScene
}

func currentDocumentHistory(tabs *gui.TabWidget, canvas *ged.GedView) documentHistoryTarget {
	switch documentMode(tabs, canvas) {
	case modeEdit:
		return documentHistoryTarget{editor: activeEditor(tabs)}
	case modeDesign:
		if canvas != nil {
			return documentHistoryTarget{scene: canvas.GedScene()}
		}
	}
	return documentHistoryTarget{}
}
func (t documentHistoryTarget) available(redo bool) bool {
	if t.editor != nil {
		if redo {
			return t.editor.CanRedo()
		}
		return t.editor.CanUndo()
	}
	if t.scene != nil && t.scene.UndoStack() != nil {
		pending := t.scene.HasPendingEdits()
		if redo {
			return !pending && t.scene.UndoStack().CanRedo()
		}
		return pending || t.scene.UndoStack().CanUndo()
	}
	return false
}
func runHistoryMenu(tabs *gui.TabWidget, canvas *ged.GedView, target documentHistoryTarget, redo bool) {
	if currentDocumentHistory(tabs, canvas) == target {
		undoCurrentDocument(tabs, canvas, redo)
	}
}

type workbenchHistoryActions struct {
	undo, redo *gui.Button
	target     documentHistoryTarget
	stop       func()
}

func (w *workbench) closeHistoryActions() {
	if w.historyActions != nil && w.historyActions.stop != nil {
		w.historyActions.stop()
	}
	w.historyActions = nil
}
func (w *workbench) syncHistoryActions() {
	a := w.historyActions
	if a == nil {
		return
	}
	target := currentDocumentHistory(w.tabs, w.canvas)
	if target != a.target {
		if a.stop != nil {
			a.stop()
			a.stop = nil
		}
		a.target = target
		refresh := func() {
			if globalWorkbench == w && w.historyActions == a {
				w.syncHistoryActions()
			}
		}
		if target.editor != nil {
			a.stop = target.editor.SubscribeHistoryChanged(refresh)
		}
		if target.scene != nil {
			id := target.scene.AddDesignChangedCallback(refresh)
			a.stop = func() { target.scene.RemoveDesignChangedCallback(id) }
		}
	}
	for i, b := range []*gui.Button{a.undo, a.redo} {
		enabled := target.available(i == 1)
		if b.IsEnabled() != enabled || b.Action().IsEnabled() != enabled {
			b.SetEnabled(enabled)
			b.Action().SetEnabled(enabled)
			b.Update()
		}
	}
}
