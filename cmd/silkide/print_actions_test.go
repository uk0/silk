package main

import (
	"bytes"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/printing"
)

func TestPrintDesignOffersDeferredPreviewAndRejectsStaleDesign(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	old := showDesignPrintDialog
	t.Cleanup(func() { showDesignPrintDialog = old })
	calls := 0
	var deferred func() (gui.IWidget, error)
	showDesignPrintDialog = func(owner gui.IWidget, source func() (gui.IWidget, error), opts printing.Options, finished ...func(printing.Job, error)) {
		calls++
		deferred = source
		if owner != w.canvas || opts.Copies != 1 || source == nil {
			t.Fatal("print dialog lost the current design owner/source/options")
		}
		for _, fn := range finished {
			fn(printing.Job{}, printing.ErrCanceled)
		}
	}
	b := menuCommandButton(buildHamburgerMenu(gui.NewButton(), w.tabs, w.canvas), "file.print")
	if b == nil || !b.IsEnabled() || b.Icon() == nil {
		t.Fatal("Print Design menu missing")
	}
	b.Action().Trigger(b)
	if calls != 1 || deferred == nil {
		t.Fatalf("print dialog calls=%d", calls)
	}
	// The accepted action can obtain an independent renderable preview.
	preview, err := deferred()
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	if err := gui.RenderWidgetPDF(&result, preview); err != nil ||
		!bytes.HasPrefix(result.Bytes(), []byte("%PDF-")) ||
		!bytes.Contains(result.Bytes(), []byte("/Subtype /Image")) {
		t.Fatalf("confirmed preview did not render to a complete PDF: %v", err)
	}
	w.activate(modeWelcome)
	if preview, err := deferred(); preview != nil || err == nil {
		t.Fatalf("stale capture silently used an out-of-view design: preview=%v, err=%v", preview != nil, err)
	}
}

func TestPrintDesignCannotPrintHiddenDocument(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeWelcome)
	old := showDesignPrintDialog
	t.Cleanup(func() { showDesignPrintDialog = old })
	showDesignPrintDialog = func(gui.IWidget, func() (gui.IWidget, error), printing.Options, ...func(printing.Job, error)) {
		t.Fatal("hidden design reached print backend")
	}
	b := menuCommandButton(buildHamburgerMenu(gui.NewButton(), w.tabs, w.canvas), "file.print")
	if b == nil || b.IsEnabled() {
		t.Fatal("Print Design enabled outside design mode")
	}
	performPrintDesign(w.canvas)
	performPrintDesign(nil)
}
