package ged

import (
	"strings"
	"testing"
)

// TestGenerateCodeServicesBacked drives a representative SCADA design — a
// tag-bound Tank, a field-device DeviceComponent and a RecipePanel — through
// GenerateCode and asserts the output is a scada.Services-backed app: the struct
// owns a *scada.Services, a BindServices method wires the whole screen via
// scada.BindScreen, main builds the shared container with scada.New, and no
// private core.NewTagDB is allocated. vetGeneratedCode then type-checks the
// generated program against the real scada package (which pulls in the historian
// / eventlog SQLite stores via cgo — expected).
func TestGenerateCodeServicesBacked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("Plant")
	scene.SetSize(240, 180)
	addFakeWithTag(t, scene, "gui.Tank", "tank1", "temp")      // tag-bound industrial widget
	addFakeWithTag(t, scene, "gui.DeviceComponent", "plc", "") // field-device component
	addFakeWithTag(t, scene, "gui.RecipePanel", "recipes", "") // one of the five operator panels

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "PlantUI"})

	want := []string{
		"Services *scada.Services",
		"func (ui *PlantUI) BindServices(s *scada.Services) error",
		"scada.New(",
		"scada.BindScreen(s, ui.Form,",
		`ui.Tank1.SetTagName("temp")`, // tag handed to the widget so BindScreen resolves it
		"*device.DeviceComponent",     // DeviceComponent typed to the device package
		"*gui.RecipePanel",
	}
	for _, w := range want {
		if !strings.Contains(code, w) {
			t.Errorf("services-backed output missing:\n  %s\n----\n%s", w, code)
		}
	}
	if strings.Contains(code, "core.NewTagDB()") {
		t.Errorf("services-backed output must not allocate a private core.NewTagDB()\n----\n%s", code)
	}

	// Compile the generated app against the real scada / gui / device / core API.
	vetGeneratedCode(t, code)
}

// TestGenerateCodePlainStaysServiceFree confirms an ordinary (non-SCADA) design —
// just a Button — keeps the legacy output: no scada.Services, no BindServices, no
// container bring-up in main.
func TestGenerateCodePlainStaysServiceFree(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Plain")
	scene.SetSize(100, 80)
	addFakeWithTag(t, scene, "gui.Button", "btn1", "")

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "PlainUI"})

	for _, bad := range []string{"scada.Services", "BindServices", "scada.New(", "scada.BindScreen"} {
		if strings.Contains(code, bad) {
			t.Errorf("plain design must not emit SCADA plumbing, found:\n  %s\n----\n%s", bad, code)
		}
	}
}
