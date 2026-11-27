package ged

import (
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

var _ graph.MoveGestureCommandBuilder = (*GedView)(nil)

// BuildMoveGestureCommand uses the release point, not the last hover. Neither
// motion nor a release outside the form can mutate the document or history.
func (view *GedView) BuildMoveGestureCommand(move *graph.MoveCommand, x, y float64) gui.ICommand {
	scene := view.Scene()
	if move == nil || scene == nil || !finiteDropCoordinate(x) || !finiteDropCoordinate(y) {
		return nil
	}
	page := scene.MapRectToScene1(scene.Bounds1())
	if !pointInDropRect(page, x, y) {
		return nil
	}
	if view.showGuides {
		px, py := view.MapFromScene(x, y)
		if px < rulerThicknessPx || py < rulerThicknessPx {
			return nil
		}
	}
	sel := view.Selection()
	target := nearestDropContainer(scene.FindItemAt(x, y, nil), sel.Contains)
	if layout := managedLayoutTarget(target); layout != nil {
		item := singleTransferItem(sel)
		plan := view.layoutTransferAt(item, layout, x, y)
		if !plan.valid() {
			view.showStatus(i18n.T(plan.reject))
			return nil
		}
		if !view.canCommitTabularDrop() {
			return nil
		}
		if nearestDropContainer(scene.FindItemAt(x, y, nil), sel.Contains) != layout || item != singleTransferItem(sel) {
			return nil
		}
		return view.buildLayoutTransfer(item, view.layoutTransferAt(item, layout, x, y))
	}
	return &canvasMoveGesture{
		move:      move,
		selection: sel,
		prepare: func() (gui.ICommand, bool) {
			view.snapSelectionToGrid()
			parent := graph.IItem(scene)
			if target != nil && target.Scene() == scene {
				parent = target
			}
			reparent := sel.GenerateReparentCommand(parent)
			items := sel.ItemList()
			if reparent != nil {
				return groupWithPark(reparent, &parkStep{items: items, parent: parent, keepLanding: true}), true
			}
			// Snapping across a clipping edge can hide a control even when
			// its owner did not change. Keep the correction in this move.
			if park := parkCanvasLanding(items, parent); park != nil {
				return park, false
			}
			return nil, false
		},
	}
}

type canvasMoveGesture struct {
	move      *graph.MoveCommand
	selection *graph.Selection
	prepare   func() (gui.ICommand, bool)
	tail      gui.ICommand
	reparents bool
}

func (cmd *canvasMoveGesture) Text() string { return cmd.move.Text() }

func (cmd *canvasMoveGesture) Redo() {
	cmd.move.Redo()
	if cmd.prepare != nil {
		// Snap exactly once, before capturing the reparent's old coordinates.
		// MoveCommand.Undo records those snapped positions for later replay.
		cmd.tail, cmd.reparents = cmd.prepare()
		cmd.prepare = nil
	}
	if cmd.reparents {
		cmd.withSelection(cmd.tail.Redo)
	} else if cmd.tail != nil {
		cmd.tail.Redo()
	}
}

func (cmd *canvasMoveGesture) Undo() {
	undo := func() {
		if cmd.tail != nil {
			cmd.tail.Undo()
		}
		cmd.move.Undo()
	}
	if cmd.reparents {
		cmd.withSelection(undo)
	} else {
		undo()
	}
}

func (cmd *canvasMoveGesture) withSelection(apply func()) {
	// Reparent detaches before attaching. Preserve whichever items the user
	// has selected for this application, without reselecting stale history.
	selected := cmd.selection.ItemList()
	apply()
	cmd.selection.Clear()
	cmd.selection.AddMulti(selected)
}
