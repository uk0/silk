//go:build !windows

package gui

import (
	"fmt"
	"os"
	"testing"
)

var nativeCursorProbeErr error

// TestMain runs this on Cocoa's pinned main thread, not a testing goroutine.
func runNativeCursorProbe() error {
	if err := nativeDesktopError("cursor probe"); err != nil {
		return err
	}
	for n := 0; n < 64; n++ {
		cursors := GenerateDropCursors(nil)
		for i, c := range cursors {
			if c == nil || c == cursorArrow || c == cursorNo || c.glfwCursor == nil {
				ReleaseDropCursors(cursors)
				return fmt.Errorf("native cursor allocation fell back: cycle %d action %d", n, i)
			}
		}
		ReleaseDropCursors(cursors)
		ReleaseDropCursors(cursors)
		for _, c := range cursors {
			if c.glfwCursor != nil {
				return fmt.Errorf("native cursor was not released")
			}
		}
	}
	if cursorArrow == nil || cursorArrow.glfwCursor == nil {
		return fmt.Errorf("shared arrow was destroyed")
	}
	return nil
}
func TestNativeDragCursorLifecycle(t *testing.T) {
	if os.Getenv("SILK_TEST_NATIVE_CURSORS") != "1" {
		t.Skip("opt in with SILK_TEST_NATIVE_CURSORS=1")
	}
	if nativeCursorProbeErr != nil {
		t.Fatal(nativeCursorProbeErr)
	}
}
