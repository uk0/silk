package ged

// Reviewer repro for the resize-gesture re-parent defect. Drop into ged/ and
// run: go test ./ged/ -run TestResizeDragDoesNotReparent -count=1
// Fails on worktree-wf_d7595f41-71e-1 @ 367375f:
//   resize gesture re-parented the widget: parent = ged.FakeWidget, want scene root

import (
	"testing"
	"time"

	"github.com/uk0/silk/graph"
)

func TestResizeDragDoesNotReparent(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()
	scene.SetSize(200, 150)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	btn.SetBounds(10, 10, 25, 7)
	btn.SetParent(scene)

	vbox, err := NewFakeWidgetFromFactory("gui.VBox")
	if err != nil {
		t.Fatalf("create VBox: %v", err)
	}
	vbox.SetBounds(60, 10, 80, 60)
	vbox.SetParent(scene)

	view.Selection().Add(btn)

	// The press point sits on the bottom-right resize handle (HANDLE_SIZE 3.2mm
	// -> 1.6mm radius reaches inside the item) AND inside the button's bounds,
	// so DecorPart claims the press but FindItemAt still hits the button.
	if decor, handle := view.FindHandleAt(34.5, 16.5); decor == nil || handle == 0 {
		t.Fatalf("press point is not on a handle (decor=%v handle=%d)", decor, handle)
	}
	if hit := scene.FindItemAt(34.5, 16.5, graph.TraversalCond_SelectableAndMoveable); hit != graph.IItem(btn) {
		t.Fatalf("press point does not hit the button (hit=%v)", hit)
	}

	lastClickTime = time.Time{}
	px, py := view.MapFromScene(34.5, 16.5)
	qx, qy := view.MapFromScene(100, 40) // cursor ends over the VBox
	view.OnLeftDown(px, py)
	view.OnMouseMove(qx, qy)
	view.OnLeftUp(qx, qy)

	if btn.Parent() != graph.IItem(scene) {
		t.Errorf("resize gesture re-parented the widget: parent = %v, want scene root", btn.Parent())
	}
}
