package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/paint"
)

// addConfiguredFake places a designer widget of the given factory into scene
// under name, first letting configure mutate its underlying widget's design
// properties. Returns nothing — the widget is owned by the scene.
func addConfiguredFake(t *testing.T, scene *GedScene, factory, name string, configure func(w interface{})) {
	t.Helper()
	w, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		t.Fatalf("create %s: %v", factory, err)
	}
	w.SetWidgetName(name)
	w.SetBounds(5, 5, 40, 30)
	if configure != nil {
		configure(w.Widget())
	}
	cmd := graph.NewAddCommand()
	cmd.AddItem(w, scene)
	scene.PushCommand(cmd)
}

// TestGenerateCodeDesignProperties verifies codegen reproduces the design-time
// properties a designer changed (color, range) and suppresses ones left at
// their default, then type-checks the emitted setters.
func TestGenerateCodeDesignProperties(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("HMI")
	scene.SetSize(200, 150)

	// Tank: change color (default {33,150,243,255}) and max (default 100);
	// leave min (0) and showLabel (true) at defaults.
	addConfiguredFake(t, scene, "gui.Tank", "tank1", func(w interface{}) {
		w.(interface{ SetColor(paint.Color) }).SetColor(paint.Color{R: 10, G: 20, B: 30, A: 255})
		w.(interface{ SetMax(float64) }).SetMax(250)
	})
	// DigitalDisplay: change string props (format + unit).
	addConfiguredFake(t, scene, "gui.DigitalDisplay", "disp1", func(w interface{}) {
		w.(interface{ SetFormat(string) }).SetFormat("%.2f")
		w.(interface{ SetUnit(string) }).SetUnit("bar")
	})
	// Indicator: change a bool prop (blink).
	addConfiguredFake(t, scene, "gui.Indicator", "ind1", func(w interface{}) {
		w.(interface{ SetBlink(bool) }).SetBlink(true)
	})
	// Gauge left entirely at defaults — must emit no design setters.
	addConfiguredFake(t, scene, "gui.Gauge", "gauge1", nil)

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "HMIUI"})

	mustContain := []string{
		"ui.Tank1.SetColor(paint.Color{R: 10, G: 20, B: 30, A: 255})",
		"ui.Tank1.SetMax(250)",
		`ui.Disp1.SetFormat("%.2f")`,
		`ui.Disp1.SetUnit("bar")`,
		"ui.Ind1.SetBlink(true)",
	}
	for _, s := range mustContain {
		if !strings.Contains(code, s) {
			t.Errorf("generated code missing changed property:\n  %s", s)
		}
	}
	mustNotContain := []string{
		"ui.Tank1.SetMin(",       // min unchanged (0) -> suppressed
		"ui.Tank1.SetShowLabel(", // showLabel unchanged (true) -> suppressed
		"ui.Gauge1.SetMin(",      // gauge fully default -> nothing
		"ui.Gauge1.SetMax(",
		"ui.Gauge1.SetUnit(",
	}
	for _, s := range mustNotContain {
		if strings.Contains(code, s) {
			t.Errorf("generated code emitted a default property it should have suppressed:\n  %s", s)
		}
	}

	// The emitted SetColor/SetMax must type-check against the real gui + paint.
	vetGeneratedCode(t, code)
}

// TestGenerateCodeColorSurvivesReload is the third leg of the color
// round-trip. emitDesignProperties reads the *live* widget, not the design
// document, so a color the designer set was in the saved file yet gone from the
// generated screen the moment the file was reopened: captureWidgetProperties
// skipped paint.Color, so LoadDesign rebuilt the widget without it. Save, load
// into a fresh scene, generate — the setters have to still be there.
func TestGenerateCodeColorSurvivesReload(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("HMI")
	scene.SetSize(200, 150)

	addConfiguredFake(t, scene, "gui.Tank", "tank1", func(w interface{}) {
		w.(interface{ SetColor(paint.Color) }).SetColor(paint.Color{R: 10, G: 20, B: 30, A: 255})
	})
	// Two colors on one widget, and a translucent one, so the alpha channel and
	// a second setter on the same object are covered too.
	addConfiguredFake(t, scene, "gui.Valve", "v1", func(w interface{}) {
		w.(interface{ SetOpenColor(paint.Color) }).SetOpenColor(paint.Color{R: 0, G: 200, B: 0, A: 255})
		w.(interface{ SetClosedColor(paint.Color) }).SetClosedColor(paint.Color{R: 200, G: 0, B: 0, A: 128})
	})

	reloaded := NewGedScene()
	if err := reloaded.LoadDesign(scene.SaveDesign()); err != nil {
		t.Fatalf("reload design: %v", err)
	}

	code := reloaded.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "HMIUI"})
	for _, s := range []string{
		"ui.Tank1.SetColor(paint.Color{R: 10, G: 20, B: 30, A: 255})",
		"ui.V1.SetOpenColor(paint.Color{R: 0, G: 200, B: 0, A: 255})",
		"ui.V1.SetClosedColor(paint.Color{R: 200, G: 0, B: 0, A: 128})",
	} {
		if !strings.Contains(code, s) {
			t.Errorf("reopened design lost a designed color:\n  %s", s)
		}
	}

	vetGeneratedCode(t, code)
}

// TestGenerateCodeStandardWidgetProperties verifies design-property
// reproduction also covers standard input widgets (float range/value, int
// range, bool state).
func TestGenerateCodeStandardWidgetProperties(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Panel")
	scene.SetSize(200, 150)

	addConfiguredFake(t, scene, "gui.Slider", "sld1", func(w interface{}) {
		w.(interface{ SetMax(float64) }).SetMax(777)
		w.(interface{ SetValue(float64) }).SetValue(42)
	})
	addConfiguredFake(t, scene, "gui.SpinBox", "spn1", func(w interface{}) {
		w.(interface{ SetMax(int) }).SetMax(12345)
	})
	addConfiguredFake(t, scene, "gui.CheckBox", "cb1", func(w interface{}) {
		w.(interface{ SetChecked(bool) }).SetChecked(true)
	})

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "PanelUI"})
	for _, s := range []string{
		"ui.Sld1.SetMax(777)",
		"ui.Sld1.SetValue(42)",
		"ui.Spn1.SetMax(12345)",
		"ui.Cb1.SetChecked(true)",
	} {
		if !strings.Contains(code, s) {
			t.Errorf("generated code missing standard-widget property:\n  %s", s)
		}
	}
	vetGeneratedCode(t, code)
}
