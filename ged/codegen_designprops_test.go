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
