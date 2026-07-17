package main

import "github.com/uk0/silk/gui"

// onUI marshals fn onto the main (event-loop / UI) thread via gui.Post.
//
// UI-thread rule: every silk widget, dock, panel, and status-bar mutation
// MUST run on the main thread. GLFW event handling and Cairo rendering are
// not goroutine-safe, so touching that state from a worker goroutine races
// the render loop. Worker goroutines (go build / test / vet, LSP, dlv, git)
// do their heavy work off-thread and hand the UI mutation back through
// onUI, which enqueues fn to run on the next event-loop iteration.
//
// This is a thin, greppable alias for gui.Post so the intent reads clearly
// at every call site. gui.Post is nil-safe and safe to call from any
// goroutine.
func onUI(fn func()) { gui.Post(fn) }
