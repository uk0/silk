//go:build windows

package gui

import (
	"testing"

	"github.com/uk0/silk/geom"
)

func TestWindowsInlineGeometryQueuesClippedPartialDamage(t *testing.T) {
	isolateWindowsRenderScheduler(t)
	root := windowsRenderFixture(1, 300, 200)
	child := &Widget{}
	child.Init(child)
	child.SetBounds(25, 40, 48, 32)
	child.SetParent(root.widget)
	root.repaintPending, root.repaintFull, root.repaintRect = false, false, geom.Rect{}
	child.SetPos(280, 188)
	want := geom.Rect{X: 25, Y: 40, Width: 275, Height: 160}
	if !root.repaintPending || root.repaintFull || root.repaintRect != want || root.paintQueued || renderTimer != 0 {
		t.Fatalf("wrong Windows partial damage: got %v, full=%t, pending=%t, want %v", root.repaintRect, root.repaintFull, root.repaintPending, want)
	}
}
