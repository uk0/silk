package gui

import (
	"testing"

	"github.com/uk0/silk/core"
)

// TestBackendRegistersMainLoop is the regression guard for a class of failure a
// build cannot catch: the window backend compiles, links and then leaves core
// without an event loop, so every silk program panics at startup with
//
//	panic: main loop mechanism unavailable, typically you need a gui package
//
// That is exactly what happened on Windows — window_windows.go implemented
// MainLoop and QuitLoop but its core.SetMainLoop call sat commented out
// referring to names that did not exist, and nothing on any platform noticed
// until someone ran the program on Windows.
//
// Importing gui must always leave a usable loop behind, on every GOOS.
func TestBackendRegistersMainLoop(t *testing.T) {
	if !core.HasMainLoop() {
		t.Fatal("importing gui did not register a main loop with core; " +
			"core.EventLoop() would panic at startup on this platform")
	}
}
