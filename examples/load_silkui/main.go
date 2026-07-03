//go:build ignore

// load_silkui — minimal SDK example that loads a .silkui design file and
// displays it at runtime. This demonstrates the pure-SDK form loader: no
// dependency on the `ged` (designer) package is required.
//
// Build:
//
//	CGO_CFLAGS="$(pkg-config --cflags cairo)" go run ./examples/load_silkui/ path/to/design.silkui
package main

import (
	"log"
	"os"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: load_silkui <file.silkui>")
	}

	// One-shot load. Any widget type registered via core.RegisterFactory
	// (all standard gui.* widgets) is resolved automatically.
	form, err := gui.LoadForm(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	f := gui.NewFrameWindow()
	f.SetTitle("Silk Loaded Form")
	gui.SetDefaultFrame(f)
	f.SuggestDocDock().AddView(form)
	f.SetClosedCallback(func(*gui.Frame) { core.Quit() })
	f.Show()

	core.EventLoop()
}
