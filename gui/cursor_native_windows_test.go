package gui

import "testing"

func TestNativeDragCursorLifecycle(t *testing.T) {
	for n := 0; n < 64; n++ {
		cursors := GenerateDropCursors(nil)
		for i, c := range cursors {
			if c == nil || c == cursorArrow || c == cursorNo || c.Native() == 0 {
				ReleaseDropCursors(cursors)
				t.Fatalf("native cursor allocation fell back: cycle %d action %d", n, i)
			}
		}
		ReleaseDropCursors(cursors)
		ReleaseDropCursors(cursors)
		for _, c := range cursors {
			if c.Native() != 0 {
				t.Fatal("native cursor was not released")
			}
		}
	}
	if cursorArrow == nil || cursorArrow.Native() == 0 {
		t.Fatal("shared arrow was destroyed")
	}
}
