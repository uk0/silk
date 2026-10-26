package ged

import (
	"github.com/uk0/silk/gui"
	"strings"
	"testing"
)

func TestSpinBoxDesignReloadAndGenerateCompleteProperties(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 100)
	addConfiguredFake(t, scene, "gui.SpinBox", "count", func(w interface{}) {
		s := w.(*gui.SpinBox)
		s.SetRange(-100, 100)
		s.SetValue(42)
		s.SetStep(7)
		s.SetSuffix(" items")
		s.SetReadOnly(true)
	})
	addConfiguredFake(t, scene, "gui.SpinBox", "page", func(w interface{}) { s := w.(*gui.SpinBox); s.SetPageStep(24) })
	loaded := NewGedScene()
	if err := loaded.LoadDesign(scene.SaveDesign()); err != nil {
		t.Fatal(err)
	}
	code := loaded.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "SpinUI"})
	for _, want := range []string{"ui.Count.SetMin(-100)", "ui.Count.SetMax(100)", "ui.Count.SetValue(42)", "ui.Count.SetStep(7)", `ui.Count.SetSuffix(" items")`, "ui.Count.SetReadOnly(true)", "ui.Page.SetPageStep(24)"} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %s\n%s", want, code)
		}
	}
	if strings.Contains(code, "ui.Count.SetPageStep(") || strings.Contains(code, "gui.NewEdit(") || strings.Count(code, "gui.NewSpinBox()") != 2 {
		t.Fatal("internal editor leaked or automatic page step changed", code)
	}
	vetGeneratedCode(t, code)
}
