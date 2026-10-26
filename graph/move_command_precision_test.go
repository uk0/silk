package graph

import "testing"

func TestPreciseMoveCommandKeepsExplicitTargets(t *testing.T) {
	a, b := NewRectItem(), NewRectItem()
	a.SetPos(-15.25, 20.75)
	b.SetPos(30.5, -10.125)
	cmd := NewMoveCommand()
	cmd.AddItem(a, -14.25, 20.75)
	cmd.AddItem(b, 31.5, -10.125)
	for i := 0; i < 5; i++ {
		cmd.Redo()
		if a.X() != -14.25 || a.Y() != 20.75 || b.X() != 31.5 || b.Y() != -10.125 {
			t.Fatal("command silently rounded explicit targets", a.Bounds1(), b.Bounds1())
		}
		cmd.Undo()
		if a.X() != -15.25 || a.Y() != 20.75 || b.X() != 30.5 || b.Y() != -10.125 {
			t.Fatal("undo changed original coordinates")
		}
	}
}
