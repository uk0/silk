package main

import (
	"fmt"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"testing"
	"unicode"
)

func TestDesignActionsSelectionAndModeGuards(t *testing.T) {
	w := testWorkbench(t)
	buildToolBar(w.frame, w.tabs, w.canvas)
	w.activate(modeDesign)
	if !w.designToolbar.IsVisible() || len(w.designActions) != 6 {
		t.Fatal("design row missing")
	}
	for i, a := range w.designActions[:4] {
		if !a.button.IsEnabled() {
			t.Fatalf("layout %d must target the whole form with no selection", i)
		}
	}
	if w.designActions[4].button.IsEnabled() {
		t.Fatal("break enabled with no existing layout")
	}
	for i := 0; i < 2; i++ {
		widget, err := ged.NewFakeWidgetFromFactory("gui.Button")
		if err != nil {
			t.Fatal(err)
		}
		widget.SetBounds(float64(10+i*40), 10, 30, 10)
		widget.SetParent(w.canvas.GedScene())
		w.canvas.Selection().Add(widget)
		if i == 0 {
			w.syncMode()
			for _, action := range w.designActions[:4] {
				if action.button.IsEnabled() {
					t.Fatalf("action %q would silently lay out the form with one selected control", action.spec.label)
				}
			}
		}
	}
	w.syncMode() // Same mode: selection state must still update.
	grid := w.designActions[2]
	if !grid.button.IsEnabled() || gui.GetToolTip(grid.button) == "" {
		t.Fatal("grid action not available or undocumented")
	}
	grid.button.Action().Trigger(nil)
	box, ok := w.canvas.GedScene().Children()[0].(*ged.FakeWidget)
	if !ok || box.WidgetFactoryName() != string(ged.LayoutGrid) {
		t.Fatal("toolbar did not apply real GridLayout")
	}
	w.syncMode()
	if !w.designActions[4].button.IsEnabled() {
		t.Fatal("break action unavailable after layout")
	}
	w.activate(modeEdit)
	if w.designToolbar.IsVisible() {
		t.Fatal("design toolbar remains over editor")
	}
	w.designActions[4].spec.run() // Palette uses this same guarded handler.
	if len(w.canvas.GedScene().Children()) != 1 {
		t.Fatal("hidden canvas mutated")
	}
	w.activate(modeDesign)
	w.designActions[4].button.Action().Trigger(nil)
	if len(w.canvas.GedScene().Children()) != 2 {
		t.Fatal("break layout lost children")
	}
}

func TestCommandPaletteNamesAreUniqueAndRegistrationIdempotent(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	registerPaletteCommands(w.tabs, w.canvas)
	n := len(paletteCommands)
	registerPaletteCommands(w.tabs, w.canvas)
	if len(paletteCommands) != n {
		t.Fatal("registration appends stale commands")
	}
	counts := map[string]int{}
	for _, cmd := range paletteCommands {
		counts[cmd.Name]++
	}
	for name, count := range counts {
		if count != 1 {
			t.Errorf("duplicate command %q (%d)", name, count)
		}
	}
	for _, spec := range designerActionSpecs(w.canvas) {
		if counts[spec.label] != 1 {
			t.Errorf("missing layout command %q", spec.label)
		}
	}
}

func TestDesignActionDescriptorsHaveNamedIconsAndHints(t *testing.T) {
	for _, spec := range designerActionSpecs(nil) {
		if spec.label == "" || spec.hint == "" {
			t.Fatal("anonymous designer action")
		}
		if icon := paint.LoadIcon(spec.icon); icon == nil || icon.IsAir() || fmt.Sprint(icon) != spec.icon {
			t.Errorf("action %q resolves missing icon %q", spec.label, spec.icon)
		}
	}
}

func TestDesignerActionsTranslateTogether(t *testing.T) {
	old := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(old) })
	registerSilkideTranslations()
	for _, locale := range []string{"en", "zh-CN"} {
		i18n.SetLocale(locale)
		specs := designerActionSpecs(nil)
		want := []string{"Lay Out Horizontally", "Lay Out Vertically", "Lay Out in a Grid", "Lay Out in a Form", "Break Layout", "Fit to View"}
		if locale == "zh-CN" {
			want = []string{"水平布局", "垂直布局", "网格布局", "表单布局", "解除布局", "适应视口"}
		}
		for i, spec := range specs {
			if spec.label != want[i] {
				t.Fatalf("%s action %d=%q", locale, i, spec.label)
			}
			if locale == "en" {
				for _, r := range spec.hint {
					if unicode.Is(unicode.Han, r) {
						t.Fatalf("untranslated hint %q", spec.hint)
					}
				}
			}
			if spec.enabled() {
				t.Fatal("nil canvas action enabled")
			}
		}
	}
}

func TestDesignPaletteAvailabilityMatchesToolbar(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	buildToolBar(w.frame, w.tabs, w.canvas)
	registerPaletteCommands(w.tabs, w.canvas)
	commands := bindPaletteCommands(paletteCommands)
	find := func(name string) paletteCommand {
		t.Helper()
		for _, c := range commands {
			if c.Name == name {
				return c
			}
		}
		t.Fatalf("missing command %q", name)
		return paletteCommand{}
	}
	check := func() {
		t.Helper()
		w.syncMode()
		for _, a := range w.designActions {
			c := find(a.spec.label)
			want := a.spec.enabled()
			if c.Available == nil || c.available() != want || a.button.IsEnabled() != want {
				t.Errorf("mode=%s command=%q palette=%v toolbar=%v want=%v", documentMode(w.tabs, w.canvas), c.Name, c.available(), a.button.IsEnabled(), want)
			}
			if (paletteRowPresentation(c).TrailingText == i18n.T("Unavailable")) == want {
				t.Errorf("wrong disabled label: %s", paletteDescription(c))
			}
		}
	}
	w.activate(modeDesign)
	check() // Empty selection uses the form as the layout target.
	for i := 0; i < 2; i++ {
		widget, err := ged.NewFakeWidgetFromFactory("gui.Button")
		if err != nil {
			t.Fatal(err)
		}
		widget.SetBounds(float64(10+i*40), 10, 30, 10)
		widget.SetParent(w.canvas.GedScene())
		w.canvas.Selection().Add(widget)
	}
	check()
	grid := find(w.designActions[2].spec.label)
	// A modal may change state before the deferred command is run.
	pending := &paletteSelection{command: &grid}
	w.activate(modeEdit)
	check()
	pending.run(gui.DialogOK)
	if len(w.canvas.GedScene().Children()) != 2 {
		t.Fatal("stale palette action mutated hidden canvas")
	}
	w.activate(modeWelcome)
	check()
	w.activate(modeDesign)
	grid.Fn()
	check()
	if len(w.canvas.GedScene().Children()) != 1 {
		t.Fatal("enabled palette grid did not execute")
	}
	find(w.designActions[4].spec.label).Fn()
	check()
	if len(w.canvas.GedScene().Children()) != 2 {
		t.Fatal("palette break layout lost children")
	}
	w.canvas.Selection().Clear()
	check()
	if !grid.available() {
		t.Fatal("whole-form layout missing from the command palette")
	}
	dlg, selection := newCommandPalette(w.canvas)
	selection.choose(dlg, find(w.designActions[4].spec.label))
	if selection.command != nil {
		t.Fatal("disabled command closed palette")
	}
}
