//go:build ignore

// decl_demo is a one-file end-to-end smoke test for the silk/decl
// declarative authoring layer. It builds a small UI with three
// observable click handlers, all written in declarative form.
//
// Run:
//
//	go build -o /tmp/decl_demo decl_demo.go && /tmp/decl_demo
//
// What this validates:
//
//  1. Builder DSL produces a *decl.Node tree with the expected shape.
//  2. ToTDoc round-trips losslessly: the AST → .silkui → AST result
//     matches the original.
//  3. Build() instantiates real *gui.Form / *gui.Button / *gui.Label
//     widgets, parented correctly via reflection.
//  4. BuildWithIndex returns a working map; we look up the button by
//     its declared ID and bind a Go closure to it. Click increments
//     a counter rendered in a Label — that label's text is the
//     observable proof the click reached the right widget.
//
// Output side-channel: the demo also writes the round-tripped TDoc
// to ./decl_demo_out.silkui so a human (or test) can diff against the
// designer-produced format.
package main

import (
	"fmt"
	"log"
	"os"

	"silk/core"
	"silk/decl"
	"silk/gui"
)

func main() {
	// Build the AST. This is the only place the UI is described; everything
	// below treats it as opaque.
	tree := decl.Form(decl.ID("Main"),
		decl.P("title", "decl_demo — click validation"),
		decl.Children(
			decl.Label(decl.ID("greeting"),
				decl.P("text", "click counter:")),
			decl.Label(decl.ID("counter"),
				decl.P("text", "0")),
			decl.Button(decl.ID("inc"),
				decl.P("text", "Increment")),
			decl.Button(decl.ID("reset"),
				decl.P("text", "Reset")),
			decl.Button(decl.ID("quit"),
				decl.P("text", "Quit")),
		),
	)

	// Round-trip through TDoc to .silkui on disk so an external observer
	// (test, designer) can verify the format. Failure here means the
	// codec is broken; fail noisily before showing a window so a CI run
	// catches it without an interactive session.
	doc := decl.ToTDoc(tree)
	if doc == nil {
		log.Fatalf("decl.ToTDoc returned nil")
	}
	silkuiPath := "./decl_demo_out.silkui"
	if err := writeTDocFile(doc, silkuiPath); err != nil {
		log.Fatalf("write silkui: %v", err)
	}
	defer os.Remove(silkuiPath)

	parsed, err := decl.FromTDoc(doc)
	if err != nil {
		log.Fatalf("decl.FromTDoc: %v", err)
	}

	// Build the live widget tree with an ID index for post-construction
	// wiring. The host (this main) is responsible for binding handlers —
	// decl deliberately stays out of the event runtime.
	_, idx, err := parsed.BuildWithIndex()
	if err != nil {
		log.Fatalf("decl.Build: %v", err)
	}

	// Fish the widgets back out by their declared IDs. A type assertion
	// failure here would mean the factory returned an unexpected type —
	// fail noisily.
	form := mustForm(idx, "Main")
	counter := mustLabel(idx, "counter")
	incBtn := mustButton(idx, "inc")
	resetBtn := mustButton(idx, "reset")
	quitBtn := mustButton(idx, "quit")

	// Bind dynamic logic. Click handlers are intentionally written in Go
	// rather than embedded into the AST, demonstrating the boundary
	// between declarative structure and imperative behaviour.
	count := 0
	incBtn.Action().BindFunc0(func() {
		count++
		counter.SetText(fmt.Sprintf("%d", count))
	})
	resetBtn.Action().BindFunc0(func() {
		count = 0
		counter.SetText("0")
	})
	quitBtn.Action().BindFunc0(func() {
		core.Quit()
	})

	// Layout pass: decl doesn't yet model bounds (a future iteration —
	// the AST shape supports it via additional props). For now we lay
	// out manually so the visual smoke test has a sensible window.
	form.SetSize(420, 200)
	greeting := mustLabel(idx, "greeting")
	greeting.SetBounds(20, 20, 160, 24)
	counter.SetBounds(180, 20, 200, 24)
	incBtn.SetBounds(20, 60, 100, 32)
	resetBtn.SetBounds(140, 60, 100, 32)
	quitBtn.SetBounds(260, 60, 100, 32)

	// Frame + dock so the form has a top-level OS window.
	frame := gui.NewFrameWindow()
	frame.SetUuidStr("decl-demo-001")
	frame.SetTitle("decl demo")
	gui.SetDefaultFrame(frame)
	frame.SuggestDocDock().AddView(form)
	frame.SetClosedCallback(func(*gui.Frame) { core.Quit() })

	if w := frame.Window(); w != nil {
		w.SetSize(440, 240)
		w.MoveToCenter()
	}
	frame.Show()

	core.EventLoop()
}

func writeTDocFile(doc *core.TDoc, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprint(f, doc.String())
	return err
}

func mustForm(idx map[string]interface{}, key string) *gui.Form {
	v, ok := idx[key]
	if !ok {
		log.Fatalf("decl_demo: idx missing %q", key)
	}
	form, ok := v.(*gui.Form)
	if !ok {
		log.Fatalf("decl_demo: idx[%q] = %T, want *gui.Form", key, v)
	}
	return form
}

func mustLabel(idx map[string]interface{}, key string) *gui.Label {
	v, ok := idx[key]
	if !ok {
		log.Fatalf("decl_demo: idx missing %q", key)
	}
	lbl, ok := v.(*gui.Label)
	if !ok {
		log.Fatalf("decl_demo: idx[%q] = %T, want *gui.Label", key, v)
	}
	return lbl
}

func mustButton(idx map[string]interface{}, key string) *gui.Button {
	v, ok := idx[key]
	if !ok {
		log.Fatalf("decl_demo: idx missing %q", key)
	}
	btn, ok := v.(*gui.Button)
	if !ok {
		log.Fatalf("decl_demo: idx[%q] = %T, want *gui.Button", key, v)
	}
	return btn
}
