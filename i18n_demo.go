//go:build ignore

// i18n_demo wires silk/i18n into a small decl-driven UI to demonstrate
// translation at the widget layer. Buttons use decl.TrKey values; the
// runtime resolves them through the active locale at Build time. Run:
//
//	go build -o /tmp/i18n_demo i18n_demo.go && /tmp/i18n_demo
//
// Default locale is zh-CN with the bundled translations.json. Pass
// SILK_LOCALE=en (or ja, ko, ...) at the env to switch.
package main

import (
	"fmt"
	"log"
	"os"

	"silk/core"
	"silk/decl"
	"silk/gui"
	"silk/i18n"
)

func main() {
	// Load bundled sample translations + activate locale. Apps that
	// don't ship translations or run on hosts without translation
	// files still render correctly thanks to source-string fallback.
	if err := i18n.LoadFromFile("./i18n/example/translations.json"); err != nil {
		log.Printf("warning: load translations: %v", err)
	}

	// Locale resolution: env override > host detection. zh-CN is the
	// fallback when nothing's configured because that's where Silk's
	// CJK font work was scoped.
	locale := os.Getenv("SILK_LOCALE")
	if locale == "" {
		if detected, err := i18n.DetectLocale(); err == nil {
			locale = detected
		} else {
			locale = "zh-CN"
		}
	}
	i18n.SetLocale(locale)
	log.Printf("i18n_demo: locale=%s", i18n.Locale())

	// Build the AST. Every label/button text is a TrKey: "File" /
	// "Save" / "OK" / "Cancel". The same AST renders correctly in
	// every supported locale; switching SILK_LOCALE re-runs and
	// re-renders without touching this code.
	tree := decl.Form(decl.ID("Main"),
		decl.P("title", decl.TrKey{Source: "About"}),
		decl.Children(
			decl.Label(decl.ID("greet"), decl.P("text", decl.TrKey{Source: "Settings"})),
			decl.Label(decl.ID("counter"), decl.P("text", "0")),
			decl.Button(decl.ID("save"), decl.P("text", decl.TrKey{Source: "Save"})),
			decl.Button(decl.ID("cancel"), decl.P("text", decl.TrKey{Source: "Cancel"})),
			decl.Button(decl.ID("quit"), decl.P("text", decl.TrKey{Source: "Quit"})),
		),
	)

	root, idx, err := tree.BuildWithIndex()
	if err != nil {
		log.Fatalf("Build: %v", err)
	}

	form := root.(*gui.Form)
	greet := idx["greet"].(*gui.Label)
	counter := idx["counter"].(*gui.Label)
	saveBtn := idx["save"].(*gui.Button)
	cancelBtn := idx["cancel"].(*gui.Button)
	quitBtn := idx["quit"].(*gui.Button)

	form.SetSize(420, 200)
	greet.SetBounds(20, 20, 180, 24)
	counter.SetBounds(200, 20, 200, 24)
	saveBtn.SetBounds(20, 60, 100, 32)
	cancelBtn.SetBounds(140, 60, 100, 32)
	quitBtn.SetBounds(260, 60, 100, 32)

	clicks := 0
	saveBtn.Action().BindFunc0(func() {
		clicks++
		// Use Tn for plurals: "%d item" vs "%d items" in en, single
		// form in zh/ja/ko. The same call adapts to the active locale.
		counter.SetText(i18n.Tn("%d item", "%d items", clicks))
	})
	cancelBtn.Action().BindFunc0(func() {
		clicks = 0
		counter.SetText("0")
	})
	quitBtn.Action().BindFunc0(func() { core.Quit() })

	frame := gui.NewFrameWindow()
	frame.SetUuidStr("i18n-demo-001")
	frame.SetTitle(fmt.Sprintf("i18n demo · locale=%s", locale))
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
