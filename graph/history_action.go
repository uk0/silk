package graph

import "github.com/uk0/silk/gui"

// Keep the stack's text/enabled/revision state, but route execution through a
// specialized view's document lifecycle. Binding directly to UndoStack used to
// skip pending edits, property refresh and design-change notifications in ged.
// Plain GraphView hosts still use the stack action unchanged.
type viewHistoryAction struct {
	gui.IAction
	view  *GraphView
	scene IScene
	redo  bool
}

func (a *viewHistoryAction) Trigger(sender interface{}) {
	if !a.IsEnabled() || a.view.Scene() != a.scene {
		return
	}
	if a.redo {
		if view, ok := a.view.Self().(interface{ Redo() }); ok {
			view.Redo()
			return
		}
	} else {
		if view, ok := a.view.Self().(interface{ Undo() }); ok {
			view.Undo()
			return
		}
	}
	a.IAction.Trigger(sender)
}
