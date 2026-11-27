package graph

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

// MoveGestureCommandBuilder is an optional GraphView owner interface. It
// composes a completed pointer drag before the graph applies any geometry or
// touches history. The supplied coordinates are the actual release in scene
// units. Return nil to cancel or move to keep the standard behavior.
type MoveGestureCommandBuilder interface {
	BuildMoveGestureCommand(move *MoveCommand, sceneX, sceneY float64) gui.ICommand
}

type MovePart struct {
	Part
	cond TraversalCond
	item IItem
}

func NewMovePart() *MovePart {
	p := new(MovePart)
	p.cond = TraversalCond_SelectableAndMoveable
	p.Init(p)
	return p
}

func (this *MovePart) OnLeftDragStart(x, y float64) {
	scene := this.Scene()
	downX, downY := this.DownPos()

	this.item = scene.FindItemAt(downX, downY, this.cond)

	// The scene is the document surface, not a movable child. In designers
	// it remains selectable for form properties and resize handles, but a drag
	// on its empty body must fall through to MarqueePart (next in ArrowTool).
	if this.item == nil || this.item == scene {
		this.item = nil
		return
	}

	selection := this.View().Selection()
	if !selection.Contains(this.item) {
		selection.Clear()
		selection.Add(this.item)
	}
	this.Activate()
	this.View().Update()
}

func (this *MovePart) OnMouseMove(x, y float64) {
	this.View().Update()
}

func (this *MovePart) OnLeftUp(x, y float64) {
	dx := this.TrackRect().Width
	dy := this.TrackRect().Height
	if dx != 0 || dy != 0 {
		move := this.View().Selection().GenerateMoveCommand(dx, dy)
		if move != nil {
			var cmd gui.ICommand = move
			if owner, ok := this.View().Self().(MoveGestureCommandBuilder); ok {
				cmd = owner.BuildMoveGestureCommand(move, x, y)
			}
			if cmd != nil {
				this.Scene().PushCommand(cmd)
			}
		}
	}
	this.View().Update()
	this.item = nil
}

func (this *MovePart) OnDraw(g paint.Painter) {

}
